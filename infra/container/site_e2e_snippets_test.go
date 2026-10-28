package container

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"vozko/domain/site"
	site_usecase "vozko/usecases/site"
)

func TestASavedSectionComesBackThroughTheSameSectionLibrary(t *testing.T) {
	w := newSiteWorld(t)
	ctx := context.Background()
	created, err := w.storage.Create(ctx, site_usecase.CreateInput{WorkspaceID: e2eWorkspace, UserID: e2eUser, Name: "Clínica Sorriso", Stack: site.StackReact, Starter: "captura-de-leads"})
	if err != nil {
		t.Fatal(err)
	}
	page := "src/routes/index.jsx"
	file := created.Files[page]
	offset := strings.Index(file.Content, "<section")
	saved, err := w.storage.SaveSnippet(ctx, site_usecase.SaveSnippetInput{
		WorkspaceID: e2eWorkspace, ProjectID: created.Project.ID, Node: page + "#" + strconv.Itoa(offset), Rev: file.Rev, Name: "Topo da clínica",
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Snippet.Key != "snippet-topo-da-clinica" || saved.Snippet.Stack != site.StackReact || !strings.HasPrefix(saved.Snippet.Content, "<section") || !saved.DesignSystem.IsDefault {
		t.Fatalf("saved %+v", saved)
	}
	listed, err := w.sections.List(ctx, e2eWorkspace, site.StackReact)
	if err != nil || listed[len(listed)-1].Key != saved.Snippet.Key {
		t.Fatalf("the saved section is listed for react pages: %v", err)
	}
	if html, err := w.sections.List(ctx, e2eWorkspace, site.StackHTML); err != nil || html[len(html)-1].Key == saved.Snippet.Key {
		t.Fatalf("a react section is not offered to html pages: %v", err)
	}
	batch := []byte(`[{"op":"insert_section","at":{"page":"` + page + `"},"rev":` + strconv.FormatUint(file.Rev, 10) + `,"library_key":"` + saved.Snippet.Key + `"}]`)
	_, _ = w.storage.Edit(ctx, site_usecase.EditInput{
		WorkspaceID: e2eWorkspace, ProjectID: created.Project.ID, ExpectedVersion: created.Project.Version,
		Author: site_usecase.PersonAuthor(e2eUser), Label: "Seção salva", Batch: batch,
	})
	sent := w.builder.executes[len(w.builder.executes)-1]
	if sent.JSXSections[saved.Snippet.Key] != saved.Snippet.Content || len(sent.Sections) != 0 {
		t.Fatalf("the builder gets the saved jsx verbatim: %+v %+v", sent.JSXSections, sent.Sections)
	}
}
