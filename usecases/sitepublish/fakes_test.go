package sitepublish_usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"vozko/domain/media"
	"vozko/domain/notification"
	"vozko/domain/site"
	"vozko/domain/sitepublish"
	"vozko/domain/webchat"
	"vozko/domain/workspace"
	workspace_addon "vozko/domain/workspace/workspace_addon"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

const (
	wsID      = "ws-1"
	userID    = "user-1"
	projectID = "8f6c2a52-1b0f-4f7e-9a34-0d5f8f1e2c3b"
)

var actor = Actor{UserID: userID, WorkspaceID: wsID}

type publication struct {
	state        Publication
	deploymentID string
}

type fakeProjects struct {
	snap         ProjectSnapshot
	err          error
	recordErr    error
	editedTo     int64
	described    []string
	published    []PublishRecord
	publications []publication
}

func (f *fakeProjects) SetPublication(_ context.Context, workspaceID, id string, state Publication, deploymentID string) error {
	if workspaceID != f.snap.WorkspaceID || id != f.snap.ID {
		return ErrProjectNotFound
	}
	f.publications = append(f.publications, publication{state: state, deploymentID: deploymentID})
	return nil
}

func (f *fakeProjects) last() publication {
	if len(f.publications) == 0 {
		return publication{}
	}
	return f.publications[len(f.publications)-1]
}

func (f *fakeProjects) Describe(ctx context.Context, workspaceID, id string) (ProjectSnapshot, error) {
	f.described = append(f.described, workspaceID+"|"+id)
	return f.Working(ctx, workspaceID, id)
}

func (f *fakeProjects) Working(_ context.Context, workspaceID, id string) (ProjectSnapshot, error) {
	if f.err != nil {
		return ProjectSnapshot{}, f.err
	}
	if workspaceID != f.snap.WorkspaceID || id != f.snap.ID {
		return ProjectSnapshot{}, ErrProjectNotFound
	}
	snap := f.snap
	snap.Settings = site.Settings{}
	return snap, nil
}

func (f *fakeProjects) RecordPublish(ctx context.Context, in PublishRecord) (PublishRecorded, error) {
	if f.recordErr != nil {
		return PublishRecorded{}, f.recordErr
	}
	f.published = append(f.published, in)
	if err := f.SetPublication(ctx, in.WorkspaceID, in.ProjectID, PublicationLive, in.DeploymentID); err != nil {
		return PublishRecorded{}, err
	}
	if f.editedTo > 0 {
		return PublishRecorded{RevisionID: "rev-publish-" + in.DeploymentID, Version: f.editedTo}, nil
	}
	return PublishRecorded{RevisionID: "rev-publish-" + in.DeploymentID, Version: in.SourceVersion, Current: true}, nil
}

type fakeWorkspaces struct{}

func (fakeWorkspaces) WorkspaceName(context.Context, string) (string, error) {
	return "Loja da Ana", nil
}

type fakeBuilder struct {
	candidate Candidate
	err       error
	requests  []CandidateRequest
}

func (f *fakeBuilder) Candidate(_ context.Context, req CandidateRequest) (Candidate, error) {
	f.requests = append(f.requests, req)
	return f.candidate, f.err
}

type fakeRenderer struct {
	markup  map[string]string
	err     error
	calls   int
	live    map[string]string
	liveErr error
	scans   []string
	og      []byte
	ogErr   error
	shots   []string
}

func (f *fakeRenderer) OpenGraph(_ context.Context, url string) ([]byte, error) {
	f.shots = append(f.shots, url)
	if f.ogErr != nil {
		return nil, f.ogErr
	}
	if f.og == nil {
		return nil, errors.New("no open graph image in this test")
	}
	return f.og, nil
}

type thumbnailCall struct {
	workspaceID string
	projectID   string
	url         string
}

type fakeThumbnails struct {
	published []thumbnailCall
}

func (f *fakeThumbnails) Published(_ context.Context, workspaceID, projectID, url string) {
	f.published = append(f.published, thumbnailCall{workspaceID: workspaceID, projectID: projectID, url: url})
}

func (f *fakeRenderer) Prerender(_ context.Context, url string, routes []string) (map[string]string, error) {
	f.calls++
	return f.markup, f.err
}

type fakeForms struct {
	keys        map[string]string
	activated   []ActivateForms
	deactivated []string
	links       []string
	linkErr     error
	err         error
}

