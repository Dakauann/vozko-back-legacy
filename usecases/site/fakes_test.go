package site_usecase

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"vozko/domain/shared"
	"vozko/domain/site"
	"vozko/domain/sitepublish"
)

type memoryStore struct {
	mu        sync.Mutex
	projects  map[string]Project
	revisions map[string]Revision
	blobs     map[string][]byte
	settings  map[string]StoredSettings
	designs   map[string]DesignSystemRecord
	commits   []Commit
	created   []NewProject
	failNext  error

	fileRevs    map[string]string
	fileRevsErr error

	conflictOnce   bool
	conflictAlways bool
}

func (m *memoryStore) setThumbnail(projectID, mediaID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.projects[projectID]
	p.ThumbnailMediaID = mediaID
	m.projects[projectID] = p
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		projects: map[string]Project{}, revisions: map[string]Revision{}, blobs: map[string][]byte{},
		settings: map[string]StoredSettings{}, designs: map[string]DesignSystemRecord{},
	}
}

func blobKey(workspaceID, sha string) string { return workspaceID + "/" + sha }

func revKey(projectID, path string, rev uint64) string {
	return projectID + "|" + path + "|" + fmt.Sprint(rev)
}

func (m *memoryStore) recordRevs(projectID string, revs []FileRev) {
	if m.fileRevs == nil {
		m.fileRevs = map[string]string{}
	}
	for _, r := range revs {
		m.fileRevs[revKey(projectID, r.Path, r.Rev)] = r.SHA256
	}
}

func (m *memoryStore) FileRevs(_ context.Context, workspaceID, projectID string, wants map[string][]uint64) (map[string]map[uint64]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fileRevsErr != nil {
		return nil, m.fileRevsErr
	}
	if p, ok := m.projects[projectID]; !ok || p.WorkspaceID != workspaceID {
		return nil, ErrProjectNotFound
	}
	out := map[string]map[uint64]string{}
	for path, revs := range wants {
		for _, rev := range revs {
			if sha, ok := m.fileRevs[revKey(projectID, path, rev)]; ok {
				if out[path] == nil {
					out[path] = map[uint64]string{}
				}
				out[path][rev] = sha
			}
		}
	}
	return out, nil
}

type fakeNumbers struct {
	refuse map[string]bool
	asked  []string
}

func (f *fakeNumbers) NumberOf(_ context.Context, workspaceID, ref string) (string, error) {
	f.asked = append(f.asked, workspaceID+"/"+ref)
	if f.refuse[ref] {
		return "", errors.New("not this workspace")
	}
	return "5511999990000", nil
}

type fakeHostnames struct {
	list  []sitepublish.HostnameRecord
	err   error
	calls int
}

func (f *fakeHostnames) ListByProjects(_ context.Context, _ string, projectIDs []string) ([]sitepublish.HostnameRecord, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	var out []sitepublish.HostnameRecord
	for _, h := range f.list {
		if slices.Contains(projectIDs, h.ProjectID) {
			out = append(out, h)
		}
	}
	return out, nil
}

type fakeCache struct {
	values map[string]string
	getErr error
}

func (f *fakeCache) GetString(key string) (string, error) {
	if f.getErr != nil {
		return "", f.getErr
	}
	return f.values[key], nil
}

func (f *fakeCache) SetString(key, value string, _ time.Duration) error {
	if f.values == nil {
		f.values = map[string]string{}
	}
	f.values[key] = value
	return nil
}

func (m *memoryStore) fail() error {
	err := m.failNext
	m.failNext = nil
	return err
}

