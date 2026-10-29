package sitepublish_usecase

import (
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"mime"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"

	"vozko/domain/media"
	"vozko/domain/site"
	"vozko/domain/siteform"
	"vozko/domain/sitehtml"
	"vozko/domain/sitepublish"
	"vozko/domain/webchat"
)

type PublishResult struct {
	Deployment            sitepublish.DeploymentRecord
	Held                  bool
	Hostnames             []string
	ProjectVersion        int64
	RevisionID            string
	HasUnpublishedChanges bool
}

const originPlaceholder = sitepublish.OriginPlaceholder

type routeFile struct {
	route       string
	file        string
	interactive bool
	countView   bool
}

func (s *Service) Publish(ctx context.Context, a Actor, projectID string) (PublishResult, error) {
	return s.publish(ctx, a, projectID, 0)
}

func (s *Service) PublishPinned(ctx context.Context, a Actor, projectID string, version int64) (PublishResult, error) {
	if version < 1 {
		return PublishResult{}, ErrPublishStale
	}
	return s.publish(ctx, a, projectID, version)
}

func (s *Service) publish(ctx context.Context, a Actor, projectID string, pinned int64) (PublishResult, error) {
	if err := s.allow(a, ResourceSites, ActionPublish); err != nil {
		return PublishResult{}, err
	}
	if err := s.ensureActive(a.WorkspaceID); err != nil {
		return PublishResult{}, err
	}
	var res PublishResult
	err := s.exclusive(projectID, func() error {
		var err error
		res, err = s.publishLocked(ctx, a, projectID, pinned)
		return err
	})
	return res, err
}

func (s *Service) publishLocked(ctx context.Context, a Actor, projectID string, pinned int64) (PublishResult, error) {
	if err := s.reviewable(ctx, projectID); err != nil {
		return PublishResult{}, err
	}
	snap, err := s.working(ctx, a, projectID)
	if err != nil {
		return PublishResult{}, err
	}
	if pinned > 0 && snap.Version != pinned {
		return PublishResult{}, fmt.Errorf("%w: proposed at %d, now %d", ErrPublishStale, pinned, snap.Version)
	}
	if err := s.ensurePlatformHostname(ctx, a.WorkspaceID, snap); err != nil {
		return PublishResult{}, err
	}
	rec := sitepublish.DeploymentRecord{
		ID: s.NewID(), WorkspaceID: a.WorkspaceID, ProjectID: projectID, RevisionID: snap.RevisionID,
		Status: sitepublish.DeploymentBuilding, CreatedAt: s.Now(),
	}
	if err := s.Deployments.Create(ctx, rec); err != nil {
		return PublishResult{}, err
	}
	built, pages, err := s.build(ctx, snap, &rec)
	if err != nil {
		return PublishResult{}, s.failDeployment(ctx, rec, err)
	}
	if err := s.upload(ctx, &rec, built); err != nil {
		return PublishResult{}, s.failDeployment(ctx, rec, err)
	}
	reason, hold := holdReason(pages, built)
	if hold {
		if err := s.Abuse.Create(ctx, sitepublish.AbuseCase{
			ID: s.NewID(), WorkspaceID: a.WorkspaceID, ProjectID: projectID, DeploymentID: rec.ID,
			Source: sitepublish.AbuseHeuristic, Status: sitepublish.AbuseHeld, Reason: reason, CreatedAt: s.Now(),
		}); err != nil {
			return PublishResult{}, s.failDeployment(ctx, rec, fmt.Errorf("sitepublish: the review hold was not recorded: %w", err))
		}
	}
	rec.Status = sitepublish.DeploymentReady
	if err := s.Deployments.Update(ctx, rec); err != nil {
		return PublishResult{}, err
	}
	if hold {
		return PublishResult{Deployment: rec, Held: true, ProjectVersion: snap.Version, RevisionID: snap.RevisionID, HasUnpublishedChanges: true}, nil
	}
	hosts, err := s.goLive(ctx, rec, a.Ref())
	if err != nil {
		return PublishResult{}, err
	}
	s.Thumbnails.Published(ctx, a.WorkspaceID, projectID, built.url)
	now := s.Now()
	rec.Status, rec.PublishedBy, rec.PublishedAt = sitepublish.DeploymentActive, a.Ref(), &now
	recorded, err := s.Projects.RecordPublish(ctx, PublishRecord{
		WorkspaceID: a.WorkspaceID, ProjectID: projectID, SourceRevisionID: snap.RevisionID, SourceVersion: snap.Version, DeploymentID: rec.ID, Project: snap.Project, Author: a.Ref(),
	})
	if err != nil {
		return PublishResult{}, fmt.Errorf("%w: deployment %s: %w", ErrPublishUnrecorded, rec.ID, err)
	}
	rec.RevisionID = recorded.RevisionID
	if err := s.Deployments.RecordPublished(ctx, projectID, rec.ID, recorded.RevisionID); err != nil {
		return PublishResult{}, fmt.Errorf("%w: deployment %s: %w", ErrPublishUnrecorded, rec.ID, err)
	}
	return PublishResult{Deployment: rec, Hostnames: hosts, ProjectVersion: recorded.Version, RevisionID: recorded.RevisionID, HasUnpublishedChanges: !recorded.Current}, nil
}

