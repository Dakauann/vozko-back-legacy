package container

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	ads "vozko/domain/advertising"
	"vozko/domain/conversation"
	"vozko/domain/emailconsent"
	"vozko/domain/lead"
	"vozko/domain/media"
	"vozko/domain/notification"
	"vozko/domain/opportunity"
	"vozko/domain/shared"
	"vozko/domain/shortlink"
	"vozko/domain/site"
	"vozko/domain/sitepublish"
	"vozko/domain/sitesubmission"
	"vozko/domain/webchat"
	wo "vozko/domain/whatsapp_outreach"
	"vozko/domain/workspace"
	workspace_addon "vozko/domain/workspace/workspace_addon"
	workspace_plan "vozko/domain/workspace/workspace_plan"
	emailconsent_usecase "vozko/usecases/emailconsent"
	"vozko/usecases/emailinbox"
	"vozko/usecases/formlead"
	lead_usecase "vozko/usecases/lead"
	opportunity_usecase "vozko/usecases/opportunity"
	site_usecase "vozko/usecases/site"
	"vozko/usecases/sitebuild"
	"vozko/usecases/siteforms"
	"vozko/usecases/siterender"
	uwuc "vozko/usecases/unofficial_whatsapp"
)

type recordedAsset struct {
	Path        string `json:"path"`
	ContentType string `json:"content_type"`
	Body        string `json:"body"`
	Stub        bool   `json:"stub"`
}

type recordedRoute struct {
	Route       string `json:"route"`
	HTMLPath    string `json:"html_path"`
	Interactive bool   `json:"interactive"`
}

type recordedMode struct {
	Routes        []recordedRoute   `json:"routes"`
	PrerenderPath string            `json:"prerender_path"`
	Imports       []site.Import     `json:"imports"`
	Assets        []recordedAsset   `json:"assets"`
	Rendered      map[string]string `json:"rendered"`
}

type recordedBuild struct {
	Starter string            `json:"starter"`
	FormKey string            `json:"form_key"`
	API     string            `json:"api"`
	Edits   json.RawMessage   `json:"edits"`
	Sources map[string]string `json:"sources"`
	Preview recordedMode      `json:"preview"`
	Publish recordedMode      `json:"publish"`
}