func (m *memoryStore) CreateProject(_ context.Context, in NewProject) (*Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.fail(); err != nil {
		return nil, err
	}
	for _, p := range m.projects {
		if p.WorkspaceID == in.Project.WorkspaceID && p.Slug == in.Project.Slug {
			return nil, ErrSlugTaken
		}
		if !in.Project.Origin.Empty() && p.Origin == in.Project.Origin {
			if p.WorkspaceID != in.Project.WorkspaceID || p.CreatedBy != in.Project.CreatedBy {
				return nil, ErrOriginTaken
			}
			existing := p
			return &existing, nil
		}
	}
	m.projects[in.Project.ID] = in.Project
	m.revisions[in.Revision.ID] = in.Revision
	m.created = append(m.created, in)
	m.recordRevs(in.Project.ID, in.FileRevs)
	for _, b := range in.Blobs {
		m.blobs[blobKey(in.Project.WorkspaceID, b.SHA256)] = b.Bytes
	}
	m.settings[in.Project.ID] = in.Settings
	p := in.Project
	return &p, nil
}

func (m *memoryStore) GetProject(_ context.Context, workspaceID, id string) (*Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[id]
	if !ok || p.WorkspaceID != workspaceID {
		return nil, ErrProjectNotFound
	}
	return &p, nil
}

func (m *memoryStore) ListProjects(_ context.Context, q ListQuery) ([]Project, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Project
	for _, p := range m.projects {
		if p.WorkspaceID != q.WorkspaceID {
			continue
		}
		if q.Status == "" && p.Status == StatusArchived || q.Status != "" && p.Status != q.Status {
			continue
		}
		if q.Search != "" && !strings.Contains(strings.ToLower(p.Name), strings.ToLower(q.Search)) {
			continue
		}
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b Project) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	return out, int64(len(out)), nil
}

