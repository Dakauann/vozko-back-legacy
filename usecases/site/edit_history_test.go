package site_usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"vozko/domain/site"
	"vozko/domain/sitepublish"
)

func TestAStaleVisualEditReachesTheBuilderWithTheFileAtThatRev(t *testing.T) {
	h := newHarness(t)
	v := h.create(t, site.StackHTML, "blank", "Nova")
	page := "pages/index.html"
	original := v.Files[page].Content
	node := page + "#" + offsetOf(original, "<h1")
	me := PersonAuthor(userA)
	first, err := saveFiles(h, v, me, map[string]string{page: "<p>topo</p>\n" + original})
	if err != nil {
		t.Fatal(err)
	}
	h.clock.advance(time.Minute)
	second, err := saveFiles(h, first, me, map[string]string{page: "<p>topo novo</p>\n" + original})
	if err != nil || second.Revision.ID != first.Revision.ID || second.Files[page].Rev != 3 {
		t.Fatalf("the second save coalesces into the same autosave: %+v %v", second.Revision, err)
	}
	h.executor.steps = func(req ExecuteRequest) []site.StepPatch {
		return []site.StepPatch{{Op: site.OpSetText, Files: map[string]string{page: strings.Replace(req.Project.Files[page].Content, "<h1", "<h1 data-x", 1)}}}
	}
	batch, _ := json.Marshal([]map[string]any{
		{"op": "set_text", "node": node, "rev": 1, "text": "Oi"},
		{"op": "set_text", "node": page + "#0", "rev": 2, "text": "Topo"},
	})
	if _, err := h.svc.Edit(context.Background(), EditInput{WorkspaceID: wsA, ProjectID: v.Project.ID, ExpectedVersion: second.Project.Version, Author: EloAuthor(thread), Batch: batch}); err != nil {
		t.Fatal(err)
	}
	bases := h.executor.calls[0].Bases[page]
	if len(bases) != 2 || bases[0] != (site.File{Content: original, Rev: 1}) || bases[1] != (site.File{Content: "<p>topo</p>\n" + original, Rev: 2}) {
		t.Fatalf("got %+v", h.executor.calls[0].Bases)
	}
}

func TestEveryCommitRecordsTheNewFileRevs(t *testing.T) {
	h := newHarness(t)
	v := h.create(t, site.StackHTML, "blank", "Nova")
	if len(h.store.created[0].FileRevs) != len(v.Files) {
		t.Fatalf("a new page records every file: %+v", h.store.created[0].FileRevs)
	}
	if _, err := saveFiles(h, v, PersonAuthor(userA), map[string]string{"pages/index.html": "<h1>Um</h1>\n"}); err != nil {
		t.Fatal(err)
	}
	revs := h.store.commits[len(h.store.commits)-1].FileRevs
	if len(revs) != 1 || revs[0].Path != "pages/index.html" || revs[0].Rev != 2 || revs[0].SHA256 != contentSHA("<h1>Um</h1>\n") {
		t.Fatalf("only the changed file is recorded: %+v", revs)
	}
}

func TestAnUnreadableHistoryRefusesTheEdit(t *testing.T) {
	h := newHarness(t)
	v := h.create(t, site.StackHTML, "blank", "Nova")
	page := "pages/index.html"
	saved, err := saveFiles(h, v, PersonAuthor(userA), map[string]string{page: "<p>x</p>\n" + v.Files[page].Content})
	if err != nil {
		t.Fatal(err)
	}
	h.store.fileRevsErr = errors.New("db down")
	_, err = h.svc.Edit(context.Background(), EditInput{WorkspaceID: wsA, ProjectID: v.Project.ID, ExpectedVersion: saved.Project.Version, Author: PersonAuthor(userA), Batch: setTextBatch(page+"#0", 1, "Oi")})
	if err == nil || len(h.executor.calls) != 0 {
		t.Fatalf("got %v", err)
	}
}