func loadRecordedBuild(t *testing.T) recordedBuild {
	t.Helper()
	raw, err := os.ReadFile("testdata/site_react_lead_capture_build.json")
	if err != nil {
		t.Fatal(err)
	}
	var rec recordedBuild
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

type replayBuilder struct {
	rec      recordedBuild
	mu       sync.Mutex
	builds   []sitebuild.BuildRequest
	extracts []sitebuild.ExtractRequest
	executes []sitebuild.ExecuteRequest
}

func (b *replayBuilder) Execute(_ context.Context, req sitebuild.ExecuteRequest) (sitebuild.ExecuteResult, error) {
	b.mu.Lock()
	b.executes = append(b.executes, req)
	b.mu.Unlock()
	var steps []site.StepPatch
	files := map[string]string{}
	for path, f := range req.Project.Files {
		files[path] = f.Content
	}
	for i, op := range req.Operations {
		if op.Op != site.OpEditFile || op.New == nil || !strings.Contains(files[op.Path], op.Old) {
			return sitebuild.ExecuteResult{Failures: []sitebuild.ExecuteFailure{{Index: i, Code: site.CodeEditNotFound, Field: "old"}}}, nil
		}
		n := 1
		if op.ReplaceAll {
			n = -1
		}
		files[op.Path] = strings.Replace(files[op.Path], op.Old, *op.New, n)
		steps = append(steps, site.StepPatch{Op: op.Op, Index: i, Files: map[string]string{op.Path: files[op.Path]}})
	}
	return sitebuild.ExecuteResult{Steps: steps}, nil
}

func (b *replayBuilder) Build(_ context.Context, req sitebuild.BuildRequest) (sitebuild.BuildResult, error) {
	b.mu.Lock()
	b.builds = append(b.builds, req)
	b.mu.Unlock()
	for path, f := range req.Project.Files {
		sum := sha256.Sum256([]byte(f.Content))
		if b.rec.Sources[path] != hex.EncodeToString(sum[:]) {
			return sitebuild.BuildResult{}, fmt.Errorf("the recording does not match %s", path)
		}
	}
	if len(req.Project.Files) != len(b.rec.Sources) {
		return sitebuild.BuildResult{}, errors.New("the recording has other files")
	}
	mode := b.rec.Preview
	if req.Mode == sitebuild.ModePublish {
		mode = b.rec.Publish
	}
	replace := strings.NewReplacer(b.rec.FormKey, req.Shell.Forms["contato"], b.rec.API, req.Shell.API, sitepublish.OriginPlaceholder, originOr(req.Shell.PrimaryOrigin))
	out := sitebuild.BuildResult{Imports: mode.Imports, PrerenderPath: mode.PrerenderPath}
	for _, a := range mode.Assets {
		body, err := base64.StdEncoding.DecodeString(a.Body)
		if err != nil {
			return sitebuild.BuildResult{}, err
		}
		out.Assets = append(out.Assets, sitebuild.Asset{Path: a.Path, ContentType: a.ContentType, Body: []byte(replace.Replace(string(body)))})
	}
	for _, r := range mode.Routes {
		out.Routes = append(out.Routes, sitebuild.RouteOutput{Route: r.Route, HTMLPath: r.HTMLPath, Interactive: r.Interactive})
	}
	return out, nil
}

func originOr(origin string) string {
	if origin == "" {
		return sitepublish.OriginPlaceholder
	}
	return origin
}

func (b *replayBuilder) last() sitebuild.BuildRequest {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.builds[len(b.builds)-1]
}

type replayRenderer struct {
	rec     recordedBuild
	calls   int
	mu      sync.Mutex
	socials []string
	shots   []string
	sizes   []siterender.Viewport
}

func (r *replayRenderer) Prerender(_ context.Context, url string, routes []string) (map[string]string, error) {
	r.calls++
	if !strings.HasPrefix(url, "https://") {
		return nil, errors.New("the candidate must be served over https")
	}
	out := map[string]string{}
	for _, route := range routes {
		markup, ok := r.rec.Publish.Rendered[route]
		if !ok {
			return nil, fmt.Errorf("no recorded markup for %s", route)
		}
		out[route] = markup
	}
	return out, nil
}

type e2eCache struct {
	mu    sync.Mutex
	items map[string]any
}

func (c *e2eCache) Set(key string, value any, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if b, ok := value.([]byte); ok {
		value = string(b)
	}
	c.items[key] = value
	return nil
}

func (c *e2eCache) Get(key string) (any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.items[key], nil
}

func (c *e2eCache) Delete(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
	return nil
}

type e2eNoMedia struct{}

func (e2eNoMedia) Locate(context.Context, string, []string) (map[string]string, error) {
	return map[string]string{}, nil
}

func (e2eNoMedia) Owns(string) bool { return false }

func (e2eNoMedia) Fetch(context.Context, string) ([]byte, string, error) {
	return nil, "", errors.New("no library media in this page")
}

type e2eWorkspaces map[string]string

func (w e2eWorkspaces) WorkspaceName(_ context.Context, workspaceID string) (string, error) {
	if name, ok := w[workspaceID]; ok {
		return name, nil
	}
	return "", errors.New("unknown workspace")
}

type e2eAnyReference struct{}

func (e2eAnyReference) Check(context.Context, string, string, site.FormSettings) error { return nil }

type e2eSiteStore struct {
	mu        sync.Mutex
	projects  map[string]site_usecase.Project
	revisions map[string]site_usecase.Revision
	blobs     map[string][]byte
	settings  map[string]site_usecase.StoredSettings
	designs   e2eDesigns
}

func newE2ESiteStore() *e2eSiteStore {
	return &e2eSiteStore{projects: map[string]site_usecase.Project{}, revisions: map[string]site_usecase.Revision{}, blobs: map[string][]byte{}, settings: map[string]site_usecase.StoredSettings{}}
}

func (m *e2eSiteStore) CreateProject(_ context.Context, in site_usecase.NewProject) (*site_usecase.Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.projects[in.Project.ID] = in.Project
	m.revisions[in.Revision.ID] = in.Revision
	for _, b := range in.Blobs {
		m.blobs[b.SHA256] = b.Bytes
	}
	m.settings[in.Project.ID] = in.Settings
	p := in.Project
	return &p, nil
}

func (m *e2eSiteStore) GetProject(_ context.Context, workspaceID, id string) (*site_usecase.Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[id]
	if !ok || p.WorkspaceID != workspaceID {
		return nil, site_usecase.ErrProjectNotFound
	}
	return &p, nil
}

func (m *e2eSiteStore) ListProjects(context.Context, site_usecase.ListQuery) ([]site_usecase.Project, int64, error) {
	return nil, 0, nil
}

func (m *e2eSiteStore) TakenSlugs(_ context.Context, workspaceID, prefix string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, p := range m.projects {
		if p.WorkspaceID == workspaceID && strings.HasPrefix(p.Slug, prefix) {
			out = append(out, p.Slug)
		}
	}
	return out, nil
}

func (m *e2eSiteStore) UpdateProject(_ context.Context, p *site_usecase.Project, expected int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.projects[p.ID]
	if !ok || current.WorkspaceID != p.WorkspaceID {
		return site_usecase.ErrProjectNotFound
	}
	if current.Version != expected {
		return shared.ErrVersionConflict
	}
	p.Version = current.Version + 1
	m.projects[p.ID] = *p
	return nil
}

func (m *e2eSiteStore) DeleteProject(_ context.Context, workspaceID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.projects, id)
	return nil
}