func (m *memoryStore) TakenSlugs(_ context.Context, workspaceID, prefix string) ([]string, error) {
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

func (m *memoryStore) checkVersion(workspaceID, id string, expected int64) (Project, error) {
	p, ok := m.projects[id]
	if !ok || p.WorkspaceID != workspaceID {
		return Project{}, ErrProjectNotFound
	}
	if p.Version != expected {
		return Project{}, shared.ErrVersionConflict
	}
	return p, nil
}

func (m *memoryStore) UpdateProject(_ context.Context, p *Project, expected int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.fail(); err != nil {
		return err
	}
	current, err := m.checkVersion(p.WorkspaceID, p.ID, expected)
	if err != nil {
		return err
	}
	for _, other := range m.projects {
		if other.ID != p.ID && other.WorkspaceID == p.WorkspaceID && other.Slug == p.Slug {
			return ErrSlugTaken
		}
	}
	p.Version = current.Version + 1
	p.UpdatedAt = time.Now()
	m.projects[p.ID] = *p
	return nil
}

func (m *memoryStore) DeleteProject(_ context.Context, workspaceID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[id]
	if !ok || p.WorkspaceID != workspaceID {
		return ErrProjectNotFound
	}
	delete(m.projects, id)
	delete(m.settings, id)
	for rid, r := range m.revisions {
		if r.ProjectID == id {
			delete(m.revisions, rid)
		}
	}
	return nil
}

func (m *memoryStore) Commit(_ context.Context, c Commit) (*Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.fail(); err != nil {
		return nil, err
	}
	p, err := m.checkVersion(c.WorkspaceID, c.ProjectID, c.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	if p.Status == StatusArchived {
		return nil, ErrProjectArchived
	}
	if c.Replace {
		existing, ok := m.revisions[c.Revision.ID]
		if !ok || existing.Kind != RevisionAutosave {
			return nil, ErrRevisionNotFound
		}
	}
	m.commits = append(m.commits, c)
	m.recordRevs(c.ProjectID, c.FileRevs)
	for _, b := range c.Blobs {
		m.blobs[blobKey(c.WorkspaceID, b.SHA256)] = b.Bytes
	}
	m.revisions[c.Revision.ID] = c.Revision
	p.WorkingRevisionID = c.Revision.ID
	p.Version++
	p.UpdatedAt = c.Revision.UpdatedAt
	m.projects[p.ID] = p
	return &p, nil
}

func (m *memoryStore) GetRevision(_ context.Context, workspaceID, projectID, revisionID string) (*Revision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.revisions[revisionID]
	if !ok || r.WorkspaceID != workspaceID || r.ProjectID != projectID {
		return nil, ErrRevisionNotFound
	}
	r.Manifest = maps.Clone(r.Manifest)
	return &r, nil
}

func (m *memoryStore) ListRevisions(_ context.Context, workspaceID, projectID string, limit, offset int) ([]Revision, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Revision
	for _, r := range m.revisions {
		if r.WorkspaceID == workspaceID && r.ProjectID == projectID {
			out = append(out, r)
		}
	}
	slices.SortFunc(out, func(a, b Revision) int { return b.CreatedAt.Compare(a.CreatedAt) })
	total := int64(len(out))
	if offset >= len(out) {
		return nil, total, nil
	}
	out = out[offset:]
	if limit < len(out) {
		out = out[:limit]
	}
	return out, total, nil
}

func (m *memoryStore) Blobs(_ context.Context, workspaceID string, shas []string) (map[string][]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string][]byte{}
	for _, sha := range shas {
		if b, ok := m.blobs[blobKey(workspaceID, sha)]; ok {
			out[sha] = b
		}
	}
	return out, nil
}

func (m *memoryStore) GetSettings(_ context.Context, workspaceID, projectID string) (*StoredSettings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.settings[projectID]
	if !ok || s.WorkspaceID != workspaceID {
		return nil, ErrProjectNotFound
	}
	return &s, nil
}

func (m *memoryStore) SaveSettings(_ context.Context, s *StoredSettings, expected int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.settings[s.ProjectID]
	if !ok || current.WorkspaceID != s.WorkspaceID {
		return ErrProjectNotFound
	}
	if current.Version != expected {
		return shared.ErrVersionConflict
	}
	s.Version = current.Version + 1
	m.settings[s.ProjectID] = *s
	return nil
}

func (m *memoryStore) CreateDesignSystem(_ context.Context, d *DesignSystemRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if d.IsDefault {
		m.clearDefault(d.WorkspaceID)
	}
	d.Version = 1
	m.designs[d.ID] = *d
	return nil
}

func (m *memoryStore) clearDefault(workspaceID string) {
	for id, other := range m.designs {
		if other.WorkspaceID == workspaceID && other.IsDefault {
			other.IsDefault = false
			m.designs[id] = other
		}
	}
}

func (m *memoryStore) GetDesignSystem(_ context.Context, workspaceID, id string) (*DesignSystemRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.designs[id]
	if !ok || d.WorkspaceID != workspaceID {
		return nil, ErrDesignSystemNotFound
	}
	return &d, nil
}

func (m *memoryStore) ListDesignSystems(_ context.Context, workspaceID string) ([]DesignSystemRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []DesignSystemRecord
	for _, d := range m.designs {
		if d.WorkspaceID == workspaceID {
			out = append(out, d)
		}
	}
	slices.SortFunc(out, func(a, b DesignSystemRecord) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

func (m *memoryStore) UpdateDesignSystem(_ context.Context, d *DesignSystemRecord, expected int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.designs[d.ID]
	if !ok || current.WorkspaceID != d.WorkspaceID {
		return ErrDesignSystemNotFound
	}
	if current.Version != expected || m.conflictAlways {
		return shared.ErrVersionConflict
	}
	if m.conflictOnce {
		m.conflictOnce = false
		current.Version++
		m.designs[d.ID] = current
		return shared.ErrVersionConflict
	}
	if d.IsDefault {
		m.clearDefault(d.WorkspaceID)
	}
	d.Version = current.Version + 1
	m.designs[d.ID] = *d
	return nil
}

func (m *memoryStore) DeleteDesignSystem(_ context.Context, workspaceID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.designs[id]
	if !ok || d.WorkspaceID != workspaceID {
		return ErrDesignSystemNotFound
	}
	delete(m.designs, id)
	for pid, p := range m.projects {
		if p.DesignSystemID == id {
			p.DesignSystemID = ""
			m.projects[pid] = p
		}
	}
	return nil
}

type fakePublished struct {
	revisions map[string]string
	err       error
}

func (f *fakePublished) PublishedRevision(_ context.Context, _, deploymentID string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.revisions[deploymentID], nil
}

type fakeForms struct {
	deactivated []string
	err         error
}

func (f *fakeForms) Deactivate(_ context.Context, projectID string) error {
	if f.err != nil {
		return f.err
	}
	f.deactivated = append(f.deactivated, projectID)
	return nil
}

type fakeExecutor struct {
	result   ExecuteResult
	err      error
	calls    []ExecuteRequest
	steps    func(ExecuteRequest) []site.StepPatch
	extract  ExtractResult
	extracts []ExtractRequest
}

func (f *fakeExecutor) Extract(_ context.Context, req ExtractRequest) (ExtractResult, error) {
	f.extracts = append(f.extracts, req)
	if f.err != nil {
		return ExtractResult{}, f.err
	}
	return f.extract, nil
}

type fakeMedia struct {
	urls  map[string]string
	err   error
	calls [][]string
}

func (f *fakeMedia) Locate(_ context.Context, workspaceID string, ids []string) (map[string]string, error) {
	f.calls = append(f.calls, append([]string{workspaceID}, ids...))
	if f.err != nil {
		return nil, f.err
	}
	out := map[string]string{}
	for _, id := range ids {
		if u, ok := f.urls[id]; ok {
			out[id] = u
		}
	}
	return out, nil
}

func (f *fakeExecutor) Execute(_ context.Context, req ExecuteRequest) (ExecuteResult, error) {
	f.calls = append(f.calls, req)
	if f.err != nil {
		return ExecuteResult{}, f.err
	}
	if f.steps != nil {
		return ExecuteResult{Steps: f.steps(req)}, nil
	}
	return f.result, nil
}

type fakePreviewer struct {
	got PreviewRequest
	err error
}

func (f *fakePreviewer) Preview(_ context.Context, req PreviewRequest) (PreviewResult, error) {
	f.got = req
	if f.err != nil {
		return PreviewResult{}, f.err
	}
	return PreviewResult{URL: "https://tok." + "preview.example/", RevisionID: req.RevisionID}, nil
}

type fakeReferences struct {
	refuse map[string]error
	seen   []string
}

func (f *fakeReferences) Check(_ context.Context, workspaceID, formID string, form site.FormSettings) error {
	f.seen = append(f.seen, workspaceID+"/"+formID)
	if err, ok := f.refuse[formID]; ok {
		return &FormReferenceError{Form: formID, Err: err}
	}
	return nil
}

func (f *fakeReferences) CheckWebchat(_ context.Context, workspaceID, ref string) error {
	f.seen = append(f.seen, workspaceID+"/webchat:"+ref)
	return f.refuse["webchat"]
}

type fakeBindings struct {
	bound map[string]bool
	keyed []string
	err   error
}

func (f *fakeBindings) KeysFor(_ context.Context, _, projectID string, formIDs []string) (map[string]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.keyed = append(f.keyed, projectID+":"+strings.Join(formIDs, ","))
	out := map[string]string{}
	for _, id := range formIDs {
		out[id] = "key-" + id
	}
	return out, nil
}

func (f *fakeBindings) WorkflowBound(_ context.Context, _, _, formID, workflowID string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.bound[formID+"/"+workflowID], nil
}

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

func (c *clock) advance(d time.Duration) { c.now = c.now.Add(d) }

type sequence struct{ n int }

func (s *sequence) next() string {
	s.n++
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", s.n)
}

var errStoreDown = errors.New("store down")

func (m *memoryStore) SetPublication(_ context.Context, workspaceID, id string, status Status, deploymentID string) (*Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[id]
	if !ok || p.WorkspaceID != workspaceID {
		return nil, ErrProjectNotFound
	}
	if p.Status == StatusArchived {
		return nil, ErrProjectArchived
	}
	p.Status, p.PublishedDeploymentID = status, deploymentID
	m.projects[id] = p
	return &p, nil
}

type fakeRetirement struct {
	calls []string
	err   error
}

func (f *fakeRetirement) Retire(_ context.Context, workspaceID, projectID string) error {
	f.calls = append(f.calls, workspaceID+"|"+projectID)
	return f.err
}