func (f *fakeForms) Deactivate(_ context.Context, projectID string) error {
	if f.err != nil {
		return f.err
	}
	f.deactivated = append(f.deactivated, projectID)
	return nil
}

func (f *fakeForms) WhatsAppLink(_ context.Context, projectID, numberRef, text, route string) (string, error) {
	f.links = append(f.links, projectID+"|"+numberRef+"|"+text+"|"+route)
	if f.linkErr != nil {
		return "", f.linkErr
	}
	return "https://vzk.to/" + numberRef, nil
}

type fakeWidgets struct {
	rows []webchat.Widget
	err  error
}

func (f *fakeWidgets) FindByPublicKey(_ context.Context, key string) (*webchat.Widget, error) {
	if f.err != nil {
		return nil, f.err
	}
	for _, w := range f.rows {
		if w.PublicKey == key {
			return &w, nil
		}
	}
	return nil, webchat.ErrWidgetNotFound
}

func (f *fakeWidgets) FindByID(_ context.Context, workspaceID, id string) (*webchat.Widget, error) {
	if f.err != nil {
		return nil, f.err
	}
	for _, w := range f.rows {
		if w.ID == id && w.WorkspaceID == workspaceID {
			return &w, nil
		}
	}
	return nil, webchat.ErrWidgetNotFound
}

func (f *fakeForms) KeysFor(_ context.Context, _, _ string, ids []string) (map[string]string, error) {
	out := map[string]string{}
	for _, id := range ids {
		if key, ok := f.keys[id]; ok {
			out[id] = key
		}
	}
	return out, nil
}

func (f *fakeForms) Activate(_ context.Context, in ActivateForms) error {
	if f.err != nil {
		return f.err
	}
	f.activated = append(f.activated, in)
	return nil
}

type fakeHosting struct {
	mu          sync.Mutex
	blobs       map[string][]byte
	puts        []string
	manifests   map[string][]byte
	hosts       map[string][]byte
	deployments map[string][]byte
	deleted     []string
	dropped     []string
	err         error
}

func newHosting() *fakeHosting {
	return &fakeHosting{blobs: map[string][]byte{}, manifests: map[string][]byte{}, hosts: map[string][]byte{}, deployments: map[string][]byte{}}
}

func (f *fakeHosting) HasBlob(_ context.Context, sha string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	_, ok := f.blobs[sha]
	return ok, nil
}

func (f *fakeHosting) PutBlob(_ context.Context, sha, _ string, body []byte) error {
	if sitepublish.SHA256Hex(body) != sha {
		return errors.New("bad address")
	}
	f.blobs[sha] = body
	f.puts = append(f.puts, sha)
	return nil
}

func (f *fakeHosting) PutManifest(_ context.Context, id string, body []byte) error {
	f.manifests[id] = body
	return nil
}

func (f *fakeHosting) PutHost(_ context.Context, host string, value []byte) error {
	if f.err != nil {
		return f.err
	}
	f.hosts[host] = value
	return nil
}

func (f *fakeHosting) DeleteHost(_ context.Context, host string) error {
	if f.err != nil {
		return f.err
	}
	delete(f.hosts, host)
	f.deleted = append(f.deleted, host)
	return nil
}

func (f *fakeHosting) PutDeployment(_ context.Context, id string, value []byte) error {
	if f.err != nil {
		return f.err
	}
	f.deployments[id] = value
	return nil
}