func (m *e2eSiteStore) SetPublication(_ context.Context, workspaceID, id string, status site_usecase.Status, deploymentID string) (*site_usecase.Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[id]
	if !ok || p.WorkspaceID != workspaceID {
		return nil, site_usecase.ErrProjectNotFound
	}
	p.Status, p.PublishedDeploymentID = status, deploymentID
	m.projects[id] = p
	return &p, nil
}

func (m *e2eSiteStore) Commit(_ context.Context, c site_usecase.Commit) (*site_usecase.Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[c.ProjectID]
	if !ok || p.WorkspaceID != c.WorkspaceID {
		return nil, site_usecase.ErrProjectNotFound
	}
	if p.Version != c.ExpectedVersion {
		return nil, shared.ErrVersionConflict
	}
	for _, b := range c.Blobs {
		m.blobs[b.SHA256] = b.Bytes
	}
	m.revisions[c.Revision.ID] = c.Revision
	p.WorkingRevisionID = c.Revision.ID
	p.Version++
	m.projects[p.ID] = p
	return &p, nil
}

func (m *e2eSiteStore) GetRevision(_ context.Context, workspaceID, projectID, revisionID string) (*site_usecase.Revision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.revisions[revisionID]
	if !ok || r.WorkspaceID != workspaceID || r.ProjectID != projectID {
		return nil, site_usecase.ErrRevisionNotFound
	}
	r.Manifest = maps.Clone(r.Manifest)
	return &r, nil
}

func (m *e2eSiteStore) ListRevisions(context.Context, string, string, int, int) ([]site_usecase.Revision, int64, error) {
	return nil, 0, nil
}

func (m *e2eSiteStore) Blobs(_ context.Context, _ string, shas []string) (map[string][]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string][]byte{}
	for _, sha := range shas {
		if b, ok := m.blobs[sha]; ok {
			out[sha] = b
		}
	}
	return out, nil
}

func (m *e2eSiteStore) GetSettings(_ context.Context, workspaceID, projectID string) (*site_usecase.StoredSettings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.settings[projectID]
	if !ok || s.WorkspaceID != workspaceID {
		return nil, site_usecase.ErrProjectNotFound
	}
	return &s, nil
}

func (m *e2eSiteStore) SaveSettings(_ context.Context, s *site_usecase.StoredSettings, expected int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current := m.settings[s.ProjectID]
	if current.Version != expected {
		return shared.ErrVersionConflict
	}
	s.Version = current.Version + 1
	m.settings[s.ProjectID] = *s
	return nil
}

func (m *e2eSiteStore) CreateDesignSystem(_ context.Context, rec *site_usecase.DesignSystemRecord) error {
	m.designs.create(rec)
	return nil
}

func (m *e2eSiteStore) GetDesignSystem(_ context.Context, workspaceID, id string) (*site_usecase.DesignSystemRecord, error) {
	return m.designs.get(workspaceID, id)
}

func (m *e2eSiteStore) ListDesignSystems(_ context.Context, workspaceID string) ([]site_usecase.DesignSystemRecord, error) {
	return m.designs.list(workspaceID), nil
}

