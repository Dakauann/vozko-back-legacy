package skills

import (
	"strings"
	"testing"
)

func TestTheSiteCodeSkillsTeachDesktopFirstClasses(t *testing.T) {
	lib, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"codigo-html-tailwind", "codigo-react-tailwind"} {
		skill, ok := lib.Find(name)
		if !ok {
			t.Fatalf("%s must ship", name)
		}
		for _, rule := range []string{"computador primeiro", "max-lg:", "max-md:", "set_classes", "breakpoint"} {
			if !strings.Contains(skill.Body, rule) {
				t.Errorf("%s must teach %q", name, rule)
			}
		}
	}
}
