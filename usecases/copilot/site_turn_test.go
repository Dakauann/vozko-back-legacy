package copilot_usecase

import (
	"strings"
	"testing"
	"time"

	"vozko/domain/copilot"
)

var siteView = copilot.View{Surface: copilot.SurfaceSite, ProjectID: "3b9a7c2d-5e6f-4a1b-8c9d-0e1f2a3b4c5d"}

type siteTool struct{ fakeTool }

func (siteTool) OfferedOn(view copilot.View) bool { return view.OnSite() }

func TestSiteToolsExistOnlyInsideTheSiteEditor(t *testing.T) {
	read := &siteTool{fakeTool{name: "site_read", meta: readMeta}}
	plain := &fakeTool{name: "list_agents", meta: readMeta}
	if got := offeredNames(studioDriver(siteView, read, plain)); got != "site_read" {
		t.Fatalf("the site editor offers only its own tools, got %s", got)
	}
	for _, other := range []copilot.View{{}, studioView, emailView} {
		if strings.Contains(offeredNames(studioDriver(other, read, plain)), "site_read") {
			t.Fatalf("%q never offers site tools", other.Surface)
		}
	}
}

func TestTheSitePromptExplainsTheEditorMechanicsOnlyThere(t *testing.T) {
	prompt := systemPrompt(siteView, time.Now())
	for _, want := range []string{"# Editor de páginas", siteView.ProjectID, "site_read", "site_edit", "edit_file", "node_stale", "@ref", "open_gaps", "site_look", "site_generate_image", "site_publish", "load_skill"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the site prompt lacks %q", want)
		}
	}
	for _, craft := range []string{"hero", "headline", "paleta", "tipografia"} {
		if strings.Contains(strings.ToLower(prompt), craft) {
			t.Errorf("craft word %q belongs in a skill, not in the prompt", craft)
		}
	}
	if strings.Contains(prompt, "# Anúncios da Meta") || strings.Contains(prompt, "# Editor de e-mail") || strings.Contains(prompt, "# Estúdio") {
		t.Fatal("a focused surface leaves out the guides of other areas")
	}
	for _, other := range []copilot.View{{}, studioView, emailView} {
		if strings.Contains(systemPrompt(other, time.Now()), "# Editor de páginas") {
			t.Fatal("the site editor section stays inside the site editor")
		}
	}
}

func TestASiteTurnTalksToTheOpenPage(t *testing.T) {
	svc := &Service{screens: newMailbox(), newID: func() string { return "cmd-1" }, turns: NewTurnRegistry(copilot.DefaultTurnLimits())}
	cc := svc.attachScreen(studioCtx(siteView), "t1", func(string, interface{}) {})
	screen, ok := cc.Screen.(*screenSession)
	if !ok || screen.projectID != siteView.ProjectID {
		t.Fatalf("the screen session must target the open page, got %+v", cc.Screen)
	}
}

func TestTheSiteOpeningNotesNameThePageAndSiteRead(t *testing.T) {
	for name, note := range map[string]string{"opened": projectOpenedNote(siteView), "back": editorBackNote(siteView)} {
		if !strings.Contains(note, siteView.ProjectID) || !strings.Contains(note, "site_read") || strings.Contains(note, "studio_read") || strings.Contains(note, "email_read") {
			t.Errorf("%s: got %q", name, note)
		}
	}
}