func (m *e2eSiteStore) UpdateDesignSystem(_ context.Context, rec *site_usecase.DesignSystemRecord, expected int64) error {
	return m.designs.update(rec, expected)
}

func (m *e2eSiteStore) DeleteDesignSystem(context.Context, string, string) error {
	return site_usecase.ErrDesignSystemNotFound
}

func (m *e2eSiteStore) WorkspaceOf(_ context.Context, projectID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[projectID]
	if !ok {
		return "", site_usecase.ErrProjectNotFound
	}
	return p.WorkspaceID, nil
}

type e2eObjects struct {
	mu      sync.Mutex
	objects map[string][]byte
	keys    map[string][]byte
}

func (o *e2eObjects) Exists(_ context.Context, key string) (bool, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	_, ok := o.objects[key]
	return ok, nil
}

func (o *e2eObjects) PutStream(_ context.Context, key, _ string, body io.Reader) error {
	raw, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.objects[key] = raw
	return nil
}

func (o *e2eObjects) DownloadFile(_ context.Context, key string) ([]byte, string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	raw, ok := o.objects[key]
	if !ok {
		return nil, "", errors.New("NoSuchKey")
	}
	return raw, "", nil
}

func (o *e2eObjects) Put(_ context.Context, key string, value []byte) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.keys[key] = value
	return nil
}

func (o *e2eObjects) Delete(_ context.Context, key string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.keys, key)
	return nil
}

func (o *e2eObjects) Get(_ context.Context, key string) ([]byte, bool, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	value, ok := o.keys[key]
	return value, ok, nil
}

type e2eVisits struct {
	mu       sync.Mutex
	recorded []sitepublish.Visit
}

func (v *e2eVisits) Record(_ context.Context, visit sitepublish.Visit) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.recorded = append(v.recorded, visit)
	return nil
}

type e2eNoDNS struct{}

func (e2eNoDNS) LookupTXT(context.Context, string) ([]string, error) {
	return nil, nil
}

type e2eNoReversals struct{}

func (e2eNoReversals) HasOpenReversal(context.Context, string) (bool, error) {
	return false, nil
}

type e2eNoCustomHostnames struct{}

func (e2eNoCustomHostnames) Create(context.Context, string) (sitepublish.CustomHostnameState, error) {
	return sitepublish.CustomHostnameState{}, errors.New("no custom hostnames in this test")
}

func (e2eNoCustomHostnames) Get(context.Context, string) (sitepublish.CustomHostnameState, error) {
	return sitepublish.CustomHostnameState{}, sitepublish.ErrHostnameNotFound
}

func (e2eNoCustomHostnames) Delete(context.Context, string) error { return nil }

type e2eAllow struct{}

func (e2eAllow) Execute(string, string, workspace.Resource, workspace.Action) error { return nil }

type e2eActive struct{}

func (e2eActive) Execute(string) (*workspace_plan.WorkspaceSubscription, error) {
	return &workspace_plan.WorkspaceSubscription{Status: workspace_plan.SubscriptionStatusActive}, nil
}

type e2eEntitlements struct{}

func (e2eEntitlements) Resolve(string, workspace_addon.EntitlementKind) (int, error) { return 1, nil }

type e2eNoWidgets struct{}

func (e2eNoWidgets) FindByPublicKey(context.Context, string) (*webchat.Widget, error) {
	return nil, webchat.ErrWidgetNotFound
}

func (e2eNoWidgets) FindByID(context.Context, string, string) (*webchat.Widget, error) {
	return nil, webchat.ErrWidgetNotFound
}

type e2eDeployments struct {
	mu   sync.Mutex
	rows map[string]sitepublish.DeploymentRecord
	seq  []string
}

func (m *e2eDeployments) Create(_ context.Context, rec sitepublish.DeploymentRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[rec.ID] = rec
	m.seq = append(m.seq, rec.ID)
	return nil
}

func (m *e2eDeployments) Update(_ context.Context, rec sitepublish.DeploymentRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.rows[rec.ID]; !ok {
		return sitepublish.ErrDeploymentNotFound
	}
	m.rows[rec.ID] = rec
	return nil
}

func (m *e2eDeployments) Get(_ context.Context, workspaceID, id string) (sitepublish.DeploymentRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.rows[id]
	if !ok || rec.WorkspaceID != workspaceID {
		return sitepublish.DeploymentRecord{}, sitepublish.ErrDeploymentNotFound
	}
	return rec, nil
}

