package skills

import (
	"fmt"
	"strings"
	"testing"

	"vozko/domain/site"
	"vozko/domain/siteform"
)

func TestTheSiteFormSkillTeachesTheWholeFormBuilder(t *testing.T) {
	lib, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	skill, ok := lib.Find("formularios-e-captura")
	if !ok {
		t.Fatal("formularios-e-captura must ship")
	}
	for _, target := range siteform.LeadTargets() {
		if !strings.Contains(skill.Body, target) {
			t.Errorf("the skill must name the lead target %s", target)
		}
	}
	for _, step := range []string{"set_attr", "required", "set_text", "insert_section", "move", "remove", "lead_fields", "\"name\"", fmt.Sprint(site.MaxFormNameRunes)} {
		if !strings.Contains(skill.Body, step) {
			t.Errorf("the skill must cover %q", step)
		}
	}
}
