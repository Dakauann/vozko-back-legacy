package skilltools

import (
	"slices"
	"strings"
	"testing"

	"vozko/domain/copilot"
	"vozko/usecases/copilot/skills"
)

var indexSurfaces = []struct {
	name string
	view copilot.View
}{
	{"panel", copilot.View{}},
	{"attendance", copilot.View{Surface: copilot.SurfaceAttendance}},
	{"leads", copilot.View{Surface: copilot.SurfaceLeads}},
	{"studio-image", copilot.View{Surface: copilot.SurfaceStudio, ProjectID: "p1", ProjectKind: copilot.StudioImage}},
	{"studio-video", copilot.View{Surface: copilot.SurfaceStudio, ProjectID: "p1", ProjectKind: copilot.StudioVideo}},
	{"email-design", copilot.View{Surface: copilot.SurfaceEmailDesign, ProjectID: "p1"}},
	{"email-campaign", copilot.View{Surface: copilot.SurfaceEmailCampaign, ProjectID: "p1"}},
	{"site", copilot.View{Surface: copilot.SurfaceSite, ProjectID: "p1"}},
}

func TestEverySurfaceIndexesOneLinePerOfferedSkill(t *testing.T) {
	library, err := skills.Load()
	if err != nil {
		t.Fatal(err)
	}
	tool := NewLoadSkillTool(library)
	for _, surface := range indexSurfaces {
		def := copilot.DefinitionOn(tool, surface.view)
		lines := strings.Split(def.Description, "\n")[1:]
		enum := def.Parameters["names"].Items.Enum
		if len(lines) != len(enum) || len(enum) == 0 {
			t.Fatalf("%s indexes %d lines for %d skills", surface.name, len(lines), len(enum))
		}
		for i, name := range enum {
			skill, _ := library.Find(name)
			if lines[i] != "- "+name+": "+skill.Description || !skill.OfferedOn(surface.view) {
				t.Errorf("%s line %d is %q", surface.name, i, lines[i])
			}
		}
		for _, skill := range library.All() {
			if skill.OfferedOn(surface.view) != slices.Contains(enum, skill.Name) {
				t.Errorf("%s indexes %s wrongly", surface.name, skill.Name)
			}
		}
	}
}

func TestTheEmailEditorIndexesNoAdSkills(t *testing.T) {
	library, err := skills.Load()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"design-de-email", "explicativo-e-dados", "lgpd-no-marketing", "nicho-advocacia", "nicho-imobiliario", "nicho-medicina", "nicho-odontologia", "oferta-e-preco", "pecas-por-segmento"}
	for _, view := range []copilot.View{{Surface: copilot.SurfaceEmailDesign, ProjectID: "p1"}, {Surface: copilot.SurfaceEmailCampaign, ProjectID: "p1"}} {
		got := copilot.DefinitionOn(NewLoadSkillTool(library), view).Parameters["names"].Items.Enum
		if !slices.Equal(got, want) {
			t.Errorf("the %s index offers %v, want %v", view.Surface, got, want)
		}
	}
}

func TestAdSkillsStayOnThePanelAndTheStudio(t *testing.T) {
	library, err := skills.Load()
	if err != nil {
		t.Fatal(err)
	}
	tool := NewLoadSkillTool(library)
	for _, view := range []copilot.View{{}, {Surface: copilot.SurfaceStudio, ProjectID: "p1", ProjectKind: copilot.StudioVideo}} {
		enum := copilot.DefinitionOn(tool, view).Parameters["names"].Items.Enum
		for _, name := range []string{"estrategia-de-campanha", "criativos-que-performam", "politicas-de-anuncios", "video-com-som", "formatos-e-zonas-seguras", "briefing-criativo", "funil-e-qualificacao"} {
			if !slices.Contains(enum, name) {
				t.Errorf("%s is missing on %q", name, view.Surface)
			}
		}
	}
}