func (m *e2eDeployments) where(keep func(sitepublish.DeploymentRecord) bool) []sitepublish.DeploymentRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []sitepublish.DeploymentRecord
	for _, id := range m.seq {
		if rec := m.rows[id]; keep(rec) {
			out = append(out, rec)
		}
	}
	return out
}

func (m *e2eDeployments) Active(_ context.Context, project string) (sitepublish.DeploymentRecord, bool, error) {
	found := m.where(func(r sitepublish.DeploymentRecord) bool {
		return r.ProjectID == project && r.Status == sitepublish.DeploymentActive
	})
	if len(found) == 0 {
		return sitepublish.DeploymentRecord{}, false, nil
	}
	return found[0], true, nil
}

func (m *e2eDeployments) Latest(_ context.Context, project string) (sitepublish.DeploymentRecord, error) {
	found := m.where(func(r sitepublish.DeploymentRecord) bool { return r.ProjectID == project })
	if len(found) == 0 {
		return sitepublish.DeploymentRecord{}, sitepublish.ErrDeploymentNotFound
	}
	return found[len(found)-1], nil
}

func (m *e2eDeployments) ListByProject(_ context.Context, workspaceID, project string, _ int) ([]sitepublish.DeploymentRecord, error) {
	found := m.where(func(r sitepublish.DeploymentRecord) bool {
		return r.ProjectID == project && r.WorkspaceID == workspaceID
	})
	slices.Reverse(found)
	return found, nil
}

func (m *e2eDeployments) ListActiveByWorkspace(_ context.Context, workspaceID string) ([]sitepublish.DeploymentRecord, error) {
	return m.where(func(r sitepublish.DeploymentRecord) bool {
		return r.WorkspaceID == workspaceID && r.Status == sitepublish.DeploymentActive
	}), nil
}

func (m *e2eDeployments) ListServable(_ context.Context, project string) ([]sitepublish.DeploymentRecord, error) {
	return m.where(func(r sitepublish.DeploymentRecord) bool {
		return r.ProjectID == project && slices.Contains([]sitepublish.DeploymentStatus{sitepublish.DeploymentReady, sitepublish.DeploymentActive, sitepublish.DeploymentSuperseded}, r.Status)
	}), nil
}

func (m *e2eDeployments) Activate(_ context.Context, project, id, by string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	target, ok := m.rows[id]
	if !ok || !(sitepublish.Deployment{Status: target.Status}).Activatable() {
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

type e2eHostnames struct {
	mu   sync.Mutex
	rows map[string]sitepublish.HostnameRecord
}

func (m *e2eHostnames) Create(_ context.Context, rec sitepublish.HostnameRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rows {
		if r.Hostname == rec.Hostname {
			return sitepublish.ErrHostnameTaken
		}
	}
	m.rows[rec.ID] = rec
	return nil
}

func (m *e2eHostnames) Update(_ context.Context, rec sitepublish.HostnameRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[rec.ID] = rec
	return nil
}

func (m *e2eHostnames) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rows, id)
	return nil
}

func (m *e2eHostnames) Get(_ context.Context, workspaceID, id string) (sitepublish.HostnameRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.rows[id]
	if !ok || rec.WorkspaceID != workspaceID {
		return sitepublish.HostnameRecord{}, sitepublish.ErrHostnameNotFound
	}
	return rec, nil
}

func (m *e2eHostnames) sorted(keep func(sitepublish.HostnameRecord) bool) []sitepublish.HostnameRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []sitepublish.HostnameRecord
	for _, r := range m.rows {
		if keep(r) {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Hostname < out[j].Hostname })
	return out
}

func (m *e2eHostnames) ListByProject(_ context.Context, workspaceID, project string) ([]sitepublish.HostnameRecord, error) {
	return m.sorted(func(r sitepublish.HostnameRecord) bool { return r.WorkspaceID == workspaceID && r.ProjectID == project }), nil
}

func (m *e2eHostnames) ListByProjects(_ context.Context, workspaceID string, projects []string) ([]sitepublish.HostnameRecord, error) {
	return m.sorted(func(r sitepublish.HostnameRecord) bool {
		return r.WorkspaceID == workspaceID && slices.Contains(projects, r.ProjectID)
	}), nil
}