func (s *Service) working(ctx context.Context, a Actor, projectID string) (ProjectSnapshot, error) {
	snap, err := s.Projects.Working(ctx, a.WorkspaceID, projectID)
	if err != nil {
		return ProjectSnapshot{}, err
	}
	settings, err := s.Projects.CurrentSettings(ctx, a.WorkspaceID, projectID)
	if err != nil {
		return ProjectSnapshot{}, err
	}
	snap.Settings = settings
	return snap, nil
}

func holdReason(pages []site.RenderedPage, built builtSite) (string, bool) {
	if reason, hold := sitepublish.HoldForReview(pages); hold {
		return reason, true
	}
	if reason, hold := sitepublish.HoldForReview(built.live); hold {
		return "live " + reason, true
	}
	return sitepublish.HoldDestinationsForReview(built.destinations)
}

func (s *Service) primaryOrigin(ctx context.Context, snap ProjectSnapshot) (string, error) {
	list, err := s.Hostnames.ListByProject(ctx, snap.WorkspaceID, snap.ID)
	if err != nil {
		return "", err
	}
	origin, ok := sitepublish.PrimaryOrigin(list, s.cfg.Addresses.Origins)
	if !ok {
		return "", fmt.Errorf("%w: the page has no active hostname", ErrBuildFailed)
	}
	return origin, nil
}

var bundleReference = regexp.MustCompile(`(?:src|href)="(/[^"#?]+)"`)

func dropPrerender(bodies map[string][]byte, prerenderPath string) {
	if prerenderPath == "" {
		return
	}
	page := "/" + strings.TrimPrefix(prerenderPath, "/")
	var candidates []string
	for _, m := range bundleReference.FindAllStringSubmatch(string(bodies[page]), -1) {
		candidates = append(candidates, m[1])
	}
	delete(bodies, page)
	for changed := true; changed; {
		changed = false
		for _, p := range candidates {
			if _, ok := bodies[p]; !ok || referenced(bodies, p) {
				continue
			}
			delete(bodies, p)
			changed = true
		}
	}
}

func referenced(bodies map[string][]byte, p string) bool {
	name := path.Base(p)
	for other, body := range bodies {
		if other != p && strings.Contains(string(body), name) {
			return true
		}
	}
	return false
}

func (s *Service) linkWhatsApp(ctx context.Context, projectID, route, html string) (string, error) {
	linked, err := sitepublish.LinkWhatsApp(html, func(a sitepublish.WhatsAppAnchor) (string, bool, error) {
		link, err := s.Forms.WhatsAppLink(ctx, projectID, a.NumberRef, a.Text, route)
		if err != nil {
			return "", false, err
		}
		return link, true, nil
	})
	if err != nil {
		return "", fmt.Errorf("route %s: %w", route, err)
	}
	return linked, nil
}