func (f *fakeHosting) serving(t *testing.T, host string) sitepublish.ServingSettings {
	t.Helper()
	raw, ok := f.hosts[host]
	if !ok {
		t.Fatalf("no kv key for %s (have %v)", host, keys(f.hosts))
	}
	s, err := sitepublish.DecodeServing(raw)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

type fakeMedia struct {
	files map[string][]byte
	rows  map[string]media.Media
	err   error
}

func (f *fakeMedia) Owns(raw string) bool { return strings.HasPrefix(raw, "https://cdn.vozko.test/") }

func (f *fakeMedia) Fetch(_ context.Context, raw string) ([]byte, string, error) {
	if f.err != nil {
		return nil, "", f.err
	}
	body, ok := f.files[raw]
	if !ok {
		return nil, "", errors.New("missing")
	}
	return body, "image/webp", nil
}

type fakeCloudflare struct {
	states  map[string]sitepublish.CustomHostnameState
	created []string
	deleted []string
	err     error
	getErr  error
}

func (f *fakeCloudflare) Create(_ context.Context, host string) (sitepublish.CustomHostnameState, error) {
	if f.err != nil {
		return sitepublish.CustomHostnameState{}, f.err
	}
	f.created = append(f.created, host)
	state := sitepublish.CustomHostnameState{Ref: "cf-" + host, Hostname: host, Phase: sitepublish.HostnamePendingDNS, SSLStatus: "pending_validation",
		Records: []sitepublish.DNSRecord{{Type: "TXT", Name: "_cf-custom-hostname." + host, Value: "own"}}}
	f.states[state.Ref] = state
	return state, nil
}

func (f *fakeCloudflare) Get(_ context.Context, ref string) (sitepublish.CustomHostnameState, error) {
	if f.getErr != nil {
		return sitepublish.CustomHostnameState{}, f.getErr
	}
	state, ok := f.states[ref]
	if !ok {
		return sitepublish.CustomHostnameState{}, sitepublish.ErrHostnameNotFound
	}
	return state, nil
}

func (f *fakeCloudflare) Delete(_ context.Context, ref string) error {
	if f.err != nil {
		return f.err
	}
	f.deleted = append(f.deleted, ref)
	delete(f.states, ref)
	return nil
}

type fakeAccess struct{ denied map[string]bool }

func (f fakeAccess) Execute(_, _ string, resource workspace.Resource, action workspace.Action) error {
	if f.denied[string(resource)+":"+string(action)] {
		return errors.New("denied")
	}
	return nil
}

type fakeSubscriptions struct{ err error }

func (f *fakeSubscriptions) Execute(string) (*workspace_plan.WorkspaceSubscription, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &workspace_plan.WorkspaceSubscription{Status: workspace_plan.SubscriptionStatusActive}, nil
}

type fakeEntitlements struct {
	allowed int
	err     error
}

func (f *fakeEntitlements) Resolve(string, workspace_addon.EntitlementKind) (int, error) {
	return f.allowed, f.err
}

type fakeDNS struct {
	txt map[string][]string
	err error
}

func (f *fakeDNS) LookupTXT(_ context.Context, name string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.txt[name], nil
}

func (f *fakeDNS) prove(rec sitepublish.HostnameRecord) {
	own := sitepublish.OwnershipRecord(rec.Hostname, rec.Verification.OwnershipToken)
	f.txt[own.Name] = append(f.txt[own.Name], own.Value)
}

type fakeReversals struct {
	open bool
	err  error
}

func (f *fakeReversals) HasOpenReversal(context.Context, string) (bool, error) {
	return f.open, f.err
}

type memDeployments struct {
	rows        map[string]sitepublish.DeploymentRecord
	projects    *fakeProjects
	abuse       *memAbuse
	statusAfter []sitepublish.DeploymentStatus
	recordErr   error
	seq         []string
	activateErr error
}

func (m *memDeployments) Create(_ context.Context, rec sitepublish.DeploymentRecord) error {
	m.rows[rec.ID] = rec
	m.seq = append(m.seq, rec.ID)
	return nil
}

func (m *memDeployments) Update(_ context.Context, rec sitepublish.DeploymentRecord) error {
	previous, ok := m.rows[rec.ID]
	if !ok {
		return sitepublish.ErrDeploymentNotFound
	}
	if previous.Status == sitepublish.DeploymentActive || previous.Status == sitepublish.DeploymentSuperseded {
		m.statusAfter = append(m.statusAfter, rec.Status)
	}
	m.rows[rec.ID] = rec
	return nil
}

func (m *memDeployments) Get(_ context.Context, workspaceID, id string) (sitepublish.DeploymentRecord, error) {
	rec, ok := m.rows[id]
	if !ok || rec.WorkspaceID != workspaceID {
		return sitepublish.DeploymentRecord{}, sitepublish.ErrDeploymentNotFound
	}
	return rec, nil
}

func (m *memDeployments) Active(_ context.Context, project string) (sitepublish.DeploymentRecord, bool, error) {
	for _, id := range m.seq {
		if rec := m.rows[id]; rec.ProjectID == project && rec.Status == sitepublish.DeploymentActive {
			return rec, true, nil
		}
	}
	return sitepublish.DeploymentRecord{}, false, nil
}

func (m *memDeployments) Latest(_ context.Context, project string) (sitepublish.DeploymentRecord, error) {
	for i := len(m.seq) - 1; i >= 0; i-- {
		if rec := m.rows[m.seq[i]]; rec.ProjectID == project {
			return rec, nil
		}
	}
	return sitepublish.DeploymentRecord{}, sitepublish.ErrDeploymentNotFound
}

func (m *memDeployments) ListByProject(_ context.Context, workspaceID, project string, _ int) ([]sitepublish.DeploymentRecord, error) {
	var out []sitepublish.DeploymentRecord
	for i := len(m.seq) - 1; i >= 0; i-- {
		if rec := m.rows[m.seq[i]]; rec.ProjectID == project && rec.WorkspaceID == workspaceID {
			out = append(out, rec)
		}
	}
	return out, nil
}

func (m *memDeployments) ListActiveByWorkspace(_ context.Context, workspaceID string) ([]sitepublish.DeploymentRecord, error) {
	var out []sitepublish.DeploymentRecord
	for _, id := range m.seq {
		if rec := m.rows[id]; rec.WorkspaceID == workspaceID && rec.Status == sitepublish.DeploymentActive {
			out = append(out, rec)
		}
	}
	return out, nil
}

func (m *memDeployments) ListServable(_ context.Context, project string) ([]sitepublish.DeploymentRecord, error) {
	var out []sitepublish.DeploymentRecord
	for _, id := range m.seq {
		rec := m.rows[id]
		if rec.ProjectID == project && (rec.Status == sitepublish.DeploymentReady || rec.Status == sitepublish.DeploymentActive || rec.Status == sitepublish.DeploymentSuperseded) {
			out = append(out, rec)
		}
	}
	return out, nil
}

func (m *memDeployments) Activate(_ context.Context, project, id, by string, at time.Time) error {
	if m.activateErr != nil {
		return m.activateErr
	}
	target, ok := m.rows[id]
	if !ok || (target.Status != sitepublish.DeploymentReady && target.Status != sitepublish.DeploymentSuperseded) {
		return sitepublish.ErrDeploymentTransition
	}
	for key, rec := range m.rows {
		if rec.ProjectID == project && rec.Status == sitepublish.DeploymentActive {
			rec.Status = sitepublish.DeploymentSuperseded
			m.rows[key] = rec
		}
	}
	target.Status, target.PublishedBy, target.PublishedAt = sitepublish.DeploymentActive, by, &at
	m.rows[id] = target
	return nil
}

type memHostnames struct {
	rows map[string]sitepublish.HostnameRecord
}

func (m *memHostnames) Create(_ context.Context, rec sitepublish.HostnameRecord) error {
	for _, r := range m.rows {
		if r.Hostname == rec.Hostname {
			return sitepublish.ErrHostnameTaken
		}
	}
	m.rows[rec.ID] = rec
	return nil
}

func (m *memHostnames) Update(_ context.Context, rec sitepublish.HostnameRecord) error {
	m.rows[rec.ID] = rec
	return nil
}

func (m *memHostnames) Delete(_ context.Context, id string) error {
	delete(m.rows, id)
	return nil
}

func (m *memHostnames) Get(_ context.Context, workspaceID, id string) (sitepublish.HostnameRecord, error) {
	rec, ok := m.rows[id]
	if !ok || rec.WorkspaceID != workspaceID {
		return sitepublish.HostnameRecord{}, sitepublish.ErrHostnameNotFound
	}
	return rec, nil
}

func (m *memHostnames) sorted() []sitepublish.HostnameRecord {
	out := make([]sitepublish.HostnameRecord, 0, len(m.rows))
	for _, r := range m.rows {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Hostname < out[j].Hostname })
	return out
}

