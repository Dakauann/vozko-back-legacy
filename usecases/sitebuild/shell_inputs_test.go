package sitebuild

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"vozko/domain/site"
	"vozko/domain/sitesubmission"
)

func formProject() site.Project {
	p := htmlProject()
	p.Files["pages/index.html"] = site.File{Content: `<form data-vz-form="contato"><input name="lead.phone"></form>` + "\n", Rev: 1}
	return p
}

func TestPreviewPointsEveryFormAtThePreviewSink(t *testing.T) {
	f := newFixture(t)
	in := previewInput()
	in.Project = formProject()
	in.Settings.Forms = map[string]site.FormSettings{"orcamento": {}}
	if _, err := f.svc.Preview(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	shell := f.builder.builds[0].Shell
	want := map[string]string{"contato": sitesubmission.PreviewFormKey, "orcamento": sitesubmission.PreviewFormKey}
	if len(shell.Forms) != len(want) || shell.Forms["contato"] != want["contato"] || shell.Forms["orcamento"] != want["orcamento"] {
		t.Fatalf("got %v", shell.Forms)
	}
	if shell.API != "https://api.vozko.test" {
		t.Fatalf("the preview posts to the api origin: %q", shell.API)
	}
}

func TestShellCarriesTheFaviconAndOrganizationData(t *testing.T) {
	f := newFixture(t)
	in := previewInput()
	in.Settings.Site = site.Identity{FaviconMediaID: "m1", StructuredData: true}
	_, err := f.svc.Candidate(context.Background(), CandidateInput{
		WorkspaceID: in.WorkspaceID, ProjectID: in.ProjectID, Project: in.Project, Settings: in.Settings, PrimaryOrigin: "https://sorriso.paginas.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	shell := f.builder.builds[0].Shell
	if shell.Favicon != "https://cdn.vozko.test/m1.webp" || shell.PrimaryOrigin != "https://sorriso.paginas.test" {
		t.Fatalf("got %+v", shell)
	}
	if len(shell.StructuredData) != 1 {
		t.Fatalf("got %d structured data blocks", len(shell.StructuredData))
	}
	var org map[string]string
	if err := json.Unmarshal(shell.StructuredData[0], &org); err != nil {
		t.Fatal(err)
	}
	if org["@type"] != "Organization" || org["name"] != "Clínica Sorriso" || org["url"] != "https://sorriso.paginas.test" || org["logo"] != "https://cdn.vozko.test/m1.webp" {
		t.Fatalf("got %v", org)
	}
}

func TestShellLeavesOutWhatIsNotEnabledAndRefusesAMissingFavicon(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.Preview(context.Background(), previewInput()); err != nil {
		t.Fatal(err)
	}
	if shell := f.builder.builds[0].Shell; shell.Favicon != "" || len(shell.StructuredData) != 0 {
		t.Fatalf("got %+v", shell)
	}
	in := previewInput()
	in.Settings.Site = site.Identity{FaviconMediaID: "gone"}
	if _, err := f.svc.Preview(context.Background(), in); !errors.Is(err, ErrMediaUnavailable) {
		t.Fatalf("a favicon that cannot be located refuses: %v", err)
	}
}

func TestPreviewLinksWhatsAppAnchorsStraightToTheNumber(t *testing.T) {
	f := newFixture(t)
	result := okResult()
	result.Assets[0].Body = []byte(`<!DOCTYPE html><html><head></head><body><a href="#" data-vz-whatsapp="phone-1" data-vz-text="Oi">Fale</a><a href="#" data-vz-whatsapp="phone-x">Outro</a><vz-whatsapp channel="phone-1">Chat</vz-whatsapp></body></html>`)
	f.builder.result = result
	out, err := f.svc.Preview(context.Background(), previewInput())
	if err != nil {
		t.Fatal(err)
	}
	host, _ := hostOf(t, out.URL)
	served, err := f.svc.Serve(context.Background(), host, "/")
	if err != nil {
		t.Fatal(err)
	}
	body := string(served.Body)
	for _, want := range []string{`href="https://wa.me/5511999990000?text=Oi" data-vz-whatsapp="phone-1"`, `<a href="#" data-vz-whatsapp="phone-x">`, `<vz-whatsapp href="https://wa.me/5511999990000?text=" channel="phone-1">`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %s in %s", want, body)
		}
	}
	if len(f.whatsapp.calls) == 0 || !strings.HasPrefix(f.whatsapp.calls[0], "ws1|") {
		t.Fatalf("the link is resolved for the page's workspace: %v", f.whatsapp.calls)
	}
}

func TestCandidateLeavesWhatsAppAnchorsForThePublish(t *testing.T) {
	f := newFixture(t)
	result := okResult()
	result.Assets[0].Body = []byte(`<!DOCTYPE html><html><head></head><body><a href="#" data-vz-whatsapp="phone-1">Fale</a></body></html>`)
	f.builder.result = result
	out, err := f.svc.Candidate(context.Background(), CandidateInput{WorkspaceID: "ws1", ProjectID: "p1", Project: htmlProject()})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out.Result.Assets[0].Body), `<a href="#" data-vz-whatsapp="phone-1">`) || len(f.whatsapp.calls) != 0 {
		t.Fatalf("publish links carry a page reference and are made at publish: %s", out.Result.Assets[0].Body)
	}
}