func (s *Service) reviewable(ctx context.Context, projectID string) error {
	open, err := s.Abuse.OpenForProject(ctx, projectID)
	if err != nil {
		return err
	}
	for _, c := range open {
		if c.Status == sitepublish.AbuseSuspended {
			return ErrSiteSuspended
		}
		if c.Status == sitepublish.AbuseHeld {
			return ErrUnderReview
		}
	}
	return nil
}

func (s *Service) goLive(ctx context.Context, rec sitepublish.DeploymentRecord, publishedBy string) ([]string, error) {
	hosts, err := s.flip(ctx, rec, publishedBy)
	if err != nil {
		s.restorePrevious(ctx, rec)
		return nil, err
	}
	return hosts, nil
}

func (s *Service) flip(ctx context.Context, rec sitepublish.DeploymentRecord, publishedBy string) ([]string, error) {
	if err := s.activateForms(ctx, rec); err != nil {
		return nil, err
	}
	settings, err := s.serving(ctx, rec)
	if err != nil {
		return nil, err
	}
	hosts, err := s.writeKeys(ctx, rec, settings)
	if err != nil {
		return nil, err
	}
	if err := s.dropOtherAddresses(ctx, rec.ProjectID, rec.ID); err != nil {
		return nil, err
	}
	if err := s.Deployments.Activate(ctx, rec.ProjectID, rec.ID, publishedBy, s.Now()); err != nil {
		return nil, err
	}
	return hosts, nil
}

func (s *Service) restorePrevious(ctx context.Context, failed sitepublish.DeploymentRecord) {
	projectID := failed.ProjectID
	if err := s.dropAddress(ctx, failed.ID); err != nil {
		log.Printf("[sitepublish] project %s: deployment %s may keep an address after failing to go live: %v", projectID, failed.ID, err)
	}
	previous, found, err := s.Deployments.Active(ctx, projectID)
	if err != nil {
		log.Printf("[sitepublish] project %s: could not read the live deployment to restore it: %v", projectID, err)
		return
	}
	if !found {
		hosts, err := s.Hostnames.ActiveHostnames(ctx, projectID)
		if err != nil {
			log.Printf("[sitepublish] project %s: hostnames may serve a deployment that did not go live: %v", projectID, err)
		}
		for _, host := range hosts {
			if err := s.Hosting.DeleteHost(ctx, host); err != nil {
				log.Printf("[sitepublish] project %s: %s may serve a deployment that did not go live: %v", projectID, host, err)
			}
		}
		if err := s.deactivateForms(ctx, projectID); err != nil {
			log.Printf("[sitepublish] project %s: forms of a deployment that did not go live may stay active: %v", projectID, err)
		}
		return
	}
	settings, err := s.serving(ctx, previous)
	if err == nil {
		_, err = s.writeKeys(ctx, previous, settings)
	}
	if err != nil {
		log.Printf("[sitepublish] project %s: keys may point at a deployment that did not go live: %v", projectID, err)
	}
	if err == nil && settings.Suspended {
		err = s.deactivateForms(ctx, projectID)
	} else {
		err = s.activateForms(ctx, previous)
	}
	if err != nil {
		log.Printf("[sitepublish] project %s: the live forms may not match the live deployment: %v", projectID, err)
	}
}

type builtSite struct {
	url          string
	files        map[string]sitepublish.FileEntry
	bodies       map[string][]byte
	routes       map[string]string
	integrations []site.IntegrationKind
	policy       site.ContentPolicy
	live         []site.RenderedPage
	destinations []string
}