func TestEditFailuresAndStepsPointAtTheOperationsTheCallerSent(t *testing.T) {
	h := newHarness(t)
	v := h.create(t, site.StackHTML, "blank", "Nova")
	page := "pages/index.html"
	node := page + "#" + offsetOf(v.Files[page].Content, "<h1")
	batch, _ := json.Marshal([]map[string]any{
		{"op": "set_text", "node": node, "rev": 1, "text": "Oi"},
		{"op": "set_whatsapp_link", "node": node, "rev": 1, "number_ref": "phone_1", "text": "Olá"},
	})
	edit := func() (*EditResult, error) {
		return h.svc.Edit(context.Background(), EditInput{WorkspaceID: wsA, ProjectID: v.Project.ID, ExpectedVersion: 1, Author: PersonAuthor(userA), Batch: batch})
	}
	h.executor.result = ExecuteResult{Failures: []site.Failure{{Index: 3, Code: site.CodeNodeStale, Field: "node"}}, Skipped: []site.Skip{{Index: 2, After: 1}}}
	var refusal *EditRefusal
	if _, err := edit(); !errors.As(err, &refusal) || refusal.Failures[0].Index != 1 || len(refusal.Skipped) != 0 {
		t.Fatalf("got %+v %v", refusal, err)
	}
	h.executor.result = ExecuteResult{}
	h.executor.steps = func(req ExecuteRequest) []site.StepPatch {
		steps := []site.StepPatch{}
		for i := range req.Batch.Operations {
			steps = append(steps, site.StepPatch{Op: req.Batch.Operations[i].Op, Index: i, Files: map[string]string{page: req.Project.Files[page].Content + strings.Repeat(" ", i+1)}})
		}
		return steps
	}
	res, err := edit()
	if err != nil {
		t.Fatal(err)
	}
	indexes := []int{}
	for _, s := range res.Steps {
		indexes = append(indexes, s.Index)
	}
	if jsonText(t, indexes) != "[0,1,1,1]" {
		t.Fatalf("got %v", indexes)
	}
	if got := h.numbers.asked; len(got) != 2 || got[1] != wsA+"/phone_1" {
		t.Fatalf("got %v", got)
	}
}

func TestWhatsAppNumbersMustBelongToTheWorkspace(t *testing.T) {
	h := newHarness(t)
	v := h.create(t, site.StackHTML, "blank", "Nova")
	page := "pages/index.html"
	node := page + "#" + offsetOf(v.Files[page].Content, "<h1")
	h.numbers.refuse = map[string]bool{"phone_x": true}
	batch, _ := json.Marshal([]map[string]any{
		{"op": "set_text", "node": node, "rev": 1, "text": "Oi"},
		{"op": "set_attr", "node": node, "rev": 1, "name": "data-vz-whatsapp", "value": "phone_x"},
	})
	var refusal *EditRefusal
	_, err := h.svc.Edit(context.Background(), EditInput{WorkspaceID: wsA, ProjectID: v.Project.ID, ExpectedVersion: 1, Author: PersonAuthor(userA), Batch: batch})
	if !errors.As(err, &refusal) || len(refusal.Failures) != 1 || refusal.Failures[0] != (site.Failure{Index: 1, Code: site.CodeWhatsAppNumberUnknown, Field: "value"}) {
		t.Fatalf("got %+v %v", refusal, err)
	}
	if len(h.executor.calls) != 0 {
		t.Fatal("a foreign number never reaches the builder")
	}
}

