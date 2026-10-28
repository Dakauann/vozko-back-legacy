package copilot

import (
	"errors"
	"testing"
)

const openSiteID = "3b9a7c2d-5e6f-4a1b-8c9d-0e1f2a3b4c5d"

func TestTheSiteViewNamesTheOpenPage(t *testing.T) {
	view := View{Surface: SurfaceSite, ProjectID: openSiteID}
	if err := view.Validate(); err != nil {
		t.Fatal(err)
	}
	if !view.OnSite() || view.OnStudio() || view.OnEmailDocument() || !view.OnEditor() || !view.Focused() {
		t.Fatalf("got %+v", view)
	}
	for name, bad := range map[string]View{
		"no page":        {Surface: SurfaceSite},
		"invented id":    {Surface: SurfaceSite, ProjectID: "p1"},
		"a studio kind":  {Surface: SurfaceSite, ProjectID: openSiteID, ProjectKind: StudioImage},
		"a period":       {Surface: SurfaceSite, ProjectID: openSiteID, DateFrom: "2026-01-01"},
		"selected leads": {Surface: SurfaceSite, ProjectID: openSiteID, SelectedLeads: 2},
	} {
		if err := bad.Validate(); !errors.Is(err, ErrInvalidView) {
			t.Errorf("%s: want ErrInvalidView, got %v", name, err)
		}
	}
}

func TestOnlySiteSkillsServeTheSiteEditor(t *testing.T) {
	page := View{Surface: SurfaceSite, ProjectID: openSiteID}
	for name, skill := range map[string]Skill{
		"everywhere":  {},
		"panel only":  {Surface: SkillPanelOnly},
		"studio only": {Surface: SkillStudioOnly},
		"email work":  {Surface: SkillEmailWork},
	} {
		if skill.OfferedOn(page) {
			t.Errorf("%s skills stay out of the site editor unless they name it", name)
		}
	}
	for name, skill := range map[string]Skill{
		"site work":         {Surface: SkillSiteWork},
		"studio, also site": {Surface: SkillStudioOnly, Also: []SkillSurface{SkillSiteWork}},
		"everywhere + site": {Also: []SkillSurface{SkillSiteWork}},
	} {
		if !skill.OfferedOn(page) {
			t.Errorf("%s must be offered in the site editor", name)
		}
	}
}

func TestSiteSkillsStayInTheSiteEditor(t *testing.T) {
	skill := Skill{Surface: SkillSiteWork}
	for _, view := range []View{{}, {Surface: SurfaceStudio, ProjectID: openSiteID, ProjectKind: StudioImage}, {Surface: SurfaceEmailDesign, ProjectID: openSiteID}} {
		if skill.OfferedOn(view) {
			t.Fatalf("site craft is not offered on %q", view.Surface)
		}
	}
	widened := Skill{Surface: SkillStudioOnly, Also: []SkillSurface{SkillSiteWork}}
	if !widened.OfferedOn(View{Surface: SurfaceStudio, ProjectID: openSiteID, ProjectKind: StudioImage}) || widened.OfferedOn(View{}) {
		t.Fatal("widening a skill to the site keeps its home surface as it was")
	}
	everywhere := Skill{Also: []SkillSurface{SkillSiteWork}}
	if !everywhere.OfferedOn(View{}) || !everywhere.OfferedOn(View{Surface: SurfaceEmailDesign, ProjectID: openSiteID}) {
		t.Fatal("an everywhere skill widened to the site stays everywhere")
	}
}

func TestSkillSurfacesAreKnownAndNamedOnce(t *testing.T) {
	ok := []Skill{
		{Name: "x", Description: "d", Body: "b", Surface: SkillSiteWork},
		{Name: "x", Description: "d", Body: "b", Surface: SkillStudioOnly, Also: []SkillSurface{SkillSiteWork}},
		{Name: "x", Description: "d", Body: "b", Also: []SkillSurface{SkillSiteWork}},
	}
	for _, skill := range ok {
		if err := skill.Validate(); err != nil {
			t.Errorf("%+v: %v", skill, err)
		}
	}
	bad := map[string]Skill{
		"unknown extra":   {Name: "x", Description: "d", Body: "b", Also: []SkillSurface{"moon"}},
		"empty extra":     {Name: "x", Description: "d", Body: "b", Also: []SkillSurface{""}},
		"home repeated":   {Name: "x", Description: "d", Body: "b", Surface: SkillSiteWork, Also: []SkillSurface{SkillSiteWork}},
		"extra repeated":  {Name: "x", Description: "d", Body: "b", Surface: SkillStudioOnly, Also: []SkillSurface{SkillSiteWork, SkillSiteWork}},
		"unknown surface": {Name: "x", Description: "d", Body: "b", Surface: "moon"},
	}
	for name, skill := range bad {
		if err := skill.Validate(); !errors.Is(err, ErrSkillSurface) {
			t.Errorf("%s: want ErrSkillSurface, got %v", name, err)
		}
	}
}

func TestTheAppReportsWhetherTheNewPageOpened(t *testing.T) {
	opened, err := SiteOpeningOf(openSiteID, []byte(`{"projectId":"`+openSiteID+`","kind":"site","opened":true}`))
	if err != nil {
		t.Fatal(err)
	}
	want := View{Surface: SurfaceSite, ProjectID: openSiteID}
	if opened.View != want || !opened.Opened || opened.Awaiting() != nil {
		t.Fatalf("got %+v", opened)
	}
	for name, raw := range map[string]string{
		"another page":  `{"projectId":"5f0c7c1e-1d2a-4b8e-9d11-3a2b1c0d9e8f","kind":"site","opened":true}`,
		"an email kind": `{"projectId":"` + openSiteID + `","kind":"email","opened":true}`,
		"unknown field": `{"projectId":"` + openSiteID + `","kind":"site","opened":true,"path":"/x"}`,
		"not json":      `aberto`,
	} {
		if _, err := SiteOpeningOf(openSiteID, []byte(raw)); !errors.Is(err, ErrInvalidCreatedProject) {
			t.Errorf("%s: want ErrInvalidCreatedProject, got %v", name, err)
		}
	}
	waiting := WaitingSite(openSiteID)
	if waiting.Opened || waiting.Awaiting() == nil || *waiting.Awaiting() != want {
		t.Fatalf("got %+v", waiting)
	}
}

func TestOpeningAPageIsAClaimedAppCommand(t *testing.T) {
	if !ScreenOpenSite.Known() || ScreenOpenSite.NeedsProject() || !ScreenOpenSite.Commits() {
		t.Fatal("opening a page runs once, on one tab, before any editor is attached")
	}
}