func (s *Service) build(ctx context.Context, snap ProjectSnapshot, rec *sitepublish.DeploymentRecord) (builtSite, []site.RenderedPage, error) {
	formIDs := sitepublish.FormIDs(snap.Project)
	for _, id := range slices.Concat(slices.Collect(maps.Keys(snap.Settings.Forms)), slices.Collect(maps.Keys(snap.Settings.Bookings))) {
		if !slices.Contains(formIDs, id) {
			formIDs = append(formIDs, id)
		}
	}
	sort.Strings(formIDs)
	keys, err := s.Forms.KeysFor(ctx, snap.WorkspaceID, snap.ID, formIDs)
	if err != nil {
		return builtSite{}, nil, fmt.Errorf("%w: %w", ErrFormsUnavailable, err)
	}
	origin, err := s.primaryOrigin(ctx, snap)
	if err != nil {
		return builtSite{}, nil, err
	}
	req := CandidateRequest{WorkspaceID: snap.WorkspaceID, ProjectID: snap.ID, Project: snap.Project, Settings: snap.Settings, FormKeys: keys, PrimaryOrigin: origin}
	candidate, err := s.candidate(ctx, req, snap.Project.Stack)
	if err != nil {
		return builtSite{}, nil, err
	}
	candidate, images := s.withSocialImages(ctx, req, candidate)
	out := builtSite{url: candidate.URL, files: map[string]sitepublish.FileEntry{}, bodies: map[string][]byte{}, routes: map[string]string{}, integrations: snap.Settings.Integrations.Enabled()}
	types := map[string]string{}
	for _, asset := range candidate.Assets {
		p := "/" + strings.TrimPrefix(asset.Path, "/")
		if !sitepublish.ValidServedPath(p) {
			return builtSite{}, nil, fmt.Errorf("%w: the builder produced the path %s", ErrBuildFailed, asset.Path)
		}
		out.bodies[p] = asset.Body
		types[p] = asset.ContentType
	}
	dropPrerender(out.bodies, candidate.PrerenderPath)
	for p, body := range images {
		out.bodies[p] = body
		types[p] = socialImageType
	}
	for p, body := range out.bodies {
		if strings.Contains(string(body), originPlaceholder) {
			return builtSite{}, nil, fmt.Errorf("%w: %s still has the site origin placeholder", ErrBuildFailed, p)
		}
	}
	routes, err := s.renderRoutes(ctx, snap.WorkspaceID, snap.ID, candidate, out.bodies)
	if err != nil {
		return builtSite{}, nil, err
	}
	if err := s.copyMedia(ctx, snap.WorkspaceID, out.bodies, types); err != nil {
		return builtSite{}, nil, err
	}
	pages := make([]site.RenderedPage, 0, len(routes))
	for _, r := range routes {
		doc, err := sitehtml.Parse(string(out.bodies[r.file]))
		if err != nil {
			return builtSite{}, nil, &IssuesFailure{Kind: ErrPublishBlocked, Issues: []site.Issue{{Code: site.CodeHTMLInvalid, Route: r.route, Detail: err.Error()}}}
		}
		pages = append(pages, site.RenderedPage{Route: r.route, Document: doc})
		out.routes[r.route] = r.file
	}
	checked := site.CheckPages(pages)
	if structural, _ := site.Split(checked.Issues); len(structural) > 0 {
		return builtSite{}, nil, &IssuesFailure{Kind: ErrPublishBlocked, Issues: structural}
	}
	gaps := checked.OpenGaps(pages, snap.Settings)
	if len(gaps) == 0 {
		if gaps, err = s.calendarGaps(ctx, snap.WorkspaceID, site.BookingIDs(pages)); err != nil {
			return builtSite{}, nil, err
		}
	}
	if len(gaps) > 0 {
		return builtSite{}, nil, &IssuesFailure{Kind: ErrGapsOpen, Issues: gaps}
	}
	manifests, bookings := withBookings(checked.Manifests, site.BookingIDs(pages), snap.Settings.Bookings)
	for _, m := range manifests {
		if keys[m.ID] == "" {
			return builtSite{}, nil, fmt.Errorf("%w: %s", ErrFormKeyMissing, m.ID)
		}
	}
	live, err := s.liveScan(ctx, candidate)
	if err != nil {
		return builtSite{}, nil, err
	}
	out.live = live
	destinations, err := s.destinations(ctx, snap, manifests)
	if err != nil {
		return builtSite{}, nil, err
	}
	out.destinations = destinations
	outbound := outboundTargets(slices.Concat(pages, live), destinations)
	widgets, err := s.widgetIDs(ctx, snap.WorkspaceID, sitepublish.WebchatWidgets(pages))
	if err != nil {
		return builtSite{}, nil, err
	}
	out.policy = site.ContentPolicyFor(out.integrations, s.cfg.APIOrigin, len(widgets) > 0)
	for p, body := range out.bodies {
		entry := sitepublish.NewFileEntry(body, contentTypeOf(p, types[p]), p)
		out.files[p] = entry
	}
	for _, r := range routes {
		if r.countView {
			entry := out.files[r.file]
			entry.CountView = true
			out.files[r.file] = entry
		}
	}
	rec.Files = out.files
	rec.Checks = site.RenderedChecks(pages)
	rec.Forms = sitepublish.FormsSnapshot{Manifests: manifests, Settings: formSettings(snap.Settings, manifests), Widgets: widgets, Bookings: bookings}
	rec.Redirects = snap.Settings.Redirects
	rec.Settings = snap.Settings
	rec.Outbound = outbound
	return out, pages, nil
}

