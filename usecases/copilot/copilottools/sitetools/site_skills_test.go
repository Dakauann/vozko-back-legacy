package sitetools

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"vozko/domain/copilot"
	"vozko/usecases/copilot/copilottools/platformtools"
	"vozko/usecases/copilot/copilottools/skilltools"
	"vozko/usecases/copilot/skills"
)

var siteSkills = []string{
	"acessibilidade-web", "codigo-html-tailwind", "codigo-react-tailwind", "copy-de-conversao", "direcao-de-arte-web",
	"formularios-e-captura", "landing-page-de-conversao", "lgpd-em-paginas", "motion-web", "nicho-advocacia",
	"nicho-imobiliario", "nicho-medicina", "nicho-odontologia", "performance-web", "seo-e-geo",
}

var snakeWord = regexp.MustCompile(`\b[a-z]+(?:_[a-z]+)+\b`)

func registeredToolNames(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "testdata", "registry_tool_names.golden"))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			names[line] = true
		}
	}
	for _, tool := range SiteTools(Deps{}) {
		names[tool.Definition().Name] = true
	}
	return names
}

func offeredToolNames() map[string]bool {
	offered := map[string]bool{}
	for _, tool := range append(SiteTools(Deps{}), platformtools.NewAskUserTool()) {
		if siteView.Offers(tool) {
			offered[tool.Definition().Name] = true
		}
	}
	offered["load_skill"] = true
	return offered
}

func TestTheSiteSurfaceOffersExactlyTheSiteSkills(t *testing.T) {
	library, err := skills.Load()
	if err != nil {
		t.Fatal(err)
	}
	var offered []string
	for _, skill := range library.All() {
		if skill.OfferedOn(siteView) {
			offered = append(offered, skill.Name)
		}
	}
	if !slices.Equal(offered, siteSkills) {
		t.Fatalf("the site surface offers %v, want %v", offered, siteSkills)
	}
	for _, name := range siteSkills {
		skill, _ := library.Find(name)
		if strings.HasPrefix(name, "nicho-") {
			if !skill.OfferedOn(copilot.View{}) {
				t.Errorf("%s keeps serving the panel", name)
			}
			continue
		}
		if skill.Surface != copilot.SkillSiteWork || skill.OfferedOn(copilot.View{}) {
			t.Errorf("%s is site craft and stays in the site editor", name)
		}
	}
}

func TestSiteSkillsStayCompactAndNameOnlyToolsTheSiteOffers(t *testing.T) {
	library, err := skills.Load()
	if err != nil {
		t.Fatal(err)
	}
	registered := registeredToolNames(t)
	offered := offeredToolNames()
	for _, name := range siteSkills {
		skill, _ := library.Find(name)
		tokens := tokensOf(len(skill.Body) + len(skill.Description))
		t.Logf("%s: %d tokens", name, tokens)
		if tokens > siteSkillTokenBudget {
			t.Errorf("%s has %d tokens, over the %d of a core skill; split it", name, tokens, siteSkillTokenBudget)
		}
		for _, word := range snakeWord.FindAllString(skill.Body, -1) {
			if registered[word] && !offered[word] {
				t.Errorf("%s names %s, which the site surface does not offer", name, word)
			}
		}
	}
}

func TestEverySiteToolIsTaughtBySomeSiteSkillOrThePrompt(t *testing.T) {
	library, err := skills.Load()
	if err != nil {
		t.Fatal(err)
	}
	var corpus strings.Builder
	for _, name := range siteSkills {
		skill, _ := library.Find(name)
		corpus.WriteString(skill.Body)
	}
	for _, name := range []string{"site_look", "site_edit", "site_read", "site_settings", "site_generate_image", "site_capture_reference"} {
		if !strings.Contains(corpus.String(), name) {
			t.Errorf("no site skill teaches %s", name)
		}
	}
	index := skilltools.NewLoadSkillTool(library).(copilot.ViewDefined).DefinitionOn(siteView)
	for _, name := range siteSkills {
		if !strings.Contains(index.Description, "- "+name+": ") {
			t.Errorf("the site index lacks %s", name)
		}
	}
}
