package sitebuild

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"vozko/domain/site"
)

type fakeSnippets struct {
	byWorkspace map[string][]site.DesignSnippet
	err         error
	calls       int
}

func (f *fakeSnippets) Snippets(_ context.Context, workspaceID string) ([]site.DesignSnippet, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.byWorkspace[workspaceID], nil
}

func workspaceSections(t *testing.T, snippets *fakeSnippets) *WorkspaceSections {
	t.Helper()
	return NewWorkspaceSections(library(t), snippets)
}

func savedSnippets() *fakeSnippets {
	return &fakeSnippets{byWorkspace: map[string][]site.DesignSnippet{
		"ws1": {
			{Key: "snippet-faixa", Name: "Faixa de oferta", Stack: site.StackHTML, Content: `<section class="faixa">Oferta</section>`},
			{Key: "snippet-card", Name: "Cartão", Stack: site.StackReact, Content: `<div className="card">Cartão</div>`},
			{Key: "hero", Name: "Meu topo", Stack: site.StackHTML, Content: `<section>Sombra</section>`},
			{Key: "snippet-faixa", Name: "Repetida", Stack: site.StackHTML, Content: `<section>Repetida</section>`},
		},
	}}
}

func TestWorkspaceSectionsListTheLibraryThenTheWorkspaceSnippets(t *testing.T) {
	sections := workspaceSections(t, savedSnippets())
	all, err := sections.List(context.Background(), "ws1", "")
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, info := range all {
		keys = append(keys, info.Key)
	}
	want := append(slices.Clone(librarySections), "snippet-faixa", "snippet-card")
	if !slices.Equal(keys, want) {
		t.Fatalf("keys = %v", keys)
	}
	faixa, card := all[len(all)-2], all[len(all)-1]
	if faixa.Kind != SectionSnippet || all[0].Kind != SectionFromLibrary {
		t.Fatalf("kinds %s %s", faixa.Kind, all[0].Kind)
	}
	if faixa.Title != (SectionTitle{PT: "Faixa de oferta", EN: "Faixa de oferta", ES: "Faixa de oferta", DE: "Faixa de oferta"}) {
		t.Fatalf("a snippet title is its name in every language: %+v", faixa.Title)
	}
	if !slices.Equal(faixa.Stacks, []site.Stack{site.StackHTML, site.StackReact}) || !slices.Equal(card.Stacks, []site.Stack{site.StackReact}) {
		t.Fatalf("an html snippet fits both stacks, a react one only react: %v %v", faixa.Stacks, card.Stacks)
	}
}

func TestWorkspaceSectionsFilterByStack(t *testing.T) {
	sections := workspaceSections(t, savedSnippets())
	html, err := sections.List(context.Background(), "ws1", site.StackHTML)
	if err != nil {
		t.Fatal(err)
	}
	for _, info := range html {
		if info.Key == "snippet-card" {
			t.Fatal("a react snippet is not offered to an html page")
		}
	}
	if len(html) != len(librarySections)+1 {
		t.Fatalf("got %d sections", len(html))
	}
	if _, err := sections.List(context.Background(), "ws1", site.Stack("vue")); !errors.Is(err, ErrSectionStackInvalid) {
		t.Fatalf("an unknown stack refuses: %v", err)
	}
}

func TestWorkspaceSectionsResolveTheSameWayTheyList(t *testing.T) {
	snippets := savedSnippets()
	sections := workspaceSections(t, snippets)
	ctx := context.Background()
	hero, ok, err := sections.Section(ctx, "ws1", site.StackHTML, "hero")
	if err != nil || !ok || hero.Stack != site.StackHTML || strings.Contains(hero.Source, "Sombra") {
		t.Fatalf("the library wins over a snippet with the same key: %+v %v %v", hero, ok, err)
	}
	if snippets.calls != 0 {
		t.Fatal("a library key never reads the snippets")
	}
	faixa, ok, err := sections.Section(ctx, "ws1", site.StackReact, "snippet-faixa")
	if err != nil || !ok || faixa.Stack != site.StackHTML || faixa.Source != `<section class="faixa">Oferta</section>` {
		t.Fatalf("the first snippet with a key wins: %+v %v %v", faixa, ok, err)
	}
	card, ok, err := sections.Section(ctx, "ws1", site.StackReact, "snippet-card")
	if err != nil || !ok || card.Stack != site.StackReact {
		t.Fatalf("a react snippet goes into a react page: %+v %v %v", card, ok, err)
	}
	if _, ok, err := sections.Section(ctx, "ws1", site.StackHTML, "snippet-card"); err != nil || ok {
		t.Fatalf("a react snippet never goes into an html page: %v %v", ok, err)
	}
	if _, ok, err := sections.Section(ctx, "ws2", site.StackHTML, "snippet-faixa"); err != nil || ok {
		t.Fatalf("another workspace's snippet does not resolve: %v %v", ok, err)
	}
	snippets.err = errBoom
	if _, _, err := sections.Section(ctx, "ws1", site.StackHTML, "snippet-faixa"); !errors.Is(err, ErrSectionsUnavailable) {
		t.Fatalf("an unreadable snippet store refuses: %v", err)
	}
	if _, err := sections.List(ctx, "ws1", ""); !errors.Is(err, ErrSectionsUnavailable) {
		t.Fatalf("an unreadable snippet store refuses the listing: %v", err)
	}
}