func (m *e2eHostnames) ListByStatus(_ context.Context, statuses []sitepublish.HostnameStatus, kind sitepublish.HostnameKind, _ int) ([]sitepublish.HostnameRecord, error) {
	return m.sorted(func(r sitepublish.HostnameRecord) bool { return r.Kind == kind && slices.Contains(statuses, r.Status) }), nil
}

func (m *e2eHostnames) ActiveHostnames(_ context.Context, project string) ([]string, error) {
	var out []string
	for _, r := range m.sorted(func(r sitepublish.HostnameRecord) bool {
		return r.ProjectID == project && r.Status == sitepublish.HostnameActive
	}) {
		out = append(out, r.Hostname)
	}
	return out, nil
}

func (m *e2eHostnames) CountCustom(_ context.Context, workspaceID string) (int, error) {
	return len(m.sorted(func(r sitepublish.HostnameRecord) bool {
		return r.WorkspaceID == workspaceID && r.Kind == sitepublish.HostnameCustom
	})), nil
}

type e2eAbuse struct{ rows []sitepublish.AbuseCase }

func (m *e2eAbuse) Create(_ context.Context, c sitepublish.AbuseCase) error {
	m.rows = append(m.rows, c)
	return nil
}

func (m *e2eAbuse) Update(_ context.Context, c sitepublish.AbuseCase) error {
	for i := range m.rows {
		if m.rows[i].ID == c.ID {
			m.rows[i] = c
			return nil
		}
	}
	return sitepublish.ErrAbuseCaseNotFound
}

func (m *e2eAbuse) List(context.Context, []sitepublish.AbuseStatus, int) ([]sitepublish.AbuseCase, error) {
	return m.rows, nil
}

func (m *e2eAbuse) OpenForProject(_ context.Context, project string) ([]sitepublish.AbuseCase, error) {
	var out []sitepublish.AbuseCase
	for _, c := range m.rows {
		if c.ProjectID == project && c.Status.Open() {
			out = append(out, c)
		}
	}
	return out, nil
}

func (m *e2eAbuse) Suspended(_ context.Context, project string) (bool, error) {
	for _, c := range m.rows {
		if c.ProjectID == project && c.Status == sitepublish.AbuseSuspended {
			return true, nil
		}
	}
	return false, nil
}

type e2eNotices struct {
	mu     sync.Mutex
	drafts []notification.Draft
}

func (n *e2eNotices) Publish(_ context.Context, d notification.Draft) (notification.Record, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.drafts = append(n.drafts, d)
	return notification.Record{}, nil
}

func (n *e2eNotices) Resolve(context.Context, string, ...string) error { return nil }

type e2eFormStore struct {
	mu          sync.Mutex
	keys        map[string]map[string]string
	active      map[string]siteforms.ActiveForm
	activations []siteforms.Activation
	deactivated []string
}

func (f *e2eFormStore) EnsureKeys(_ context.Context, _, projectID string, proposed map[string]string) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.keys[projectID] == nil {
		f.keys[projectID] = map[string]string{}
	}
	out := map[string]string{}
	for id, key := range proposed {
		if existing, ok := f.keys[projectID][id]; ok {
			key = existing
		}
		f.keys[projectID][id] = key
		out[id] = key
	}
	return out, nil
}

func (f *e2eFormStore) Activate(_ context.Context, a siteforms.Activation) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for key, form := range f.active {
		if form.ProjectID == a.ProjectID {
			delete(f.active, key)
		}
	}
	for _, form := range a.Forms {
		f.active[form.FormKey] = form
	}
	f.activations = append(f.activations, a)
	return nil
}

func (f *e2eFormStore) Deactivate(_ context.Context, projectID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for key, form := range f.active {
		if form.ProjectID == projectID {
			delete(f.active, key)
		}
	}
	f.deactivated = append(f.deactivated, projectID)
	return nil
}

func (f *e2eFormStore) ActiveForm(_ context.Context, formKey string) (*siteforms.ActiveForm, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	form, ok := f.active[formKey]
	if !ok {
		return nil, nil
	}
	return &form, nil
}

func (f *e2eFormStore) ProjectsEmbedding(context.Context, string, string) ([]string, error) {
	return nil, nil
}

func (f *e2eFormStore) WorkspaceForms(context.Context, string) ([]siteforms.PageForm, error) {
	return nil, nil
}

