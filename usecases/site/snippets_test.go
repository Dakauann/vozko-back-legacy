package site_usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"vozko/domain/shared"
	"vozko/domain/site"
)

func (h *harness) snippet(t *testing.T, projectID, name string) *SavedSnippet {
	t.Helper()
	saved, err := h.svc.SaveSnippet(context.Background(), SaveSnippetInput{WorkspaceID: wsA, ProjectID: projectID, Node: "pages/index.html#0", Rev: 1, Name: name})
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

func TestSaveSnippetCreatesTheDefaultDesignSystemWhenTheWorkspaceHasNone(t *testing.T) {
	h := newHarness(t)
	v := h.create(t, site.StackHTML, "blank", "Clínica")
	h.executor.extract = ExtractResult{Source: `<section class="cta">Agende</section>`}
	saved := h.snippet(t, v.Project.ID, "  Chamada final  ")
	if saved.Snippet != (site.DesignSnippet{Key: "snippet-chamada-final", Name: "Chamada final", Stack: site.StackHTML, Content: `<section class="cta">Agende</section>`}) {
		t.Fatalf("snippet %+v", saved.Snippet)
	}
	if !saved.DesignSystem.IsDefault || saved.DesignSystem.System.Name != DefaultDesignSystemName || len(saved.DesignSystem.System.Snippets) != 1 {
		t.Fatalf("design system %+v", saved.DesignSystem)
	}
	call := h.executor.extracts[0]
	if call.WorkspaceID != wsA || call.ProjectID != v.Project.ID || call.Node != "pages/index.html#0" || call.Rev != 1 || call.Project.Stack != site.StackHTML || len(call.Project.Files) == 0 {
		t.Fatalf("extract %+v", call)
	}
	again := h.snippet(t, v.Project.ID, "Chamada final")
	if again.Snippet.Key != "snippet-chamada-final-2" || again.DesignSystem.ID != saved.DesignSystem.ID || len(again.DesignSystem.System.Snippets) != 2 {
		t.Fatalf("a repeated name gets a free key on the same design system: %+v", again)
	}
}

func TestSaveSnippetUsesTheProjectDesignSystemThenTheWorkspaceDefault(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	fallback, err := h.svc.CreateDesignSystem(ctx, wsA, site.DesignSystem{Name: "Marca"}, true)
	if err != nil {
		t.Fatal(err)
	}
	own, err := h.svc.CreateDesignSystem(ctx, wsA, site.DesignSystem{Name: "Campanha"}, false)
	if err != nil {
		t.Fatal(err)
	}
	h.executor.extract = ExtractResult{Source: `<div className="card" />`}
	plain := h.create(t, site.StackReact, "blank", "Sem sistema")
	if _, err := h.svc.Update(ctx, wsA, plain.Project.ID, plain.Project.Version, Change{DesignSystemID: new(string)}); err != nil {
		t.Fatal(err)
	}
	if saved := h.snippet(t, plain.Project.ID, "Cartão"); saved.DesignSystem.ID != fallback.ID || saved.Snippet.Stack != site.StackReact || saved.Snippet.Key != "snippet-cartao" {
		t.Fatalf("a page without a design system saves on the workspace default: %+v", saved)
	}
	branded, err := h.svc.Create(ctx, CreateInput{WorkspaceID: wsA, UserID: userA, Name: "Com sistema", Stack: site.StackReact, Starter: "blank", DesignSystemID: own.ID})
	if err != nil {
		t.Fatal(err)
	}
	if saved := h.snippet(t, branded.Project.ID, "Cartão"); saved.DesignSystem.ID != own.ID || saved.Snippet.Key != "snippet-cartao-2" {
		t.Fatalf("a page saves on its own design system, keys stay unique in the workspace: %+v", saved)
	}
}

func TestSaveSnippetRefusesWhatCannotBecomeASection(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	v := h.create(t, site.StackHTML, "blank", "Clínica")
	if _, err := h.svc.SaveSnippet(ctx, SaveSnippetInput{WorkspaceID: wsA, ProjectID: v.Project.ID, Node: "pages/index.html#0", Rev: 1, Name: " "}); !errors.Is(err, ErrNameInvalid) {
		t.Fatalf("a blank name: %v", err)
	}
	if _, err := h.svc.SaveSnippet(ctx, SaveSnippetInput{WorkspaceID: wsA, ProjectID: v.Project.ID, Node: "pages/index.html#0", Rev: 1, Name: strings.Repeat("a", 121)}); !errors.Is(err, ErrNameInvalid) {
		t.Fatalf("a long name: %v", err)
	}
	var refusal *EditRefusal
	if _, err := h.svc.SaveSnippet(ctx, SaveSnippetInput{WorkspaceID: wsA, ProjectID: v.Project.ID, Rev: 1, Name: "Faixa"}); !errors.As(err, &refusal) || refusal.Failures[0].Code != site.CodeFieldRequired || refusal.Failures[0].Field != "node" {
		t.Fatalf("a missing node: %v", err)
	}
	h.executor.extract = ExtractResult{Failure: &site.Failure{Code: site.CodeNodeStale, Field: "node"}}
	if _, err := h.svc.SaveSnippet(ctx, SaveSnippetInput{WorkspaceID: wsA, ProjectID: v.Project.ID, Node: "pages/index.html#9", Rev: 1, Name: "Faixa"}); !errors.As(err, &refusal) || refusal.Failures[0].Code != site.CodeNodeStale {
		t.Fatalf("a stale node: %v", err)
	}
	h.executor.extract = ExtractResult{}
	h.executor.err = errors.New("builder down")
	if _, err := h.svc.SaveSnippet(ctx, SaveSnippetInput{WorkspaceID: wsA, ProjectID: v.Project.ID, Node: "pages/index.html#0", Rev: 1, Name: "Faixa"}); !errors.Is(err, ErrBuilderUnavailable) {
		t.Fatalf("an unreachable builder: %v", err)
	}
	h.executor.err = nil
	h.executor.extract = ExtractResult{Source: strings.Repeat("x", site.MaxProjectBytes+1)}
	if _, err := h.svc.SaveSnippet(ctx, SaveSnippetInput{WorkspaceID: wsA, ProjectID: v.Project.ID, Node: "pages/index.html#0", Rev: 1, Name: "Faixa"}); !errors.Is(err, ErrSnippetsFull) {
		t.Fatalf("a library past the size of a project: %v", err)
	}
	if _, err := h.svc.SaveSnippet(ctx, SaveSnippetInput{WorkspaceID: wsB, ProjectID: v.Project.ID, Node: "pages/index.html#0", Rev: 1, Name: "Faixa"}); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("another workspace's page: %v", err)
	}
}

