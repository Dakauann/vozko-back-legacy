package sitetools

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"strings"
	"testing"

	"vozko/domain/copilot"
	"vozko/domain/shared"
	"vozko/domain/site"
	"vozko/domain/sitepublish"
	"vozko/usecases/copilot/copilottools/tooltest"
	site_usecase "vozko/usecases/site"
	sitepublish_usecase "vozko/usecases/sitepublish"
	"vozko/usecases/siterender"
)

const (
	projectID  = "3b9a7c2d-5e6f-4a1b-8c9d-0e1f2a3b4c5d"
	threadID   = "7a6b5c4d-3e2f-4a1b-9c8d-7e6f5a4b3c2d"
	previewURL = "https://tok.preview.example.com/"
)

var siteView = copilot.View{Surface: copilot.SurfaceSite, ProjectID: projectID}

type fakeStorage struct {
	view       *site_usecase.View
	settings   *site_usecase.StoredSettings
	created    []site_usecase.CreateInput
	edits      []site_usecase.EditInput
	puts       [][]byte
	previewErr error
	build      string
	editErr    error
	conflicts  int
	previous   *site.Project
}

func (f *fakeStorage) Get(_ context.Context, workspaceID, id string) (*site_usecase.View, error) {
	if f.view == nil || f.view.Project.ID != id || f.view.Project.WorkspaceID != workspaceID {
		return nil, site_usecase.ErrProjectNotFound
	}
	copied := *f.view
	copied.Files = maps.Clone(f.view.Files)
	copied.Settings = f.settings
	return &copied, nil
}

func (f *fakeStorage) Create(_ context.Context, in site_usecase.CreateInput) (*site_usecase.View, error) {
	starter, err := site_usecase.StarterFor(in.Stack, in.Starter)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Name) == "" {
		return nil, site_usecase.ErrNameInvalid
	}
	for _, earlier := range f.created {
		if earlier.Origin == in.Origin {
			return f.view, nil
		}
	}
	f.created = append(f.created, in)
	f.view = &site_usecase.View{Project: site_usecase.Project{ID: projectID, WorkspaceID: in.WorkspaceID, Name: in.Name, Stack: in.Stack, Status: site_usecase.StatusDraft, Version: 1}, Files: starter.Project().Files}
	return f.view, nil
}

func (f *fakeStorage) Edit(_ context.Context, in site_usecase.EditInput) (*site_usecase.EditResult, error) {
	f.edits = append(f.edits, in)
	if f.editErr != nil {
		return nil, f.editErr
	}
	if f.conflicts > 0 {
		f.conflicts--
		f.view.Project.Version++
		return nil, shared.ErrVersionConflict
	}
	if in.ExpectedVersion != f.view.Project.Version {
		return nil, shared.ErrVersionConflict
	}
	project := f.view.Source()
	batch, err := site.PrepareBatch(project, in.Batch)
	var refused *site.BatchError
	if errors.As(err, &refused) {
		return nil, &site_usecase.EditRefusal{Failures: refused.Failures, Skipped: refused.Skipped, Ignored: batch.Ignored}
	}
	if err != nil {
		return nil, err
	}
	steps := make([]site.StepPatch, 0, len(batch.Operations))
	current := project.Clone()
	for i, op := range batch.Operations {
		step := site.StepPatch{Op: op.Op, Index: i, Files: map[string]string{}}
		switch op.Op {
		case site.OpEditFile:
			text := current.Files[op.Path].Content
			if !strings.Contains(text, op.Old) {
				return nil, &site_usecase.EditRefusal{Failures: []site.Failure{{Index: i, Code: site.CodeEditNotFound, Field: "old"}}}
			}
			step.Files[op.Path] = strings.Replace(text, op.Old, *op.New, 1)
		case site.OpWriteFile:
			step.Files[op.Path] = *op.Content
		case site.OpReplaceImage:
			file, _, _ := strings.Cut(op.Node, "#")
			step.Files[file] = current.Files[file].Content + "<!-- " + op.MediaID + " -->"
		default:
			return nil, &site_usecase.EditRefusal{Failures: []site.Failure{{Index: i, Code: site.CodeOperationUnknown, Field: "op"}}}
		}
		next, err := site.ApplySteps(current, []site.StepPatch{step})
		if err != nil {
			return nil, err
		}
		current = next
		steps = append(steps, step)
	}
	before := f.view.Files
	f.view.Files = current.Files
	f.view.Project.Version++
	view := *f.view
	view.Changes = site.Changes(before, current.Files, steps)
	return &site_usecase.EditResult{View: view, Steps: steps, Ignored: batch.Ignored}, nil
}