func TestNoLibraryKeyUsesTheSnippetPrefix(t *testing.T) {
	for _, key := range librarySections {
		if strings.HasPrefix(key, site.SnippetKeyPrefix) {
			t.Fatalf("%s would collide with saved sections", key)
		}
	}
}

type workspaceLibrary struct {
	sections map[string]Section
	err      error
}

func (w workspaceLibrary) Section(_ context.Context, workspaceID string, stack site.Stack, key string) (Section, bool, error) {
	if w.err != nil {
		return Section{}, false, w.err
	}
	s, ok := w.sections[workspaceID+"/"+string(stack)+"/"+key]
	return s, ok, nil
}

func TestExecuteSendsJSXSnippetsApartFromHTMLSections(t *testing.T) {
	f := newFixture(t)
	f.svc.sections = workspaceLibrary{sections: map[string]Section{
		"ws1/react/faq":          {Stack: site.StackHTML, Source: "<section>FAQ</section>"},
		"ws1/react/snippet-card": {Stack: site.StackReact, Source: `<div className="card" />`},
	}}
	f.builder.execute = ExecuteResult{Steps: []site.StepPatch{{Op: site.OpInsertSection, Index: 0}}}
	project := site.NewProject(site.StackReact, map[string]string{"site.json": `{"name":"C","routes":[{"path":"/","page":"src/routes/index.jsx"}]}`, "src/routes/index.jsx": "export default function Page() { return <main />; }\n"})
	batch, err := site.PrepareBatch(project, []byte(`[
		{"op":"insert_section","at":{"page":"src/routes/index.jsx"},"rev":1,"library_key":"faq"},
		{"op":"insert_section","at":{"page":"src/routes/index.jsx"},"rev":1,"library_key":"snippet-card"}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Execute(context.Background(), ExecuteInput{WorkspaceID: "ws1", ProjectID: "p1", Project: project, Batch: batch}); err != nil {
		t.Fatal(err)
	}
	req := f.builder.executes[0]
	if req.Sections["faq"] != "<section>FAQ</section>" || len(req.Sections) != 1 || req.JSXSections["snippet-card"] != `<div className="card" />` || len(req.JSXSections) != 1 {
		t.Fatalf("sections %v jsx %v", req.Sections, req.JSXSections)
	}
	f.svc.sections = workspaceLibrary{err: ErrSectionsUnavailable}
	if _, err := f.svc.Execute(context.Background(), ExecuteInput{WorkspaceID: "ws1", ProjectID: "p1", Project: project, Batch: batch}); !errors.Is(err, ErrSectionsUnavailable) {
		t.Fatalf("an unreadable section library refuses the batch: %v", err)
	}
}

func TestExtractAsksTheBuilderForTheNodeSource(t *testing.T) {
	f := newFixture(t)
	f.builder.extract = ExtractResult{Source: "<h1>Oi</h1>"}
	out, err := f.svc.Extract(context.Background(), ExtractInput{WorkspaceID: "ws1", ProjectID: "p1", Project: htmlProject(), Node: "pages/index.html#0", Rev: 1, Bases: map[string][]site.File{"pages/index.html": {{Content: "<h1>Antes</h1>", Rev: 1}}}})
	if err != nil || out.Source != "<h1>Oi</h1>" {
		t.Fatalf("got %+v %v", out, err)
	}
	if req := f.builder.extracts[0]; req.Node != "pages/index.html#0" || req.Rev != 1 || req.Project.Stack != site.StackHTML || req.Bases["pages/index.html"][0].Content != "<h1>Antes</h1>" {
		t.Fatalf("request %+v", req)
	}
}

func TestCandidateFillsMissingSocialImagesAndKeepsTheChosenOnes(t *testing.T) {
	f := newFixture(t)
	settings := site.Settings{Pages: map[string]site.PageMeta{"/obrigado": {OGMediaID: "og1"}}}
	_, err := f.svc.Candidate(context.Background(), CandidateInput{
		WorkspaceID: "ws1", ProjectID: "p1", Project: htmlProject(), Settings: settings, PrimaryOrigin: "https://sorriso.paginas.test",
		OGImages: map[string]string{"/": "https://sorriso.paginas.test/_m/abc.jpg", "/obrigado": "https://sorriso.paginas.test/_m/def.jpg"},
	})
	if err != nil {
		t.Fatal(err)
	}
	pages := f.builder.builds[0].Shell.Pages
	if pages["/"].OGImage != "https://sorriso.paginas.test/_m/abc.jpg" || pages["/obrigado"].OGImage != "https://cdn.vozko.test/og1.webp" {
		t.Fatalf("pages %+v", pages)
	}
	if _, err := f.svc.Candidate(context.Background(), CandidateInput{
		WorkspaceID: "ws1", ProjectID: "p1", Project: htmlProject(), OGImages: map[string]string{"/": "javascript:alert(1)"},
	}); !errors.Is(err, ErrBuilderRefused) {
		t.Fatalf("a social image must be an https url: %v", err)
	}
}