func (s *Service) liveScan(ctx context.Context, candidate Candidate) ([]site.RenderedPage, error) {
	routes := make([]string, len(candidate.Routes))
	for i, r := range candidate.Routes {
		routes[i] = r.Route
	}
	rendered, err := s.Renderer.LiveDOM(ctx, candidate.URL, routes)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLiveScanFailed, err)
	}
	pages := make([]site.RenderedPage, 0, len(routes))
	for _, route := range routes {
		html, ok := rendered[route]
		if !ok || strings.TrimSpace(html) == "" {
			return nil, fmt.Errorf("%w: route %s", ErrLiveScanFailed, route)
		}
		doc, err := sitehtml.Parse(html)
		if err != nil {
			return nil, fmt.Errorf("%w: route %s: %w", ErrLiveScanFailed, route, err)
		}
		pages = append(pages, site.RenderedPage{Route: route, Document: doc})
	}
	if refused := liveRefusals(pages); len(refused) > 0 {
		return nil, &IssuesFailure{Kind: ErrPublishBlocked, Issues: refused}
	}
	return pages, nil
}

var liveRuleCodes = []string{site.CodeCredentialFieldForbidden, site.CodeFormActionForbidden}

func liveRefusals(pages []site.RenderedPage) []site.Issue {
	var refused []site.Issue
	for _, issue := range site.CheckPages(pages).Issues {
		if slices.Contains(liveRuleCodes, issue.Code) {
			refused = append(refused, issue)
		}
	}
	return refused
}

func (s *Service) destinations(ctx context.Context, snap ProjectSnapshot, manifests []siteform.Manifest) ([]string, error) {
	out := sitepublish.SettingsDestinations(snap.Settings)
	for _, m := range manifests {
		form, ok := snap.Settings.Forms[m.ID]
		if !ok || form.Success == nil || form.Success.Kind != site.SuccessWhatsApp {
			continue
		}
		link, err := s.Forms.WhatsAppLink(ctx, snap.ID, form.Success.Channel, "", "")
		if err != nil {
			return nil, fmt.Errorf("%w: form %s: %w", ErrFormsUnavailable, m.ID, err)
		}
		if !slices.Contains(out, link) {
			out = append(out, link)
		}
	}
	return out, nil
}

func (s *Service) calendarGaps(ctx context.Context, workspaceID string, bookings []string) ([]site.Issue, error) {
	if len(bookings) == 0 {
		return nil, nil
	}
	connected, err := s.Calendars.Connected(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("%w: the booking calendar could not be checked: %w", ErrFormsUnavailable, err)
	}
	if connected {
		return nil, nil
	}
	return []site.Issue{{Code: site.CodeBookingCalendarDisconnected, Detail: bookings[0]}}, nil
}

func withBookings(manifests []siteform.Manifest, ids []string, settings map[string]site.Booking) ([]siteform.Manifest, map[string]site.Booking) {
	out := slices.Clone(manifests)
	bookings := map[string]site.Booking{}
	for _, id := range ids {
		booking, ok := settings[id]
		if !ok {
			continue
		}
		out = append(out, booking.Manifest(id))
		bookings[id] = booking
	}
	if len(bookings) == 0 {
		return out, nil
	}
	return out, bookings
}

func formSettings(settings site.Settings, manifests []siteform.Manifest) map[string]site.FormSettings {
	out := map[string]site.FormSettings{}
	for _, m := range manifests {
		if f, ok := settings.Forms[m.ID]; ok {
			out[m.ID] = f
		}
	}
	return out
}