func (f *fakeStorage) Preview(context.Context, string, string) (site_usecase.PreviewResult, error) {
	if f.previewErr != nil {
		return site_usecase.PreviewResult{}, f.previewErr
	}
	return site_usecase.PreviewResult{URL: previewURL, Build: f.build}, nil
}

func (f *fakeStorage) GetSettings(context.Context, string, string) (*site_usecase.StoredSettings, error) {
	if f.settings == nil {
		return nil, site_usecase.ErrProjectNotFound
	}
	copied := *f.settings
	return &copied, nil
}

func (f *fakeStorage) PutSettings(_ context.Context, workspaceID, projectID string, expected int64, raw []byte) (*site_usecase.StoredSettings, error) {
	f.puts = append(f.puts, raw)
	if expected != f.settings.Version {
		return nil, shared.ErrVersionConflict
	}
	settings, err := site.DecodeSettings(raw)
	if err != nil {
		return nil, err
	}
	f.settings = &site_usecase.StoredSettings{ProjectID: projectID, WorkspaceID: workspaceID, Settings: settings, Version: expected + 1}
	return f.settings, nil
}

func (f *fakeStorage) RevisionSource(context.Context, string, string, string) (site.Project, error) {
	if f.previous == nil {
		return site.Project{}, site_usecase.ErrRevisionNotFound
	}
	return *f.previous, nil
}

type fakeRenderer struct {
	reference siterender.Reference
	refErr    error
}

func (f *fakeRenderer) CaptureReference(_ context.Context, url string) (siterender.Reference, error) {
	return f.reference, f.refErr
}

type fakePublisher struct {
	hosts       []sitepublish.HostnameRecord
	deployments []sitepublish.DeploymentRecord
	result      sitepublish_usecase.PublishResult
	err         error
	published   []sitepublish_usecase.Actor
	settings    sitepublish.SettingsChanges
	settingsErr error
	pinned      []int64
}

func (f *fakePublisher) SettingsChanges(context.Context, sitepublish_usecase.Actor, string) (sitepublish.SettingsChanges, error) {
	return f.settings, f.settingsErr
}

func (f *fakePublisher) PublishPinned(_ context.Context, a sitepublish_usecase.Actor, _ string, version int64) (sitepublish_usecase.PublishResult, error) {
	f.published = append(f.published, a)
	f.pinned = append(f.pinned, version)
	return f.result, f.err
}

func (f *fakePublisher) ListHostnames(context.Context, sitepublish_usecase.Actor, string) ([]sitepublish.HostnameRecord, error) {
	return f.hosts, nil
}

func (f *fakePublisher) ListDeployments(context.Context, sitepublish_usecase.Actor, string, int) ([]sitepublish.DeploymentRecord, error) {
	return f.deployments, nil
}

type fakeEditor struct {
	replies  map[copilot.ScreenCommandName]copilot.ScreenReply
	errs     map[copilot.ScreenCommandName]error
	commands []copilot.ScreenCommand
	pages    map[string]string
	files    func() *site_usecase.View
}

func (f *fakeEditor) stamped(html string) string {
	view := f.files()
	revs, _ := json.Marshal(site.PreviewRevisions(view.Project.Stack, view.Files))
	return strings.Replace(html, "<head>", `<head><meta name="vz-revs" content="`+strings.ReplaceAll(string(revs), `"`, "&quot;")+`">`, 1)
}

func (f *fakeEditor) inspect(order inspectOrder) copilot.ScreenReply {
	documents := map[string]string{}
	for _, route := range order.Routes {
		html, ok := f.pages[route]
		if !ok {
			return copilot.ScreenReply{OK: false, Error: &copilot.ScreenError{Code: "capture_failed", Message: "a rota não abriu"}}
		}
		documents[route] = f.stamped(html)
	}
	data, _ := json.Marshal(inspectReply{Documents: documents})
	return copilot.ScreenReply{OK: true, Data: data}
}