type e2eSubmissions struct {
	mu      sync.Mutex
	rows    []sitesubmission.Submission
	effects *e2eEffects
}

func (s *e2eSubmissions) ByIdempotency(_ context.Context, formKey, key string) (*sitesubmission.Submission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.rows {
		if r.FormKey == formKey && r.IdempotencyKey == key {
			return &r, nil
		}
	}
	return nil, nil
}

func (s *e2eSubmissions) Create(ctx context.Context, sub sitesubmission.Submission, run sitesubmission.EffectRun) (sitesubmission.Submission, bool, error) {
	if existing, _ := s.ByIdempotency(ctx, sub.FormKey, sub.IdempotencyKey); existing != nil {
		return *existing, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows = append(s.rows, sub)
	if len(run.Pending) > 0 {
		s.effects.save(run)
	}
	return sub, true, nil
}

type e2eEffects struct {
	mu   sync.Mutex
	runs map[string]*sitesubmission.EffectRun
}

func (e *e2eEffects) save(run sitesubmission.EffectRun) {
	e.mu.Lock()
	defer e.mu.Unlock()
	run.Pending = slices.Clone(run.Pending)
	e.runs[run.SubmissionID] = &run
}

func (e *e2eEffects) Claim(_ context.Context, id string, now time.Time, _ time.Duration) (*sitesubmission.EffectRun, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	run, ok := e.runs[id]
	if !ok || run.FinishedAt != nil || run.NextAttemptAt.After(now) {
		return nil, nil
	}
	claimed := *run
	claimed.Pending = slices.Clone(run.Pending)
	return &claimed, nil
}

func (e *e2eEffects) Due(context.Context, time.Time, int) ([]string, error) {
	return nil, nil
}

func (e *e2eEffects) Progress(_ context.Context, run sitesubmission.EffectRun) error {
	e.save(run)
	return nil
}

func (e *e2eEffects) Settle(_ context.Context, run sitesubmission.EffectRun) error {
	e.save(run)
	return nil
}

type e2eNoDeals struct{}

func (e2eNoDeals) DealsOfLead(context.Context, opportunity.LeadDealsQuery) ([]*opportunity.Opportunity, error) {
	return nil, nil
}

func (s *e2eSubmissions) AttachConversation(context.Context, string, string, string) error {
	return nil
}

func (s *e2eSubmissions) List(context.Context, siteforms.SubmissionQuery) ([]sitesubmission.Submission, int64, error) {
	return nil, 0, nil
}

type e2eSpent struct {
	mu   sync.Mutex
	seen map[string]bool
}

func (s *e2eSpent) SetNX(key, _ string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen[key] {
		return false, nil
	}
	s.seen[key] = true
	return true, nil
}

type e2eOpenLimiter struct{}

func (e2eOpenLimiter) Allow(string) (bool, time.Duration, error) { return true, 0, nil }

type e2eLeads struct {
	mu       sync.Mutex
	byPhone  map[string]*lead.Lead
	consents []lead.ConsentChange
	profiles []lead_usecase.ProfileUpdate
	triggers []formlead.Trigger
}

func (l *e2eLeads) FindOrCreate(workspaceID, number string, _ lead.LeadUpdate) (*lead.Lead, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if found, ok := l.byPhone[number]; ok {
		return found, false, nil
	}
	created := &lead.Lead{ID: fmt.Sprintf("lead-%d", len(l.byPhone)+1), WorkspaceID: workspaceID}
	l.byPhone[number] = created
	return created, true, nil
}

func (l *e2eLeads) FindOrCreateManyByEmail(context.Context, string, []lead.BulkEmailInput) (map[string]*lead.Lead, error) {
	return nil, errors.New("this form identifies by phone")
}

func (l *e2eLeads) Update(_ context.Context, in lead_usecase.ProfileUpdate) (lead_usecase.ProfileResult, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.profiles = append(l.profiles, in)
	return lead_usecase.ProfileResult{LeadID: in.LeadID}, nil
}

func (l *e2eLeads) Record(_ context.Context, _, _ string, change lead.ConsentChange) (lead_usecase.ConsentOutcome, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.consents = append(l.consents, change)
	return lead_usecase.ConsentOutcome{Changed: true}, nil
}

func (l *e2eLeads) FormSubmitted(_ context.Context, in formlead.Trigger) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.triggers = append(l.triggers, in)
	return nil
}