func TestSaveSnippetRetriesWhenTheDesignSystemChangedMeanwhile(t *testing.T) {
	h := newHarness(t)
	v := h.create(t, site.StackHTML, "blank", "Clínica")
	h.executor.extract = ExtractResult{Source: `<section>Oi</section>`}
	h.snippet(t, v.Project.ID, "Primeira")
	h.store.conflictOnce = true
	saved := h.snippet(t, v.Project.ID, "Segunda")
	if len(saved.DesignSystem.System.Snippets) != 2 {
		t.Fatalf("got %+v", saved.DesignSystem.System.Snippets)
	}
	h.store.conflictAlways = true
	if _, err := h.svc.SaveSnippet(context.Background(), SaveSnippetInput{WorkspaceID: wsA, ProjectID: v.Project.ID, Node: "pages/index.html#0", Rev: 1, Name: "Terceira"}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a design system that keeps changing gives up: %v", err)
	}
}

func TestSnippetsListTheDefaultDesignSystemFirst(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if _, err := h.svc.CreateDesignSystem(ctx, wsA, site.DesignSystem{Name: "Outra", Snippets: []site.DesignSnippet{{Key: "snippet-b", Name: "B", Stack: site.StackHTML, Content: "<p>B</p>"}}}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.CreateDesignSystem(ctx, wsA, site.DesignSystem{Name: "Marca", Snippets: []site.DesignSnippet{{Key: "snippet-a", Name: "A", Stack: site.StackReact, Content: "<p>A</p>"}}}, true); err != nil {
		t.Fatal(err)
	}
	snippets, err := h.svc.Snippets(ctx, wsA)
	if err != nil {
		t.Fatal(err)
	}
	if len(snippets) != 2 || snippets[0].Key != "snippet-a" || snippets[1].Key != "snippet-b" {
		t.Fatalf("got %+v", snippets)
	}
	if other, err := h.svc.Snippets(ctx, wsB); err != nil || len(other) != 0 {
		t.Fatalf("another workspace sees nothing: %+v %v", other, err)
	}
}

func TestASnippetPickedAtAnOlderRevReachesTheBuilderWithThatRevision(t *testing.T) {
	h := newHarness(t)
	v := h.create(t, site.StackHTML, "blank", "Clínica")
	page := "pages/index.html"
	original := v.Files[page].Content
	if _, err := saveFiles(h, v, PersonAuthor(userA), map[string]string{page: "<p>topo</p>\n" + original}); err != nil {
		t.Fatal(err)
	}
	h.executor.extract = ExtractResult{Source: `<section>Agende</section>`}
	h.snippet(t, v.Project.ID, "Chamada")
	bases := h.executor.extracts[0].Bases[page]
	if len(bases) != 1 || bases[0] != (site.File{Content: original, Rev: 1}) {
		t.Fatalf("got %+v", h.executor.extracts[0].Bases)
	}
	h.executor.extracts = nil
	saved, err := h.svc.SaveSnippet(context.Background(), SaveSnippetInput{WorkspaceID: wsA, ProjectID: v.Project.ID, Node: page + "#0", Rev: 2, Name: "Atual"})
	if err != nil || saved == nil || len(h.executor.extracts[0].Bases) != 0 {
		t.Fatalf("a current rev needs no older revision: %+v %v", h.executor.extracts, err)
	}
}