func (f *fakeEditor) Run(_ context.Context, cmd copilot.ScreenCommand) (copilot.ScreenReply, error) {
	f.commands = append(f.commands, cmd)
	if err := f.errs[cmd.Name]; err != nil {
		return copilot.ScreenReply{}, err
	}
	if reply, ok := f.replies[cmd.Name]; ok {
		return reply, nil
	}
	switch cmd.Name {
	case copilot.ScreenRead:
		return copilot.ScreenReply{OK: true, Data: []byte(`{"stack":"react","files":{"src/routes/index.jsx":1},"route":"/","selection":{"node":"src/routes/index.jsx#120","tag":"h1","text":"Agende"}}`)}, nil
	case copilot.ScreenInspect:
		return f.inspect(cmd.Args.(inspectOrder)), nil
	case copilot.ScreenEdit:
		payload := cmd.Args.(editPayload)
		files := map[string]uint64{}
		for path := range payload.Base {
			files[path] = 2
		}
		data, _ := json.Marshal(map[string]interface{}{"files": files, "conflicts": []string{}, "skipped": []int{}})
		return copilot.ScreenReply{OK: true, Data: data}, nil
	}
	return copilot.ScreenReply{OK: true, Data: []byte(`{}`)}, nil
}

func (f *fakeEditor) sent(name copilot.ScreenCommandName) []copilot.ScreenCommand {
	var out []copilot.ScreenCommand
	for _, cmd := range f.commands {
		if cmd.Name == name {
			out = append(out, cmd)
		}
	}
	return out
}

const cleanHome = `<!doctype html><html lang="pt-BR"><head><title>Oi</title></head><body><main><h1 data-vz-node="src/routes/index.jsx#120">Agende</h1><a href="/privacidade">Privacidade</a></main></body></html>`

const gappedHome = `<!doctype html><html lang="pt-BR"><head><title>Oi</title></head><body><main><h1>Agende</h1><img src="https://cdn.example.com/a.jpg" data-vz-node="src/routes/index.jsx#300"><a href="#" data-vz-node="src/routes/index.jsx#400">Fale</a></main></body></html>`

func starterView(t *testing.T, key string) *site_usecase.View {
	t.Helper()
	starter, err := site_usecase.StarterFor(site.StackReact, key)
	if err != nil {
		t.Fatal(err)
	}
	return &site_usecase.View{
		Project: site_usecase.Project{ID: projectID, WorkspaceID: "ws-1", Name: "Clínica Sorriso", Stack: site.StackReact, Status: site_usecase.StatusDraft, Version: 3},
		Files:   starter.Project().Files,
	}
}

func starterSettings(t *testing.T, key string) *site_usecase.StoredSettings {
	t.Helper()
	starter, err := site_usecase.StarterFor(site.StackReact, key)
	if err != nil {
		t.Fatal(err)
	}
	return &site_usecase.StoredSettings{ProjectID: projectID, WorkspaceID: "ws-1", Settings: starter.Settings, Version: 1}
}

func allRoutes(html string) map[string]string {
	return map[string]string{"/": html, "/obrigado": cleanHome, "/privacidade": cleanHome}
}

type rig struct {
	deps      Deps
	storage   *fakeStorage
	renderer  *fakeRenderer
	publisher *fakePublisher
	editor    *fakeEditor
	images    *tooltest.StubImages
	cc        copilot.Context
}

func newRig(t *testing.T) *rig {
	t.Helper()
	storage := &fakeStorage{view: starterView(t, "captura-de-leads"), settings: starterSettings(t, "captura-de-leads")}
	renderer := &fakeRenderer{}
	publisher := &fakePublisher{}
	editor := &fakeEditor{pages: allRoutes(cleanHome), files: func() *site_usecase.View { return storage.view }}
	images := &tooltest.StubImages{}
	cc := tooltest.OwnerWithScreen(siteView, editor)
	cc.UserID = "user-1"
	cc.ChargeReference = "aichat:" + threadID
	cc.SeesImages = true
	return &rig{
		deps:    Deps{Storage: storage, Renderer: renderer, Publisher: publisher, Media: images, Pins: &fakePins{values: map[string]string{}}},
		storage: storage, renderer: renderer, publisher: publisher, editor: editor, images: images, cc: cc,
	}
}

func decoded(t *testing.T, data interface{}) map[string]interface{} {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("%s: %v", raw, err)
	}
	return out
}

func args(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatal(err)
	}
	return out
}