func (m *memHostnames) ListByProject(_ context.Context, workspaceID, project string) ([]sitepublish.HostnameRecord, error) {
	var out []sitepublish.HostnameRecord
	for _, r := range m.sorted() {
		if r.WorkspaceID == workspaceID && r.ProjectID == project {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *memHostnames) ListByStatus(_ context.Context, statuses []sitepublish.HostnameStatus, kind sitepublish.HostnameKind, limit int) ([]sitepublish.HostnameRecord, error) {
	var out []sitepublish.HostnameRecord
	for _, r := range m.sorted() {
		for _, s := range statuses {
			if r.Status == s && r.Kind == kind {
				out = append(out, r)
			}
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *memHostnames) ListPage(_ context.Context, page sitepublish.HostnamePage) ([]sitepublish.HostnameRecord, error) {
	var out []sitepublish.HostnameRecord
	for _, r := range m.rows {
		if slices.Contains(page.Statuses, r.Status) && r.Kind == page.Kind && r.ID > page.AfterID && (!page.HoldingProviderRecord || r.CloudflareRef != "") {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if len(out) > page.Limit {
		out = out[:page.Limit]
	}
	return out, nil
}

func (m *memHostnames) ActiveHostnames(_ context.Context, project string) ([]string, error) {
	var out []string
	for _, r := range m.sorted() {
		if r.ProjectID == project && r.Status == sitepublish.HostnameActive {
			out = append(out, r.Hostname)
		}
	}
	return out, nil
}

func (m *memHostnames) CountCustom(_ context.Context, workspaceID string) (int, error) {
	n := 0
	for _, r := range m.rows {
		if r.WorkspaceID == workspaceID && r.Kind == sitepublish.HostnameCustom && r.Status != sitepublish.HostnameRemoving {
			n++
		}
	}
	return n, nil
}

type memAbuse struct {
	rows []sitepublish.AbuseCase
}

func (m *memAbuse) Create(_ context.Context, c sitepublish.AbuseCase) error {
	m.rows = append(m.rows, c)
	return nil
}

func (m *memAbuse) Update(_ context.Context, c sitepublish.AbuseCase) error {
	for i := range m.rows {
		if m.rows[i].ID == c.ID {
			m.rows[i] = c
			return nil
		}
	}
	return sitepublish.ErrAbuseCaseNotFound
}

func (m *memAbuse) List(context.Context, []sitepublish.AbuseStatus, int) ([]sitepublish.AbuseCase, error) {
	return m.rows, nil
}

func (m *memAbuse) OpenForProject(_ context.Context, project string) ([]sitepublish.AbuseCase, error) {
	var out []sitepublish.AbuseCase
	for _, c := range m.rows {
		if c.ProjectID == project && c.Status.Open() {
			out = append(out, c)
		}
	}
	return out, nil
}

func (m *memAbuse) Suspended(_ context.Context, project string) (bool, error) {
	for _, c := range m.rows {
		if c.ProjectID == project && c.Status == sitepublish.AbuseSuspended {
			return true, nil
		}
	}
	return false, nil
}

type fakeAlerts struct {
	published []notification.Draft
	resolved  []string
}

func (f *fakeAlerts) Publish(_ context.Context, d notification.Draft) (notification.Record, error) {
	f.published = append(f.published, d)
	return notification.Record{}, nil
}

func (f *fakeAlerts) Resolve(_ context.Context, _ string, keys ...string) error {
	f.resolved = append(f.resolved, keys...)
	return nil
}

type rig struct {
	svc         *Service
	projects    *fakeProjects
	builder     *fakeBuilder
	renderer    *fakeRenderer
	forms       *fakeForms
	hosting     *fakeHosting
	media       *fakeMedia
	cloudflare  *fakeCloudflare
	access      fakeAccess
	subs        *fakeSubscriptions
	widgets     *fakeWidgets
	deployments *memDeployments
	hostnames   *memHostnames
	abuse       *memAbuse
	alerts      *fakeAlerts
	thumbnails  *fakeThumbnails
	locks       *fakeLocks
	counters    *fakeCounters
	dns         *fakeDNS
	reversals   *fakeReversals
	entitled    *fakeEntitlements
	calendars   *fakeCalendars
}

const homeHTML = `<!doctype html><html lang="pt-BR" data-vz-api="https://api.vozko.test"><head><title>Loja da Ana</title><script type="module" src="/assets/index-5XKZ3QJ2.js"></script><script type="module" src="/assets/vz-runtime-ABCD1234.js"></script></head><body><h1>Bolos caseiros</h1><img src="https://cdn.vozko.test/media/bolo.webp" alt="Bolo de cenoura"><a href="https://wa.me/5511999999999">Fale conosco</a></body></html>`

const formHTML = `<!doctype html><html lang="pt-BR"><head><title>Contato</title><script type="module" src="/assets/index-5XKZ3QJ2.js"></script><script type="module" src="/assets/vz-runtime-ABCD1234.js"></script></head><body><h1>Contato</h1><form data-vz-form="contato"><input name="lead.phone" type="tel" required><label><input type="checkbox" name="consent" required> Aceito a <a href="/privacidade">política de privacidade</a></label><button type="submit">Enviar</button></form></body></html>`

func htmlCandidate(home string) Candidate {
	return Candidate{
		URL: "https://cand.preview.test/",
		Assets: []BuildAsset{
			{Path: "index.html", ContentType: "text/html; charset=utf-8", Body: []byte(home)},
			{Path: "assets/index-5XKZ3QJ2.js", ContentType: "text/javascript", Body: []byte("console.log(1)")},
			{Path: "assets/vz-runtime-ABCD1234.js", ContentType: "text/javascript", Body: []byte("runtime()")},
			{Path: "assets/app-AB12CD34.css", ContentType: "text/css", Body: []byte(`.hero{background:url("https://cdn.vozko.test/media/bolo.webp")}`)},
		},
		Imports: []site.Import{{Importer: "scripts/main.js", Specifier: "./util.js"}},
		Routes:  []BuildRoute{{Route: "/", HTMLPath: "index.html"}},
	}
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{
		projects: &fakeProjects{snap: ProjectSnapshot{
			ID: projectID, WorkspaceID: wsID, Name: "Bolos", Slug: "bolos", RevisionID: "rev-7", Version: 12,
			Project: site.NewProject(site.StackHTML, map[string]string{"index.html": "<h1>Bolos</h1>"}),
		}},
		builder:    &fakeBuilder{candidate: htmlCandidate(homeHTML)},
		renderer:   &fakeRenderer{},
		forms:      &fakeForms{keys: map[string]string{"contato": "fk_contato"}},
		hosting:    newHosting(),
		media:      &fakeMedia{files: map[string][]byte{"https://cdn.vozko.test/media/bolo.webp": []byte("webp-bytes")}, rows: map[string]media.Media{"https://cdn.vozko.test/media/bolo.webp": {ID: "m-1", WorkspaceID: wsID, URL: "https://cdn.vozko.test/media/bolo.webp", Type: media.MediaTypeProductImage}}},
		cloudflare: &fakeCloudflare{states: map[string]sitepublish.CustomHostnameState{}},
		access:     fakeAccess{denied: map[string]bool{}},
		subs:       &fakeSubscriptions{},
		widgets: &fakeWidgets{rows: []webchat.Widget{
			{ID: "6b1f6d2e-3c4a-4f5b-8e9d-0a1b2c3d4e5f", WorkspaceID: wsID, PublicKey: "pk_chat"},
			{ID: "7c2a7e3f-4d5b-4a6c-9fae-1b2c3d4e5f60", WorkspaceID: "ws-other", PublicKey: "pk_other"},
		}},
		deployments: &memDeployments{rows: map[string]sitepublish.DeploymentRecord{}},
		hostnames:   &memHostnames{rows: map[string]sitepublish.HostnameRecord{}},
		abuse:       &memAbuse{},
		alerts:      &fakeAlerts{},
		thumbnails:  &fakeThumbnails{},
		locks:       &fakeLocks{held: map[string]string{}},
		counters:    &fakeCounters{counts: map[string]int64{}},
		dns:         &fakeDNS{txt: map[string][]string{}},
		reversals:   &fakeReversals{},
		calendars:   &fakeCalendars{connected: true},
	}
	r.deployments.projects, r.deployments.abuse = r.projects, r.abuse
	r.build(t, 10)
	return r
}

func (r *rig) build(t *testing.T, allowedDomains int) {
	t.Helper()
	n := 0
	r.entitled = &fakeEntitlements{allowed: allowedDomains}
	svc, err := New(Deps{
		Projects: r.projects, Workspaces: fakeWorkspaces{}, Builder: r.builder, Renderer: r.renderer, Forms: r.forms,
		Hosting: r.hosting, Media: r.media, CustomHostnames: r.cloudflare, Access: r.access, Subscriptions: r.subs,
		Entitlements: r.entitled, DNS: r.dns, Reversals: r.reversals, Widgets: r.widgets, Deployments: r.deployments,
		Hostnames: r.hostnames, Abuse: r.abuse, Alerts: r.alerts, Thumbnails: r.thumbnails, Locks: r.locks,
		Counters: r.counters, Calendars: r.calendars,
		Now:   func() time.Time { return time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC) },
		NewID: func() string { n++; return fmt.Sprintf("00000000-0000-4000-8000-%012d", n) },
	}, Config{
		Addresses: sitepublish.Addresses{Domain: "paginas.test"}, APIOrigin: "https://api.vozko.test", CNAMETarget: "sites.paginas.test",
		ApplicationDomains: []string{"vozko.test"}, BrandWords: []string{"Vozko"},
	})
	if err != nil {
		t.Fatal(err)
	}
	r.svc = svc
}

func (f *fakeRenderer) LiveDOM(_ context.Context, url string, routes []string) (map[string]string, error) {
	f.scans = append(f.scans, url)
	if f.liveErr != nil {
		return nil, f.liveErr
	}
	out := map[string]string{}
	for _, route := range routes {
		if html, ok := f.live[route]; ok {
			out[route] = html
			continue
		}
		if f.live == nil {
			out[route] = `<!doctype html><html lang="pt-BR"><head><title>Loja da Ana</title></head><body><h1>Bolos caseiros</h1></body></html>`
		}
	}
	return out, nil
}

func (f *fakeHosting) DeleteDeployment(_ context.Context, id string) error {
	if f.err != nil {
		return f.err
	}
	delete(f.deployments, id)
	f.dropped = append(f.dropped, id)
	return nil
}

func (f *fakeMedia) Lookup(_ context.Context, workspaceID, raw string) (*media.Media, error) {
	row, ok := f.rows[raw]
	if !ok || row.WorkspaceID != workspaceID {
		return nil, media.ErrMediaNotFound
	}
	return &row, nil
}

type fakeLocks struct {
	held     map[string]string
	err      error
	acquired []string
}

func (f *fakeLocks) SetNX(key, value string, _ time.Duration) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	if _, taken := f.held[key]; taken {
		return false, nil
	}
	f.held[key] = value
	f.acquired = append(f.acquired, key)
	return true, nil
}

type fakeCounters struct {
	counts map[string]int64
	err    error
}

func (f *fakeCounters) IncrWithTTL(key string, _ time.Duration) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.counts[key]++
	return f.counts[key], nil
}

func (f *fakeCounters) Del(keys ...string) error {
	if f.err != nil {
		return f.err
	}
	for _, k := range keys {
		delete(f.counts, k)
	}
	return nil
}

func (f *fakeLocks) Del(keys ...string) error {
	for _, k := range keys {
		delete(f.held, k)
	}
	return nil
}

func (m *memDeployments) RecordPublished(_ context.Context, project, id, revisionID string) error {
	if m.recordErr != nil {
		return m.recordErr
	}
	rec, ok := m.rows[id]
	if !ok || rec.ProjectID != project {
		return sitepublish.ErrDeploymentNotFound
	}
	rec.RevisionID = revisionID
	m.rows[id] = rec
	return nil
}

func (m *memDeployments) LiveProjects(_ context.Context, after string, limit int) ([]sitepublish.LiveProject, error) {
	var out []sitepublish.LiveProject
	for _, id := range m.seq {
		rec := m.rows[id]
		if rec.Status != sitepublish.DeploymentActive || rec.ProjectID <= after {
			continue
		}
		live := sitepublish.LiveProject{WorkspaceID: rec.WorkspaceID, ProjectID: rec.ProjectID, DeploymentID: rec.ID, ProjectStatus: string(m.projects.last().state)}
		live.AbuseSuspended, _ = m.abuse.Suspended(context.Background(), rec.ProjectID)
		out = append(out, live)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ProjectID < out[j].ProjectID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *memHostnames) CreateCustom(ctx context.Context, rec sitepublish.HostnameRecord, allowed int) error {
	used, _ := m.CountCustom(ctx, rec.WorkspaceID)
	if used >= allowed {
		return sitepublish.ErrDomainQuota
	}
	return m.Create(ctx, rec)
}

func (m *memHostnames) ListCustomByWorkspace(_ context.Context, workspaceID string) ([]sitepublish.HostnameRecord, error) {
	var out []sitepublish.HostnameRecord
	for _, r := range m.sorted() {
		if r.WorkspaceID == workspaceID && r.Kind == sitepublish.HostnameCustom {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *memHostnames) CustomWorkspaces(_ context.Context, after string, limit int) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, r := range m.rows {
		if r.Kind == sitepublish.HostnameCustom && r.Status != sitepublish.HostnameRemoving && r.WorkspaceID > after && !seen[r.WorkspaceID] {
			seen[r.WorkspaceID] = true
			out = append(out, r.WorkspaceID)
		}
	}
	sort.Strings(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
