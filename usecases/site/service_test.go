package site_usecase

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"vozko/domain/shared"
	"vozko/domain/site"
	"vozko/domain/sitepublish"
)

const (
	wsA    = "11111111-1111-4111-8111-111111111111"
	wsB    = "22222222-2222-4222-8222-222222222222"
	userA  = "33333333-3333-4333-8333-333333333333"
	userB  = "44444444-4444-4444-8444-444444444444"
	thread = "thread_9"
)

type harness struct {
	svc       *Service
	store     *memoryStore
	executor  *fakeExecutor
	preview   *fakePreviewer
	refs      *fakeReferences
	bindings  *fakeBindings
	forms     *fakeForms
	retired   *fakeRetirement
	published *fakePublished
	media     *fakeMedia
	numbers   *fakeNumbers
	hostnames *fakeHostnames
	cache     *fakeCache
	clock     *clock
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{
		store: newMemoryStore(), executor: &fakeExecutor{}, preview: &fakePreviewer{}, refs: &fakeReferences{refuse: map[string]error{}}, bindings: &fakeBindings{bound: map[string]bool{"contato/w1": true}}, forms: &fakeForms{}, retired: &fakeRetirement{}, published: &fakePublished{revisions: map[string]string{}},
		media: &fakeMedia{urls: map[string]string{}}, numbers: &fakeNumbers{}, hostnames: &fakeHostnames{}, cache: &fakeCache{},
		clock: &clock{now: time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)},
	}
	ids := &sequence{}
	svc, err := NewService(Deps{Store: h.store, Executor: h.executor, Previewer: h.preview, References: h.refs, Bindings: h.bindings, Forms: h.forms, Retirement: h.retired, Published: h.published, Media: h.media, Numbers: h.numbers, Hostnames: h.hostnames, Previews: h.cache, Now: h.clock.Now, NewID: ids.next})
	if err != nil {
		t.Fatal(err)
	}
	h.svc = svc
	return h
}