func TestEditAndSaveReturnTheFilesTheEditorReplaces(t *testing.T) {
	h := newHarness(t)
	v := h.create(t, site.StackHTML, "blank", "Nova")
	page := "pages/index.html"
	h.executor.steps = func(req ExecuteRequest) []site.StepPatch {
		return []site.StepPatch{{Op: site.OpSetText, Files: map[string]string{page: "<h1>Novo</h1>\n"}}}
	}
	res, err := h.svc.Edit(context.Background(), EditInput{WorkspaceID: wsA, ProjectID: v.Project.ID, ExpectedVersion: 1, Author: PersonAuthor(userA),
		Batch: setTextBatch(page+"#"+offsetOf(v.Files[page].Content, "<h1"), 1, "Novo")})
	if err != nil {
		t.Fatal(err)
	}
	if changes := res.View.Changes; len(changes.Files) != 1 || changes.Files[page] != (site.File{Content: "<h1>Novo</h1>\n", Rev: 2}) || len(changes.Deleted) != 0 || changes.Renamed == nil {
		t.Fatalf("got %s", jsonText(t, res.View.Changes))
	}
	saved, err := h.svc.SaveFiles(context.Background(), SaveFilesInput{WorkspaceID: wsA, ProjectID: v.Project.ID, ExpectedVersion: res.View.Project.Version, Author: PersonAuthor(userA),
		Files: map[string]string{"pages/sobre.html": "<p>a</p>\n", page: "<h1>Novo</h1>\n"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Changes.Files) != 1 || saved.Changes.Files["pages/sobre.html"].Rev != 1 || len(saved.Changes.Deleted) != 0 {
		t.Fatalf("got %+v", saved.Changes)
	}
}

func TestTheViewDerivesFactsTheEditorUsedToParse(t *testing.T) {
	h := newHarness(t)
	v := h.create(t, site.StackReact, "captura-de-leads", "Clínica")
	facts := v.Facts()
	if len(facts.Routes) != 3 || facts.Routes[0].Path != "/" || facts.RoutesIssue != "" {
		t.Fatalf("routes %+v", facts)
	}
	if len(facts.Tokens) == 0 || facts.Tokens[0].Name != "--color-brand" || facts.Tokens[0].Path != "src/index.css" {
		t.Fatalf("tokens %+v", facts.Tokens)
	}
	if len(facts.Knobs) != 2 || facts.Knobs[1].Number == nil || *facts.Knobs[1].Number != 1.25 || facts.KnobsIssue != "" {
		t.Fatalf("knobs %+v", facts.Knobs)
	}
	if jsonText(t, facts.Breakpoints) != `["sm","md","lg","xl"]` {
		t.Fatalf("breakpoints %v", facts.Breakpoints)
	}
	broken := *v
	broken.Files = map[string]site.File{site.ManifestPath: {Content: "{", Rev: 2}, site.KnobsPath: {Content: "[", Rev: 2}}
	facts = broken.Facts()
	if facts.RoutesIssue != site.CodeManifestInvalid || facts.KnobsIssue != site.CodeKnobsInvalid || facts.Routes == nil || facts.Knobs == nil {
		t.Fatalf("got %+v", facts)
	}
}

func TestPublishStateIsDerivedOnce(t *testing.T) {
	cases := []struct {
		status      Status
		unpublished bool
		want        PublishState
	}{
		{StatusDraft, true, PublishDraft},
		{StatusPublished, false, PublishLive},
		{StatusPublished, true, PublishLiveWithChanges},
		{StatusSuspended, true, PublishSuspended},
		{StatusArchived, true, PublishArchived},
	}
	for _, c := range cases {
		if got := (Project{Status: c.status, HasUnpublishedChanges: c.unpublished}).PublishState(); got != c.want {
			t.Fatalf("%s %v: got %s", c.status, c.unpublished, got)
		}
	}
}

func TestTheViewCarriesThePrimaryURL(t *testing.T) {
	h := newHarness(t)
	v := h.create(t, site.StackHTML, "blank", "Nova")
	h.hostnames.list = []sitepublish.HostnameRecord{
		{ProjectID: v.Project.ID, Hostname: "nova.paginas.test", Kind: sitepublish.HostnamePlatform, Status: sitepublish.HostnameActive},
		{ProjectID: v.Project.ID, Hostname: "www.nova.com.br", Kind: sitepublish.HostnameCustom, Status: sitepublish.HostnameActive},
	}
	got, err := h.svc.Get(context.Background(), wsA, v.Project.ID)
	if err != nil || got.PrimaryURL != "https://www.nova.com.br" {
		t.Fatalf("got %q %v", got.PrimaryURL, err)
	}
	h.hostnames.err = errors.New("db down")
	if _, err := h.svc.Get(context.Background(), wsA, v.Project.ID); err == nil {
		t.Fatal("an unreadable hostname list fails closed")
	}
}

func TestAuthorsSplitIntoKindAndID(t *testing.T) {
	if k, id := PersonAuthor(userA).Parts(); k != AuthorPerson || id != userA {
		t.Fatalf("got %s %s", k, id)
	}
	if k, id := EloAuthor(thread).Parts(); k != AuthorElo || id != thread {
		t.Fatalf("got %s %s", k, id)
	}
	if k, id := Author("x").Parts(); k != "" || id != "" {
		t.Fatalf("got %s %s", k, id)
	}
}

func TestViewersOnlyGetAPreviewAlreadyBuiltForTheWorkingVersion(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	v := h.create(t, site.StackHTML, "blank", "Nova")
	if _, err := h.svc.CachedPreview(ctx, wsA, v.Project.ID); !errors.Is(err, ErrPreviewNotBuilt) {
		t.Fatalf("got %v", err)
	}
	built, err := h.svc.Preview(ctx, wsA, v.Project.ID)
	if err != nil {
		t.Fatal(err)
	}
	cached, err := h.svc.CachedPreview(ctx, wsA, v.Project.ID)
	if err != nil || cached != built {
		t.Fatalf("got %+v %v", cached, err)
	}
	if _, err := saveFiles(h, v, PersonAuthor(userA), map[string]string{"pages/index.html": "<h1>Um</h1>\n"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.CachedPreview(ctx, wsA, v.Project.ID); !errors.Is(err, ErrPreviewNotBuilt) {
		t.Fatalf("a new working version needs a new build: %v", err)
	}
	h.cache.getErr = errors.New("redis down")
	if _, err := h.svc.CachedPreview(ctx, wsA, v.Project.ID); err == nil || errors.Is(err, ErrPreviewNotBuilt) {
		t.Fatalf("got %v", err)
	}
}

func jsonText(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