type e2eOptIns struct{}

func (e2eOptIns) Subscribe(context.Context, string, emailconsent_usecase.SubscribeInput) (emailconsent.Subscription, error) {
	return emailconsent.Subscription{}, nil
}

type e2eRoulette struct{}

func (e2eRoulette) PickForLead(string, string) (string, error) {
	return "", errors.New("no roulette set")
}

type e2eOwners struct{}

func (e2eOwners) SetOwner(_ context.Context, _ lead_usecase.Actor, id, _ string) (*lead.Lead, error) {
	return &lead.Lead{ID: id}, nil
}

type e2eDeals struct{}

func (e2eDeals) InitialStage(string, string) (string, error) { return "", errors.New("no funnel set") }

func (e2eDeals) Create(string, opportunity_usecase.CreateInput) (*opportunity.Opportunity, error) {
	return nil, errors.New("no funnel set")
}

type e2eOfficial struct {
	mu     sync.Mutex
	opened []wo.OpenEntryInput
}

func (o *e2eOfficial) Open(_ context.Context, in wo.OpenEntryInput) (*wo.OpenedEntry, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.opened = append(o.opened, in)
	return &wo.OpenedEntry{EntryID: "entry-1", EntryType: "whatsapp"}, nil
}

type e2eUnofficial struct{}

func (e2eUnofficial) Execute(context.Context, uwuc.StartConversationInput) (*uwuc.StartedConversation, error) {
	return nil, errors.New("not the follow up channel")
}

type e2eEmail struct{}

func (e2eEmail) OpenConversation(context.Context, emailinbox.OpenConversationInput) (string, error) {
	return "", errors.New("not the follow up channel")
}

type e2eConversions struct{}

func (e2eConversions) SendWebsiteLead(context.Context, string, ads.WebsiteLead) error { return nil }

type e2eLinks struct {
	mu      sync.Mutex
	created []shortlink.CreateShortLinkInput
}

func (l *e2eLinks) Execute(_ context.Context, in shortlink.CreateShortLinkInput) (*shortlink.ShortLink, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.created = append(l.created, in)
	n := len(l.created)
	return &shortlink.ShortLink{ID: fmt.Sprintf("sl-%d", n), Code: fmt.Sprintf("c%d", n), TargetURL: in.TargetURL}, nil
}

type e2eNumbers map[string]string

func (n e2eNumbers) NumberOf(_ context.Context, _ string, ref string) (string, error) {
	if digits, ok := n[ref]; ok {
		return digits, nil
	}
	return "", errors.New("unknown number")
}

type e2eReferences struct {
	mu   sync.Mutex
	rows []siteforms.PageReference
}

func (r *e2eReferences) Find(_ context.Context, key siteforms.PageReferenceKey) (*siteforms.PageReference, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, row := range r.rows {
		if row.Key == key {
			return &row, nil
		}
	}
	return nil, nil
}

func (r *e2eReferences) Create(_ context.Context, ref siteforms.PageReference) (siteforms.PageReference, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows = append(r.rows, ref)
	return ref, true, nil
}

func (r *e2eReferences) Target(_ context.Context, code string) (*conversation.PageReferenceTarget, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, row := range r.rows {
		if row.Code == code {
			return &conversation.PageReferenceTarget{WorkspaceID: row.WorkspaceID, ProjectID: row.Key.ProjectID, Route: row.Key.Route, NumberRef: row.Key.NumberRef}, nil
		}
	}
	return nil, nil
}

type e2eRetirement struct {
	world *siteWorld
}

func (r e2eRetirement) Retire(ctx context.Context, workspaceID, projectID string) error {
	return r.world.publisher.Retire(ctx, workspaceID, projectID)
}

func (r *replayRenderer) LiveDOM(_ context.Context, url string, routes []string) (map[string]string, error) {
	if !strings.HasPrefix(url, "https://") {
		return nil, errors.New("the candidate must be served over https")
	}
	out := map[string]string{}
	for _, route := range routes {
		markup := r.rec.Publish.Rendered[route]
		out[route] = `<!doctype html><html lang="pt-BR"><head><title>Página</title></head><body>` + markup + `</body></html>`
	}
	return out, nil
}

func (e2eNoMedia) Lookup(context.Context, string, string) (*media.Media, error) {
	return nil, media.ErrMediaNotFound
}