func (h *harness) create(t *testing.T, stack site.Stack, starter, name string) *View {
	t.Helper()
	v, err := h.svc.Create(context.Background(), CreateInput{WorkspaceID: wsA, UserID: userA, Name: name, Stack: stack, Starter: starter})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestNewServiceRefusesMissingDependencies(t *testing.T) {
	if _, err := NewService(Deps{}); !errors.Is(err, ErrMissingDependency) {
		t.Fatalf("got %v", err)
	}
}

func TestCreatingFromAStarterStoresFilesOnceSettingsAndAFirstVersion(t *testing.T) {
	h := newHarness(t)
	v := h.create(t, site.StackHTML, "captura-de-leads", "Clínica Sorriso")
	starter, _ := StarterFor(site.StackHTML, "captura-de-leads")
	p := v.Project
	if p.Slug != "clinica-sorriso" || p.Status != StatusDraft || p.Version != 1 || p.Stack != site.StackHTML || p.CreatedBy != userA {
		t.Fatalf("project %+v", p)
	}
	if v.Revision.Kind != RevisionCheckpoint || v.Revision.Author != PersonAuthor(userA) || v.Revision.Label != starter.Title || p.WorkingRevisionID != v.Revision.ID {
		t.Fatalf("revision %+v", v.Revision)
	}
	for path, content := range starter.Files {
		if v.Files[path].Content != content || v.Files[path].Rev != 1 {
			t.Fatalf("%s differs", path)
		}
	}
	if v.Settings == nil || v.Settings.Version != 1 || v.Settings.Settings.Forms["contato"].Success.Route != "/obrigado" {
		t.Fatalf("settings %+v", v.Settings)
	}
	unique := map[string]bool{}
	for _, content := range starter.Files {
		unique[contentSHA(content)] = true
	}
	if len(h.store.blobs) != len(unique) {
		t.Fatalf("blobs %d, distinct contents %d", len(h.store.blobs), len(unique))
	}
}

func TestCreateRefusesBadInput(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	cases := map[string]struct {
		in   CreateInput
		want error
	}{
		"unknown starter": {CreateInput{WorkspaceID: wsA, UserID: userA, Name: "A", Stack: site.StackHTML, Starter: "loja"}, ErrStarterUnknown},
		"unknown stack":   {CreateInput{WorkspaceID: wsA, UserID: userA, Name: "A", Stack: "vue", Starter: "blank"}, ErrStarterUnknown},
		"blank name":      {CreateInput{WorkspaceID: wsA, UserID: userA, Name: "  ", Stack: site.StackHTML, Starter: "blank"}, ErrNameInvalid},
		"long name":       {CreateInput{WorkspaceID: wsA, UserID: userA, Name: strings.Repeat("a", 121), Stack: site.StackHTML, Starter: "blank"}, ErrNameInvalid},
		"no workspace":    {CreateInput{UserID: userA, Name: "A", Stack: site.StackHTML, Starter: "blank"}, ErrWorkspaceRequired},
		"no user":         {CreateInput{WorkspaceID: wsA, Name: "A", Stack: site.StackHTML, Starter: "blank"}, ErrAuthorInvalid},
		"foreign design":  {CreateInput{WorkspaceID: wsA, UserID: userA, Name: "A", Stack: site.StackHTML, Starter: "blank", DesignSystemID: "nope"}, ErrDesignSystemNotFound},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := h.svc.Create(ctx, c.in); !errors.Is(err, c.want) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestCreateNamesThePageFromTheBriefWhenNoNameIsGiven(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	v, err := h.svc.Create(ctx, CreateInput{WorkspaceID: wsA, UserID: userA, Brief: "uma landing page para a Clínica Sorriso.\nCom formulário", Stack: site.StackReact, Starter: "blank"})
	if err != nil || v.Project.Name != "Uma landing page para a Clínica Sorriso" || v.Project.Slug != "uma-landing-page-para-a-clinica-sorriso" {
		t.Fatalf("got %+v %v", v, err)
	}
	named, err := h.svc.Create(ctx, CreateInput{WorkspaceID: wsA, UserID: userA, Name: "Promo", Brief: "outra coisa", Stack: site.StackReact, Starter: "blank"})
	if err != nil || named.Project.Name != "Promo" {
		t.Fatalf("a given name wins, got %+v %v", named, err)
	}
	if _, err := h.svc.Create(ctx, CreateInput{WorkspaceID: wsA, UserID: userA, Brief: "   ", Stack: site.StackReact, Starter: "blank"}); !errors.Is(err, ErrNameInvalid) {
		t.Fatalf("no name and no brief is refused, got %v", err)
	}
}

func TestSlugsStayUniqueAndAvoidReservedWords(t *testing.T) {
	h := newHarness(t)
	first := h.create(t, site.StackReact, "blank", "Promoção")
	second := h.create(t, site.StackReact, "blank", "Promoção")
	third := h.create(t, site.StackReact, "blank", "Promoção")
	reserved := h.create(t, site.StackReact, "blank", "Admin")
	tiny := h.create(t, site.StackReact, "blank", "!")
	got := []string{first.Project.Slug, second.Project.Slug, third.Project.Slug, reserved.Project.Slug, tiny.Project.Slug}
	want := []string{"promocao", "promocao-2", "promocao-3", "admin-pagina", "pagina"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	for _, slug := range got {
		if err := sitepublish.ValidateSlug(slug); err != nil {
			t.Fatalf("%s: %v", slug, err)
		}
	}
}

func TestANewPageInheritsTheDefaultDesignSystem(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	d, err := h.svc.CreateDesignSystem(ctx, wsA, site.DesignSystem{Name: "Marca"}, true)
	if err != nil {
		t.Fatal(err)
	}
	v := h.create(t, site.StackHTML, "blank", "Nova")
	if v.Project.DesignSystemID != d.ID {
		t.Fatalf("got %q", v.Project.DesignSystemID)
	}
}

func TestCreateWithAnEloOriginIsIdempotent(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	in := CreateInput{WorkspaceID: wsA, UserID: userA, Name: "Pela Elo", Stack: site.StackReact, Starter: "blank", Origin: Origin{ThreadID: thread, CommandID: "cmd-1"}}
	first, err := h.svc.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	again, err := h.svc.Create(ctx, in)
	if err != nil || again.Project.ID != first.Project.ID {
		t.Fatalf("got %+v %v", again, err)
	}
	in.UserID = userB
	if _, err := h.svc.Create(ctx, in); !errors.Is(err, ErrOriginTaken) {
		t.Fatalf("got %v", err)
	}
}

func TestGetIsScopedToTheWorkspaceAndFailsClosedOnAMissingBlob(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	v := h.create(t, site.StackHTML, "blank", "Nova")
	got, err := h.svc.Get(ctx, wsA, v.Project.ID)
	if err != nil || len(got.Files) != len(v.Files) || got.Settings == nil || got.Revision.ID != v.Revision.ID {
		t.Fatalf("got %+v %v", got, err)
	}
	if _, err := h.svc.Get(ctx, wsB, v.Project.ID); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("another workspace read the page: %v", err)
	}
	for key := range h.store.blobs {
		delete(h.store.blobs, key)
		break
	}
	if _, err := h.svc.Get(ctx, wsA, v.Project.ID); !errors.Is(err, ErrRevisionDamaged) {
		t.Fatalf("got %v", err)
	}
}

func saveFiles(h *harness, v *View, author Author, files map[string]string, deleted ...string) (*View, error) {
	return h.svc.SaveFiles(context.Background(), SaveFilesInput{WorkspaceID: wsA, ProjectID: v.Project.ID, ExpectedVersion: v.Project.Version, Author: author, Files: files, Deleted: deleted})
}

func TestSavingFilesBumpsRevsAndCoalescesAutosaveWithinTheWindow(t *testing.T) {
	h := newHarness(t)
	v := h.create(t, site.StackHTML, "blank", "Nova")
	me := PersonAuthor(userA)
	first, err := saveFiles(h, v, me, map[string]string{"pages/index.html": "<h1>Um</h1>\n", "pages/sobre.html": "<p>Sobre</p>\n"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Files["pages/index.html"].Rev != 2 || first.Files["pages/sobre.html"].Rev != 1 || first.Files[site.ThemePath].Rev != 1 || first.Project.Version != 2 {
		t.Fatalf("revs %+v version %d", first.Files, first.Project.Version)
	}
	if first.Revision.Kind != RevisionAutosave || first.Revision.ParentID != v.Revision.ID {
		t.Fatalf("revision %+v", first.Revision)
	}
	h.clock.advance(time.Minute)
	second, err := saveFiles(h, first, me, map[string]string{"pages/index.html": "<h1>Dois</h1>\n"})
	if err != nil {
		t.Fatal(err)
	}
	if second.Revision.ID != first.Revision.ID || !h.store.commits[len(h.store.commits)-1].Replace || second.Files["pages/index.html"].Rev != 3 {
		t.Fatalf("within the window the autosave is updated in place: %+v", second.Revision)
	}
	h.clock.advance(AutosaveWindow)
	third, err := saveFiles(h, second, me, map[string]string{"pages/index.html": "<h1>Três</h1>\n"})
	if err != nil || third.Revision.ID == second.Revision.ID || third.Revision.ParentID != second.Revision.ID {
		t.Fatalf("after the window a new version starts: %+v %v", third.Revision, err)
	}
	other, err := saveFiles(h, third, PersonAuthor(userB), map[string]string{"pages/index.html": "<h1>Quatro</h1>\n"})
	if err != nil || other.Revision.ID == third.Revision.ID {
		t.Fatalf("another person never edits my version in place: %+v %v", other.Revision, err)
	}
	list, total, err := h.svc.ListRevisions(context.Background(), wsA, v.Project.ID, 50, 0)
	if err != nil || total != 4 || len(list) != 4 {
		t.Fatalf("got %d %d %v", len(list), total, err)
	}
}

func TestSaveFilesRefusals(t *testing.T) {
	h := newHarness(t)
	v := h.create(t, site.StackHTML, "captura-de-leads", "Nova")
	me := PersonAuthor(userA)
	if _, err := saveFiles(h, v, me, map[string]string{site.ThemePath: v.Files[site.ThemePath].Content + "@plugin \"x\";\n"}); !refusedWith(err, site.CodeCSSDirectiveForbidden) {
		t.Fatalf("css lockdown: %v", err)
	}
	if _, err := saveFiles(h, v, me, map[string]string{"../etc/passwd": "x"}); !refusedWith(err, site.CodePathInvalid) {
		t.Fatalf("path rules: %v", err)
	}
	if _, err := saveFiles(h, v, me, map[string]string{"pages/grande.html": strings.Repeat("a", site.MaxProjectBytes+1)}); !refusedWith(err, site.CodeFileTooLarge) {
		t.Fatalf("size bound: %v", err)
	}
	if _, err := saveFiles(h, v, me, nil, "pages/obrigado.html"); !refusedWith(err, site.CodeRoutePageMissing) {
		t.Fatalf("a route page cannot vanish: %v", err)
	}
	if _, err := saveFiles(h, v, me, nil, "pages/nada.html"); !errors.Is(err, ErrPathNotFound) {
		t.Fatalf("got %v", err)
	}
	if _, err := saveFiles(h, v, Author("bot"), map[string]string{"pages/index.html": "x"}); !errors.Is(err, ErrAuthorInvalid) {
		t.Fatalf("got %v", err)
	}
	stale := *v
	stale.Project.Version = 7
	if _, err := saveFiles(h, &stale, me, map[string]string{"pages/index.html": "<p>x</p>"}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("got %v", err)
	}
	same, err := saveFiles(h, v, me, map[string]string{"pages/index.html": v.Files["pages/index.html"].Content})
	if err != nil || same.Project.Version != v.Project.Version || len(h.store.commits) != 0 {
		t.Fatalf("an unchanged save writes nothing: %v", err)
	}
}

func refusedWith(err error, code string) bool {
	var issues *IssuesError
	return errors.As(err, &issues) && issues.Code() == code && errors.Is(err, ErrFilesRefused)
}

func TestAnUntouchedOldIssueNeverBlocksASave(t *testing.T) {
	h := newHarness(t)
	v := h.create(t, site.StackHTML, "blank", "Nova")
	project := h.store.projects[v.Project.ID]
	rev := h.store.revisions[project.WorkingRevisionID]
	bad := "@import \"tailwindcss\";\n@config \"x.js\";\n"
	rev.Manifest[site.ThemePath] = ManifestEntry{SHA256: contentSHA(bad), Rev: 1}
	h.store.revisions[rev.ID] = rev
	h.store.blobs[blobKey(wsA, contentSHA(bad))] = []byte(bad)
	if _, err := saveFiles(h, v, PersonAuthor(userA), map[string]string{"pages/index.html": "<p>ok</p>\n"}); err != nil {
		t.Fatalf("got %v", err)
	}
}

func TestArchivedPagesCannotBeEdited(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	v := h.create(t, site.StackHTML, "blank", "Nova")
	archived := true
	p, err := h.svc.Update(ctx, wsA, v.Project.ID, v.Project.Version, Change{Archived: &archived})
	if err != nil || p.Status != StatusArchived {
		t.Fatalf("got %+v %v", p, err)
	}
	v.Project.Version = p.Version
	if _, err := saveFiles(h, v, PersonAuthor(userA), map[string]string{"pages/index.html": "<p>x</p>"}); !errors.Is(err, ErrProjectArchived) {
		t.Fatalf("got %v", err)
	}
	archived = false
	p, err = h.svc.Update(ctx, wsA, v.Project.ID, p.Version, Change{Archived: &archived})
	if err != nil || p.Status != StatusDraft {
		t.Fatalf("unarchive: %+v %v", p, err)
	}
}

func setTextBatch(node string, rev uint64, text string) json.RawMessage {
	raw, _ := json.Marshal([]map[string]any{{"op": "set_text", "node": node, "rev": rev, "text": text}})
	return raw
}

func TestAnEloEditRunsTheBuilderAndCheckpointsAVersion(t *testing.T) {
	h := newHarness(t)
	v := h.create(t, site.StackHTML, "blank", "Nova")
	page := "pages/index.html"
	h.executor.steps = func(req ExecuteRequest) []site.StepPatch {
		return []site.StepPatch{{Op: site.OpSetText, Index: 0, Files: map[string]string{page: strings.Replace(req.Project.Files[page].Content, "Comece", "Começe já", 1)}}}
	}
	res, err := h.svc.Edit(context.Background(), EditInput{
		WorkspaceID: wsA, ProjectID: v.Project.ID, ExpectedVersion: 1, Author: EloAuthor(thread), Label: "Título novo",
		Batch: setTextBatch(page+"#"+offsetOf(v.Files[page].Content, "<h1"), 1, "Começe já"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(h.executor.calls) != 1 || h.executor.calls[0].ProjectID != v.Project.ID || len(h.executor.calls[0].Batch.Operations) != 1 {
		t.Fatalf("calls %+v", h.executor.calls)
	}
	if !strings.Contains(res.View.Files[page].Content, "Começe já") || res.View.Files[page].Rev != 2 || len(res.Steps) != 1 {
		t.Fatalf("result %+v", res)
	}
	if res.View.Revision.Kind != RevisionElo || res.View.Revision.Author != EloAuthor(thread) || res.View.Revision.Label != "Título novo" || res.View.Project.Version != 2 {
		t.Fatalf("revision %+v", res.View.Revision)
	}
}

func offsetOf(content, needle string) string {
	return strconv.Itoa(strings.Index(content, needle))
}

func TestAPersonsVisualEditIsAnAutosave(t *testing.T) {
	h := newHarness(t)
	v := h.create(t, site.StackHTML, "blank", "Nova")
	page := "pages/index.html"
	h.executor.steps = func(req ExecuteRequest) []site.StepPatch {
		return []site.StepPatch{{Op: site.OpSetText, Files: map[string]string{page: req.Project.Files[page].Content + "\n"}}}
	}
	res, err := h.svc.Edit(context.Background(), EditInput{WorkspaceID: wsA, ProjectID: v.Project.ID, ExpectedVersion: 1, Author: PersonAuthor(userA),
		Batch: setTextBatch(page+"#"+offsetOf(v.Files[page].Content, "<h1"), 1, "Oi")})
	if err != nil || res.View.Revision.Kind != RevisionAutosave {
		t.Fatalf("got %+v %v", res, err)
	}
}

func TestEditRefusals(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	v := h.create(t, site.StackHTML, "blank", "Nova")
	page := "pages/index.html"
	node := page + "#" + offsetOf(v.Files[page].Content, "<h1")
	edit := func(batch json.RawMessage) error {
		_, err := h.svc.Edit(ctx, EditInput{WorkspaceID: wsA, ProjectID: v.Project.ID, ExpectedVersion: 1, Author: PersonAuthor(userA), Batch: batch})
		return err
	}
	if err := edit(json.RawMessage(`{"op":"set_text"}`)); !errors.Is(err, site.ErrBatchMalformed) {
		t.Fatalf("malformed: %v", err)
	}
	var refusal *EditRefusal
	if err := edit(json.RawMessage(`[{"op":"delete_file","path":"pages/nada.html"}]`)); !errors.As(err, &refusal) || len(refusal.Failures) != 1 || !errors.Is(err, site.ErrEditRefused) {
		t.Fatalf("prepare refusal: %v", err)
	}
	if len(h.executor.calls) != 0 {
		t.Fatal("a refused batch never reaches the builder")
	}
	h.executor.result = ExecuteResult{Failures: []site.Failure{{Index: 0, Code: site.CodeNodeStale}}}
	if err := edit(setTextBatch(node, 1, "Oi")); !errors.As(err, &refusal) || refusal.Failures[0].Code != site.CodeNodeStale {
		t.Fatalf("builder refusal: %v", err)
	}
	h.executor.result = ExecuteResult{Failures: []site.Failure{{Index: 0, Code: site.CodeEditNotFound, Field: "old", Detail: "nearest match at line 3: <h1>Oi</h1>"}}}
	if err := edit(setTextBatch(node, 1, "Oi")); !errors.As(err, &refusal) || refusal.Failures[0].Detail != "nearest match at line 3: <h1>Oi</h1>" {
		t.Fatalf("the builder's nearest match reaches the refusal: %v", err)
	}
	h.executor.result = ExecuteResult{Steps: []site.StepPatch{{Op: site.OpSetText, Deleted: []string{"pages/nada.html"}}}}
	if err := edit(setTextBatch(node, 1, "Oi")); !errors.Is(err, ErrStepsInvalid) {
		t.Fatalf("invalid steps: %v", err)
	}
	h.executor.result = ExecuteResult{Steps: []site.StepPatch{{Op: site.OpSetText, Files: map[string]string{site.ThemePath: "@import \"tailwindcss\";\n@plugin \"x\";\n"}}}}
	if err := edit(setTextBatch(node, 1, "Oi")); !errors.As(err, &refusal) || len(refusal.Issues) == 0 || refusal.Issues[0].Code != site.CodeCSSDirectiveForbidden {
		t.Fatalf("a structural issue refuses the whole batch: %v", err)
	}
	h.executor.err = errors.New("dial tcp: refused")
	if err := edit(setTextBatch(node, 1, "Oi")); !errors.Is(err, ErrBuilderUnavailable) {
		t.Fatalf("builder down: %v", err)
	}
	if len(h.store.commits) != 0 {
		t.Fatal("nothing is stored after a refusal")
	}
}

func TestVersionsCheckpointRestoreAndPublishSnapshots(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	me := PersonAuthor(userA)
	v := h.create(t, site.StackHTML, "blank", "Nova")
	page := "pages/index.html"
	original := v.Files[page].Content
	edited, err := saveFiles(h, v, me, map[string]string{page: "<h1>Novo</h1>\n"})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := h.svc.SaveVersion(ctx, SaveVersionInput{WorkspaceID: wsA, ProjectID: v.Project.ID, ExpectedVersion: edited.Project.Version, Author: me, Label: "Antes da Elo"})
	if err != nil || saved.Revision.Kind != RevisionCheckpoint || saved.Revision.Label != "Antes da Elo" || saved.Files[page].Content != "<h1>Novo</h1>\n" {
		t.Fatalf("checkpoint %+v %v", saved, err)
	}
	if _, err := h.svc.SaveVersion(ctx, SaveVersionInput{WorkspaceID: wsA, ProjectID: v.Project.ID, ExpectedVersion: saved.Project.Version, Author: me, Label: strings.Repeat("a", 121)}); !errors.Is(err, ErrLabelInvalid) {
		t.Fatalf("got %v", err)
	}
	restored, err := h.svc.Restore(ctx, RestoreInput{WorkspaceID: wsA, ProjectID: v.Project.ID, RevisionID: v.Revision.ID, ExpectedVersion: saved.Project.Version, Author: me})
	if err != nil {
		t.Fatal(err)
	}
	if restored.Revision.Kind != RevisionRestore || restored.Files[page].Content != original || restored.Files[page].Rev != 3 || restored.Revision.ParentID != saved.Revision.ID {
		t.Fatalf("restore %+v %+v", restored.Revision, restored.Files[page])
	}
	if restored.Files[site.ThemePath].Rev != 1 {
		t.Fatalf("an unchanged file keeps its rev: %+v", restored.Files[site.ThemePath])
	}
	if _, err := h.svc.Restore(ctx, RestoreInput{WorkspaceID: wsA, ProjectID: v.Project.ID, RevisionID: "missing", ExpectedVersion: restored.Project.Version, Author: me}); !errors.Is(err, ErrRevisionNotFound) {
		t.Fatalf("got %v", err)
	}
	source, err := h.svc.RevisionSource(ctx, wsA, v.Project.ID, saved.Revision.ID)
	if err != nil || source.Files[page].Content != "<h1>Novo</h1>\n" {
		t.Fatalf("source %+v %v", source, err)
	}
	if _, err := h.svc.RevisionSource(ctx, wsB, v.Project.ID, saved.Revision.ID); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestSettingsAreValidatedStrictlyAndAgainstTheWorkspace(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	v := h.create(t, site.StackHTML, "blank", "Nova")
	raw := []byte(`{"forms":{"contato":{"funnel_id":"f1","stage_id":"s1","workflow_id":"w1","follow_up":{"kind":"whatsapp","id":"p1"}}}}`)
	saved, err := h.svc.PutSettings(ctx, wsA, v.Project.ID, 1, raw)
	if err != nil || saved.Version != 2 || saved.Settings.Forms["contato"].WorkflowID != "w1" || h.refs.seen[0] != wsA+"/contato" {
		t.Fatalf("got %+v %v", saved, err)
	}
	if _, err := h.svc.PutSettings(ctx, wsA, v.Project.ID, 2, []byte(`{"theme":"x"}`)); !errors.Is(err, site.ErrSettingsInvalid) {
		t.Fatalf("got %v", err)
	}
	if _, err := h.svc.PutSettings(ctx, wsA, v.Project.ID, 2, []byte(`{"forms":{"contato":{"workflow_id":"w1"}}}`)); !errors.Is(err, site.ErrFormChannelRequired) {
		t.Fatalf("got %v", err)
	}
	h.refs.refuse["contato"] = ErrFormWorkflowUnknown
	var refErr *FormReferenceError
	if _, err := h.svc.PutSettings(ctx, wsA, v.Project.ID, 2, raw); !errors.As(err, &refErr) || !errors.Is(err, ErrFormWorkflowUnknown) || refErr.Form != "contato" {
		t.Fatalf("got %v", err)
	}
	delete(h.refs.refuse, "contato")
	if _, err := h.svc.PutSettings(ctx, wsA, v.Project.ID, 1, raw); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("got %v", err)
	}
	if _, err := h.svc.PutSettings(ctx, wsB, v.Project.ID, 2, raw); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestDesignSystemsKeepOneDefaultPerWorkspace(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	first, err := h.svc.CreateDesignSystem(ctx, wsA, site.DesignSystem{Name: "Um"}, true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.svc.CreateDesignSystem(ctx, wsA, site.DesignSystem{Name: "Dois", Tokens: map[string]string{"--color-brand": "#0f766e"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	list, _ := h.svc.ListDesignSystems(ctx, wsA)
	defaults := 0
	for _, d := range list {
		if d.IsDefault {
			defaults++
			if d.ID != second.ID {
				t.Fatalf("the newest default wins: %+v", d)
			}
		}
	}
	if defaults != 1 {
		t.Fatalf("defaults %d", defaults)
	}
	if _, err := h.svc.CreateDesignSystem(ctx, wsA, site.DesignSystem{Name: " "}, false); !errors.Is(err, site.ErrDesignSystemInvalid) {
		t.Fatalf("got %v", err)
	}
	updated, err := h.svc.UpdateDesignSystem(ctx, wsA, first.ID, first.Version, site.DesignSystem{Name: "Um novo"}, false)
	if err != nil || updated.System.Name != "Um novo" || updated.Version != 2 {
		t.Fatalf("got %+v %v", updated, err)
	}
	if _, err := h.svc.UpdateDesignSystem(ctx, wsA, first.ID, 1, site.DesignSystem{Name: "x"}, false); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("got %v", err)
	}
	if _, err := h.svc.GetDesignSystem(ctx, wsB, first.ID); !errors.Is(err, ErrDesignSystemNotFound) {
		t.Fatalf("got %v", err)
	}
	v := h.create(t, site.StackHTML, "blank", "Nova")
	if err := h.svc.DeleteDesignSystem(ctx, wsA, second.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := h.svc.Get(ctx, wsA, v.Project.ID)
	if got.Project.DesignSystemID != "" {
		t.Fatal("a deleted design system is unlinked from its pages")
	}
}

func TestPreviewBuildsTheWorkingVersionWithItsSettings(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	v := h.create(t, site.StackHTML, "captura-de-leads", "Nova")
	res, err := h.svc.Preview(ctx, wsA, v.Project.ID)
	if err != nil || res.URL == "" || res.RevisionID != v.Revision.ID {
		t.Fatalf("got %+v %v", res, err)
	}
	if h.preview.got.ProjectID != v.Project.ID || len(h.preview.got.Project.Files) != len(v.Files) || h.preview.got.Settings.Forms["contato"].Success == nil {
		t.Fatalf("request %+v", h.preview.got)
	}
	h.preview.err = errors.New("timeout")
	if _, err := h.svc.Preview(ctx, wsA, v.Project.ID); !errors.Is(err, ErrPreviewUnavailable) {
		t.Fatalf("got %v", err)
	}
}

func TestExportZipsTheSourceFiles(t *testing.T) {
	h := newHarness(t)
	v := h.create(t, site.StackReact, "captura-de-leads", "Clínica Sorriso")
	name, body, err := h.svc.Export(context.Background(), wsA, v.Project.ID)
	if err != nil || name != "clinica-sorriso.zip" {
		t.Fatalf("got %q %v", name, err)
	}
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	if len(reader.File) != len(v.Files) {
		t.Fatalf("got %d files", len(reader.File))
	}
	for _, f := range reader.File {
		rc, _ := f.Open()
		content, _ := io.ReadAll(rc)
		rc.Close()
		if string(content) != v.Files[f.Name].Content {
			t.Fatalf("%s differs", f.Name)
		}
	}
}

func TestRenameSlugArchiveAndDeleteRules(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	v := h.create(t, site.StackHTML, "blank", "Nova")
	h.create(t, site.StackHTML, "blank", "Outra")
	name, slug := "Clínica Nova", "clinica-nova"
	p, err := h.svc.Update(ctx, wsA, v.Project.ID, 1, Change{Name: &name, Slug: &slug})
	if err != nil || p.Name != name || p.Slug != slug || p.Version != 2 {
		t.Fatalf("got %+v %v", p, err)
	}
	for bad, want := range map[string]error{"www": sitepublish.ErrSlugReserved, "A B": sitepublish.ErrSlugInvalid, "outra": ErrSlugTaken} {
		s := bad
		if _, err := h.svc.Update(ctx, wsA, v.Project.ID, p.Version, Change{Slug: &s}); !errors.Is(err, want) {
			t.Fatalf("%s: got %v", bad, err)
		}
	}
	blank := " "
	if _, err := h.svc.Update(ctx, wsA, v.Project.ID, p.Version, Change{Name: &blank}); !errors.Is(err, ErrNameInvalid) {
		t.Fatalf("got %v", err)
	}
	if _, err := h.svc.Update(ctx, wsA, v.Project.ID, 1, Change{Name: &name}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("got %v", err)
	}
	live := h.store.projects[v.Project.ID]
	live.Status = StatusPublished
	h.store.projects[v.Project.ID] = live
	archived := true
	if _, err := h.svc.Update(ctx, wsA, v.Project.ID, live.Version, Change{Archived: &archived}); !errors.Is(err, ErrProjectPublished) {
		t.Fatalf("got %v", err)
	}
	moved := "outra-coisa"
	if _, err := h.svc.Update(ctx, wsA, v.Project.ID, live.Version, Change{Slug: &moved}); !errors.Is(err, ErrProjectPublished) {
		t.Fatalf("a live address never changes here: %v", err)
	}
	if err := h.svc.Delete(ctx, wsA, v.Project.ID); !errors.Is(err, ErrProjectPublished) {
		t.Fatalf("got %v", err)
	}
	live.Status = StatusDraft
	h.store.projects[v.Project.ID] = live
	if err := h.svc.Delete(ctx, wsB, v.Project.ID); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("got %v", err)
	}
	if len(h.forms.deactivated) != 0 {
		t.Fatalf("refused deletes leave the forms alone: %v", h.forms.deactivated)
	}
	h.forms.err = errors.New("forms store down")
	if err := h.svc.Delete(ctx, wsA, v.Project.ID); !errors.Is(err, h.forms.err) {
		t.Fatalf("a page whose forms cannot be turned off is kept: %v", err)
	}
	if _, err := h.svc.Get(ctx, wsA, v.Project.ID); err != nil {
		t.Fatalf("still there: %v", err)
	}
	h.forms.err = nil
	h.retired.err = errors.New("hosting down")
	if err := h.svc.Delete(ctx, wsA, v.Project.ID); !errors.Is(err, h.retired.err) {
		t.Fatalf("a page whose addresses cannot be dropped is kept: %v", err)
	}
	h.retired.err = nil
	if err := h.svc.Delete(ctx, wsA, v.Project.ID); err != nil {
		t.Fatal(err)
	}
	if len(h.retired.calls) != 2 || h.retired.calls[1] != wsA+"|"+v.Project.ID {
		t.Fatalf("deleting a page drops every published address: %v", h.retired.calls)
	}
	if len(h.forms.deactivated) != 2 || h.forms.deactivated[1] != v.Project.ID {
		t.Fatalf("deleting a page turns its forms off: %v", h.forms.deactivated)
	}
	if _, err := h.svc.Get(ctx, wsA, v.Project.ID); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestListHidesArchivedPagesUnlessAsked(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	a := h.create(t, site.StackHTML, "blank", "Ativa")
	b := h.create(t, site.StackHTML, "blank", "Antiga")
	archived := true
	if _, err := h.svc.Update(ctx, wsA, b.Project.ID, 1, Change{Archived: &archived}); err != nil {
		t.Fatal(err)
	}
	list, total, err := h.svc.List(ctx, ListQuery{WorkspaceID: wsA, Limit: 10})
	if err != nil || total != 1 || list[0].ID != a.Project.ID {
		t.Fatalf("got %+v %d %v", list, total, err)
	}
	list, _, _ = h.svc.List(ctx, ListQuery{WorkspaceID: wsA, Status: StatusArchived, Limit: 10})
	if len(list) != 1 || list[0].ID != b.Project.ID {
		t.Fatalf("got %+v", list)
	}
	if _, _, err := h.svc.List(ctx, ListQuery{WorkspaceID: wsA, Status: "deleted"}); !errors.Is(err, ErrStatusInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestPublicationStateMovesWithoutTouchingTheContentVersion(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	v := h.create(t, site.StackHTML, "blank", "Nova")
	p, err := h.svc.SetPublication(ctx, wsA, v.Project.ID, StatusPublished, "dep-1")
	if err != nil || p.Status != StatusPublished || p.PublishedDeploymentID != "dep-1" || p.Version != v.Project.Version {
		t.Fatalf("got %+v %v", p, err)
	}
	p, err = h.svc.SetPublication(ctx, wsA, v.Project.ID, StatusSuspended, "dep-1")
	if err != nil || p.Status != StatusSuspended {
		t.Fatalf("got %+v %v", p, err)
	}
	p, err = h.svc.SetPublication(ctx, wsA, v.Project.ID, StatusDraft, "")
	if err != nil || p.Status != StatusDraft || p.PublishedDeploymentID != "" {
		t.Fatalf("got %+v %v", p, err)
	}
	for _, bad := range []struct {
		status Status
		dep    string
	}{{StatusPublished, ""}, {StatusArchived, ""}, {StatusDraft, "dep-1"}, {"live", "dep-1"}} {
		if _, err := h.svc.SetPublication(ctx, wsA, v.Project.ID, bad.status, bad.dep); !errors.Is(err, ErrPublicationInvalid) {
			t.Fatalf("%+v: got %v", bad, err)
		}
	}
	if _, err := h.svc.SetPublication(ctx, wsB, v.Project.ID, StatusPublished, "dep-1"); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("got %v", err)
	}
	archived := true
	if _, err := h.svc.Update(ctx, wsA, v.Project.ID, v.Project.Version, Change{Archived: &archived}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.SetPublication(ctx, wsA, v.Project.ID, StatusPublished, "dep-1"); !errors.Is(err, ErrProjectArchived) {
		t.Fatalf("got %v", err)
	}
}

func TestPreviewPassesBuildFailuresAndProjectIssuesThrough(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	v := h.create(t, site.StackHTML, "blank", "Nova")
	h.preview.err = &BuildFailedError{Errors: []BuildIssue{{Path: "pages/index.html", Line: 3, Column: 7, Message: "Unexpected token"}}}
	var failed *BuildFailedError
	if _, err := h.svc.Preview(ctx, wsA, v.Project.ID); !errors.As(err, &failed) || failed.Errors[0].Line != 3 || errors.Is(err, ErrPreviewUnavailable) {
		t.Fatalf("got %v", err)
	}
	h.preview.err = &IssuesError{Issues: []site.Issue{{Code: site.CodeImportForbidden, Path: "scripts/a.js"}}}
	var issues *IssuesError
	if _, err := h.svc.Preview(ctx, wsA, v.Project.ID); !errors.As(err, &issues) || issues.Code() != site.CodeImportForbidden {
		t.Fatalf("got %v", err)
	}
}

func TestTheViewSaysWhenTheWorkingVersionIsNotTheLiveOne(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	v := h.create(t, site.StackHTML, "blank", "Nova")
	got, err := h.svc.Get(ctx, wsA, v.Project.ID)
	if err != nil || !got.HasUnpublishedChanges {
		t.Fatalf("a page never published has unpublished changes: %+v %v", got, err)
	}
	live := h.store.projects[v.Project.ID]
	live.Status, live.PublishedDeploymentID = StatusPublished, "dep-1"
	h.store.projects[v.Project.ID] = live
	h.published.revisions["dep-1"] = live.WorkingRevisionID
	if got, err = h.svc.Get(ctx, wsA, v.Project.ID); err != nil || got.HasUnpublishedChanges {
		t.Fatalf("the live revision is the working one: %+v %v", got, err)
	}
	if _, err := saveFiles(h, got, PersonAuthor(userA), map[string]string{"pages/index.html": "<h1>Mudou</h1>\n"}); err != nil {
		t.Fatal(err)
	}
	if got, err = h.svc.Get(ctx, wsA, v.Project.ID); err != nil || !got.HasUnpublishedChanges {
		t.Fatalf("an edit after the publish is unpublished: %+v %v", got, err)
	}
	h.published.err = errors.New("deployments down")
	if _, err := h.svc.Get(ctx, wsA, v.Project.ID); !errors.Is(err, h.published.err) {
		t.Fatalf("an unreadable deployment refuses instead of guessing: %v", err)
	}
}

func TestDuplicateCopiesTheSourceSettingsAndDesignButNeverThePublication(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	v := h.create(t, site.StackHTML, "captura-de-leads", "Clínica Sorriso")
	edited, err := saveFiles(h, v, PersonAuthor(userA), map[string]string{"pages/obrigado.html": "<h1>Valeu</h1>\n"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.PutSettings(ctx, wsA, v.Project.ID, v.Settings.Version, []byte(`{"consent":{"banner":true}}`)); err != nil {
		t.Fatal(err)
	}
	live := h.store.projects[v.Project.ID]
	live.Status, live.PublishedDeploymentID, live.DesignSystemID = StatusPublished, "dep-1", "design-1"
	h.store.projects[v.Project.ID] = live
	copied, err := h.svc.Duplicate(ctx, DuplicateInput{WorkspaceID: wsA, UserID: userA, ProjectID: v.Project.ID})
	if err != nil {
		t.Fatal(err)
	}
	p := copied.Project
	if p.ID == v.Project.ID || p.Name != "Clínica Sorriso (cópia)" || p.Slug == live.Slug || p.Status != StatusDraft || p.PublishedDeploymentID != "" || p.DesignSystemID != "design-1" || p.Version != 1 || p.CreatedBy != userA {
		t.Fatalf("copy %+v", p)
	}
	if copied.Files["pages/obrigado.html"].Content != "<h1>Valeu</h1>\n" || len(copied.Files) != len(edited.Files) || !copied.Settings.Settings.Consent.Banner {
		t.Fatalf("files %d settings %+v", len(copied.Files), copied.Settings.Settings)
	}
	named, err := h.svc.Duplicate(ctx, DuplicateInput{WorkspaceID: wsA, UserID: userA, ProjectID: v.Project.ID, Name: "Promo de verão"})
	if err != nil || named.Project.Name != "Promo de verão" {
		t.Fatalf("named %+v %v", named, err)
	}
	if _, err := h.svc.Duplicate(ctx, DuplicateInput{WorkspaceID: wsB, UserID: userA, ProjectID: v.Project.ID}); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("another workspace cannot copy it: %v", err)
	}
}

func TestTheSettingsWorkflowMustBeTheOneThisFormTriggersAndDraftFormsGetKeys(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	v := h.create(t, site.StackHTML, "blank", "Nova")
	raw := []byte(`{"forms":{"contato":{"workflow_id":"w1","follow_up":{"kind":"whatsapp","id":"p1"}},"news":{}}}`)
	if _, err := h.svc.PutSettings(ctx, wsA, v.Project.ID, 1, raw); err != nil {
		t.Fatal(err)
	}
	if strings.Join(h.bindings.keyed, ",") != v.Project.ID+":contato,news" {
		t.Fatalf("keys %v", h.bindings.keyed)
	}
	other := []byte(`{"forms":{"contato":{"workflow_id":"w9","follow_up":{"kind":"whatsapp","id":"p1"}}}}`)
	var refErr *FormReferenceError
	if _, err := h.svc.PutSettings(ctx, wsA, v.Project.ID, 2, other); !errors.As(err, &refErr) || !errors.Is(err, ErrFormWorkflowNotBound) || refErr.Form != "contato" {
		t.Fatalf("got %v", err)
	}
	h.bindings.err = errors.New("db down")
	if _, err := h.svc.PutSettings(ctx, wsA, v.Project.ID, 2, raw); err == nil {
		t.Fatal("an unreadable binding refuses the save")
	}
}

func TestTheSiteChatMustBeAWidgetOfTheWorkspace(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	v := h.create(t, site.StackHTML, "blank", "Nova")
	if _, err := h.svc.PutSettings(ctx, wsA, v.Project.ID, 1, []byte(`{"site":{"webchat_widget":"pk_ours"}}`)); err != nil {
		t.Fatal(err)
	}
	h.refs.refuse["webchat"] = ErrWebchatWidgetUnknown
	if _, err := h.svc.PutSettings(ctx, wsA, v.Project.ID, 2, []byte(`{"site":{"webchat_widget":"pk_theirs"}}`)); !errors.Is(err, ErrWebchatWidgetUnknown) {
		t.Fatalf("got %v", err)
	}
}

func TestLegacyFormSettingsWithoutADealOwnerStayReadableAndOnlyRefuseWhenThatFormIsSaved(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	v := h.create(t, site.StackHTML, "blank", "Nova")
	legacy := &StoredSettings{ProjectID: v.Project.ID, WorkspaceID: wsA, Settings: site.Settings{Forms: map[string]site.FormSettings{"contato": {FunnelID: "f1"}}}}
	if err := h.store.SaveSettings(ctx, legacy, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Get(ctx, wsA, v.Project.ID); err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := h.svc.GetSettings(ctx, wsA, v.Project.ID); err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if _, err := h.svc.Preview(ctx, wsA, v.Project.ID); err != nil {
		t.Fatalf("preview: %v", err)
	}
	unrelated := []byte(`{"forms":{"contato":{"funnel_id":"f1"}},"pages":{"/":{"title":"Oferta"}}}`)
	saved, err := h.svc.PutSettings(ctx, wsA, v.Project.ID, 2, unrelated)
	if err != nil {
		t.Fatalf("an unrelated save keeps working: %v", err)
	}
	changed := []byte(`{"forms":{"contato":{"funnel_id":"f1","stage_id":"s1"}},"pages":{"/":{"title":"Oferta"}}}`)
	if _, err := h.svc.PutSettings(ctx, wsA, v.Project.ID, saved.Version, changed); !errors.Is(err, site.ErrFormDealOwnerRequired) || site.ErrorCode(err) != site.CodeFormDealOwnerRequired {
		t.Fatalf("saving that form without an owner is refused: %v", err)
	}
	added := []byte(`{"forms":{"contato":{"funnel_id":"f1"},"novo":{"funnel_id":"f1"}},"pages":{"/":{"title":"Oferta"}}}`)
	if _, err := h.svc.PutSettings(ctx, wsA, v.Project.ID, saved.Version, added); !errors.Is(err, site.ErrFormDealOwnerRequired) {
		t.Fatalf("a new form without an owner is refused: %v", err)
	}
}

func TestLegacyUnboundWorkflowsAndChatsDoNotBlockUnrelatedSaves(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	v := h.create(t, site.StackHTML, "blank", "Nova")
	legacy := &StoredSettings{ProjectID: v.Project.ID, WorkspaceID: wsA, Settings: site.Settings{
		Forms: map[string]site.FormSettings{"contato": {WorkflowID: "w9", FollowUp: &site.FollowUp{Kind: site.FollowUpWhatsApp, ID: "p1"}}},
		Site:  site.Identity{WebchatWidget: "pk_gone"},
	}}
	if err := h.store.SaveSettings(ctx, legacy, 1); err != nil {
		t.Fatal(err)
	}
	h.refs.refuse["webchat"] = ErrWebchatWidgetUnknown
	raw := []byte(`{"forms":{"contato":{"workflow_id":"w9","follow_up":{"kind":"whatsapp","id":"p1"}}},"site":{"webchat_widget":"pk_gone"},"pages":{"/":{"title":"Oferta"}}}`)
	if _, err := h.svc.PutSettings(ctx, wsA, v.Project.ID, 2, raw); err != nil {
		t.Fatalf("unchanged legacy references never block another save: %v", err)
	}
}

func TestSavingABookingReservesItsKeyBesideTheForms(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	v := h.create(t, site.StackHTML, "blank", "Nova")
	raw := []byte(`{"forms":{"contato":{}},"bookings":{"agenda":{"duration_minutes":30,"horizon_days":7,"title":"Avaliação com {nome}","consent_text":"Aceito","hours":{"timezone":"America/Sao_Paulo","days":{"mon":[{"start":"09:00","end":"12:00"}]}}}}}`)
	if _, err := h.svc.PutSettings(ctx, wsA, v.Project.ID, 1, raw); err != nil {
		t.Fatal(err)
	}
	if strings.Join(h.bindings.keyed, ",") != v.Project.ID+":agenda,contato" {
		t.Fatalf("keys %v", h.bindings.keyed)
	}
	bad := []byte(`{"bookings":{"agenda":{"duration_minutes":3,"horizon_days":7,"title":"x","consent_text":"Aceito","hours":{"timezone":"America/Sao_Paulo","days":{"mon":[{"start":"09:00","end":"12:00"}]}}}}}`)
	if _, err := h.svc.PutSettings(ctx, wsA, v.Project.ID, 2, bad); !errors.Is(err, site.ErrBookingDurationInvalid) {
		t.Fatalf("got %v", err)
	}
}
