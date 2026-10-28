package skills

import (
	"errors"
	"slices"
	"testing"
	"testing/fstest"

	"vozko/domain/copilot"
)

func TestASkillCanAlsoServeTheSiteEditor(t *testing.T) {
	files := fstest.MapFS{"library/marca/SKILL.md": {Data: []byte("---\nname: marca\ndescription: d\nsurface: studio\nalso: site\n---\n\nx\n")}}
	lib, err := load(files)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := lib.Find("marca")
	if got.Surface != copilot.SkillStudioOnly || !slices.Equal(got.Also, []copilot.SkillSurface{copilot.SkillSiteWork}) {
		t.Fatalf("got %+v", got)
	}
	for name, header := range map[string]string{
		"unknown":  "also: moon",
		"repeated": "also: site, site",
		"blank":    "also: site, ",
	} {
		raw := "---\nname: a\ndescription: d\n" + header + "\n---\nx"
		if _, err := load(fstest.MapFS{"library/a/SKILL.md": {Data: []byte(raw)}}); !errors.Is(err, copilot.ErrSkillSurface) {
			t.Errorf("%s: want ErrSkillSurface, got %v", name, err)
		}
	}
}