func contentTypeOf(p, declared string) string {
	if strings.TrimSpace(declared) != "" {
		return declared
	}
	if ct := mime.TypeByExtension(path.Ext(p)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

func (s *Service) widget(ctx context.Context, workspaceID, ref string) (*webchat.Widget, error) {
	widget, err := webchat.FindSiteWidget(ctx, s.Widgets, workspaceID, ref)
	if errors.Is(err, webchat.ErrWidgetNotFound) {
		return nil, fmt.Errorf("%w: %s", ErrWidgetUnknown, ref)
	}
	return widget, err
}

func (s *Service) widgetIDs(ctx context.Context, workspaceID string, refs []string) ([]string, error) {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		widget, err := s.widget(ctx, workspaceID, ref)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(ids, widget.ID) {
			ids = append(ids, widget.ID)
		}
	}
	return ids, nil
}

func (s *Service) linkWebchat(ctx context.Context, workspaceID, html string) (string, error) {
	return sitepublish.LinkWebchat(html, func(ref string) (string, bool, error) {
		widget, err := s.widget(ctx, workspaceID, ref)
		if err != nil {
			return "", false, err
		}
		return widget.PublicKey, true, nil
	})
}

func (s *Service) renderRoutes(ctx context.Context, workspaceID, projectID string, candidate Candidate, bodies map[string][]byte) ([]routeFile, error) {
	if len(candidate.Routes) == 0 {
		return nil, fmt.Errorf("%w: the build has no routes", ErrBuildFailed)
	}
	var markup map[string]string
	if candidate.PrerenderPath != "" {
		paths := make([]string, len(candidate.Routes))
		for i, r := range candidate.Routes {
			paths[i] = r.Route
		}
		rendered, err := s.Renderer.Prerender(ctx, candidate.URL, paths)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrPrerenderFailed, err)
		}
		markup = rendered
	}
	routes := make([]routeFile, 0, len(candidate.Routes))
	for _, r := range candidate.Routes {
		file := "/" + strings.TrimPrefix(r.HTMLPath, "/")
		body, ok := bodies[file]
		if !ok || !site.ValidRoutePath(r.Route) {
			return nil, fmt.Errorf("%w: route %s has no page", ErrBuildFailed, r.Route)
		}
		html := string(body)
		if markup != nil {
			inner, ok := markup[r.Route]
			if !ok {
				return nil, fmt.Errorf("%w: route %s", ErrPrerenderFailed, r.Route)
			}
			filled, err := sitepublish.FillPrerender(html, inner)
			if err != nil {
				return nil, fmt.Errorf("%w: route %s: %w", ErrPrerenderFailed, r.Route, err)
			}
			html = filled
		}
		html, err := s.linkWhatsApp(ctx, projectID, r.Route, html)
		if err != nil {
			return nil, err
		}
		if html, err = s.linkWebchat(ctx, workspaceID, html); err != nil {
			return nil, err
		}
		entry := routeFile{route: r.Route, file: file, interactive: r.Interactive}
		doc, err := sitehtml.Parse(html)
		if err != nil {
			return nil, &IssuesFailure{Kind: ErrPublishBlocked, Issues: []site.Issue{{Code: site.CodeHTMLInvalid, Route: r.Route, Detail: err.Error()}}}
		}
		if structural, _ := site.Split(site.CheckPages([]site.RenderedPage{{Route: r.Route, Document: doc}}).Issues); len(structural) > 0 {
			return nil, &IssuesFailure{Kind: ErrPublishBlocked, Issues: structural}
		}
		if !r.Interactive {
			keep := sitepublish.NeedsRuntime(doc)
			if html, err = sitepublish.StripAppBundle(html, keep); err != nil {
				return nil, &IssuesFailure{Kind: ErrPublishBlocked, Issues: []site.Issue{{Code: site.CodeHTMLInvalid, Route: r.Route, Detail: err.Error()}}}
			}
			entry.countView = !keep
		}
		bodies[file] = []byte(html)
		routes = append(routes, entry)
	}
	return routes, nil
}

func rewritable(contentType, p string) bool {
	ct := strings.ToLower(contentTypeOf(p, contentType))
	return strings.HasPrefix(ct, "text/html") || strings.HasPrefix(ct, "text/css")
}

func (s *Service) copyMedia(ctx context.Context, workspaceID string, bodies map[string][]byte, types map[string]string) error {
	copied := map[string]string{}
	for p, body := range bodies {
		if !rewritable(types[p], p) {
			continue
		}
		for _, ref := range sitepublish.AbsoluteURLs(string(body)) {
			if _, done := copied[ref]; done || !s.Media.Owns(ref) {
				continue
			}
			if err := s.publicMedia(ctx, workspaceID, ref); err != nil {
				return err
			}
			data, contentType, err := s.Media.Fetch(ctx, ref)
			if err != nil {
				return fmt.Errorf("%w: %s: %w", ErrMediaUnavailable, ref, err)
			}
			served := sitepublish.MediaPathPrefix + sitepublish.SHA256Hex(data) + mediaExtension(ref, contentType)
			bodies[served] = data
			types[served] = contentType
			copied[ref] = served
		}
	}
	if len(copied) == 0 {
		return nil
	}
	refs := make([]string, 0, len(copied))
	for ref := range copied {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool { return len(refs[i]) > len(refs[j]) })
	for p, body := range bodies {
		if !rewritable(types[p], p) {
			continue
		}
		text := string(body)
		for _, ref := range refs {
			text = strings.ReplaceAll(text, ref, copied[ref])
		}
		bodies[p] = []byte(text)
	}
	return nil
}

func mediaExtension(ref, contentType string) string {
	clean := strings.SplitN(strings.SplitN(ref, "?", 2)[0], "#", 2)[0]
	if ext := strings.ToLower(path.Ext(clean)); len(ext) > 1 && len(ext) <= 6 && strings.Trim(ext[1:], "abcdefghijklmnopqrstuvwxyz0123456789") == "" {
		return ext
	}
	if exts, err := mime.ExtensionsByType(contentType); err == nil && len(exts) > 0 {
		return exts[0]
	}
	return ""
}

func outboundTargets(pages []site.RenderedPage, destinations []string) []string {
	targets := sitepublish.OutboundURLs(pages)
	for _, target := range destinations {
		if !slices.Contains(targets, target) {
			targets = append(targets, target)
		}
	}
	return targets
}

func (s *Service) upload(ctx context.Context, rec *sitepublish.DeploymentRecord, built builtSite) error {
	done := map[string]bool{}
	for p, entry := range built.files {
		if done[entry.SHA256] {
			continue
		}
		done[entry.SHA256] = true
		has, err := s.Hosting.HasBlob(ctx, entry.SHA256)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrHostingUnavailable, err)
		}
		if has {
			continue
		}
		if err := s.Hosting.PutBlob(ctx, entry.SHA256, entry.ContentType, built.bodies[p]); err != nil {
			return fmt.Errorf("%w: %w", ErrHostingUnavailable, err)
		}
	}
	manifest := sitepublish.DeploymentManifest{
		DeploymentID: rec.ID, ProjectID: rec.ProjectID, APIOrigin: s.cfg.APIOrigin, Files: built.files, Routes: built.routes, Integrations: built.integrations, Policy: built.policy,
	}
	if _, ok := built.files["/404.html"]; ok {
		manifest.NotFound = "/404.html"
	}
	body, err := manifest.Encode()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrBuildFailed, err)
	}
	if err := s.Hosting.PutManifest(ctx, rec.ID, body); err != nil {
		return fmt.Errorf("%w: %w", ErrHostingUnavailable, err)
	}
	return nil
}

func (s *Service) publicMedia(ctx context.Context, workspaceID, ref string) error {
	row, err := s.Media.Lookup(ctx, workspaceID, ref)
	if errors.Is(err, media.ErrMediaNotFound) {
		return fmt.Errorf("%w: %s", ErrMediaRefused, ref)
	}
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrMediaUnavailable, ref, err)
	}
	if row == nil || row.WorkspaceID != workspaceID || row.URL != ref || row.Type.Private() {
		return fmt.Errorf("%w: %s", ErrMediaRefused, ref)
	}
	return nil
}
