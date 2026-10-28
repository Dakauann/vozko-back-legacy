package site_usecase

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"

	"vozko/domain/site"
	"vozko/domain/sitehtml"
)

var requiredStarters = map[site.Stack][]string{
	site.StackReact: {"blank", "captura-de-leads"},
	site.StackHTML:  {"blank", "captura-de-leads"},
}

func TestEveryRequiredStarterExists(t *testing.T) {
	all, err := Starters()
	if err != nil {
		t.Fatal(err)
	}
	for stack, keys := range requiredStarters {
		for _, key := range keys {
			starter, err := StarterFor(stack, key)
			if err != nil {
				t.Fatalf("%s/%s: %v", stack, key, err)
			}
			if starter.Title == "" || starter.Description == "" || len(starter.Files) == 0 {
				t.Fatalf("%s/%s is incomplete: %+v", stack, key, starter)
			}
		}
	}
	if len(all) < 4 {
		t.Fatalf("got %d starters", len(all))
	}
	if _, err := StarterFor(site.StackHTML, "nao-existe"); !errors.Is(err, ErrStarterUnknown) {
		t.Fatalf("got %v", err)
	}
	if _, err := StarterFor("vue", "blank"); !errors.Is(err, ErrStarterUnknown) {
		t.Fatalf("got %v", err)
	}
}

func TestEveryStarterPassesTheProjectRules(t *testing.T) {
	all, err := Starters()
	if err != nil {
		t.Fatal(err)
	}
	for _, starter := range all {
		t.Run(string(starter.Stack)+"/"+starter.Key, func(t *testing.T) {
			project := starter.Project()
			if issues := site.Validate(project); len(issues) != 0 {
				t.Fatalf("issues %+v", issues)
			}
			for _, path := range project.Paths() {
				if kind, _ := site.KindOf(project.Stack, path); kind == site.KindStyle {
					if issues := site.CheckCSS(project, path); len(issues) != 0 {
						t.Fatalf("%s: %+v", path, issues)
					}
					if !strings.Contains(project.Files[path].Content, `@import "tailwindcss"`) || !strings.Contains(project.Files[path].Content, "@theme") {
						t.Fatalf("%s is not a Tailwind v4 theme", path)
					}
				}
			}
			if err := starter.Settings.Validate(); err != nil {
				t.Fatal(err)
			}
			if issues := site.CheckImports(project.Stack, importsOf(project)); len(issues) != 0 {
				t.Fatalf("imports %+v", issues)
			}
			for path, file := range project.Files {
				if strings.ContainsAny(file.Content, "–—") {
					t.Fatalf("%s has an em or en dash", path)
				}
			}
		})
	}
}

var importSpecifier = regexp.MustCompile(`(?m)^\s*import\s+(?:[^"']+\s+from\s+)?["']([^"']+)["']`)

func importsOf(p site.Project) []site.Import {
	var out []site.Import
	for _, path := range p.Paths() {
		if !strings.HasSuffix(path, ".jsx") && !strings.HasSuffix(path, ".js") {
			continue
		}
		for _, match := range importSpecifier.FindAllStringSubmatch(p.Files[path].Content, -1) {
			if strings.HasSuffix(match[1], ".css") {
				continue
			}
			out = append(out, site.Import{Importer: path, Specifier: match[1]})
		}
	}
	return out
}

func TestLeadStartersCarryTheFormAndWhatsAppContract(t *testing.T) {
	for _, stack := range site.Stacks() {
		starter, err := StarterFor(stack, "captura-de-leads")
		if err != nil {
			t.Fatal(err)
		}
		joined := ""
		for _, path := range starter.Project().Paths() {
			joined += starter.Files[path]
		}
		for _, want := range []string{`data-vz-form="contato"`, `name="lead.name"`, `name="lead.phone"`, `name="lead.email"`, `name="consent"`, `data-vz-whatsapp`, `href="/privacidade"`} {
			if !strings.Contains(joined, want) {
				t.Fatalf("%s misses %s", stack, want)
			}
		}
		if starter.Settings.Forms["contato"].Success == nil {
			t.Fatalf("%s: the form needs a success action", stack)
		}
	}
}

func TestTheHTMLLeadStarterRendersAValidFormWithConsent(t *testing.T) {
	starter, err := StarterFor(site.StackHTML, "captura-de-leads")
	if err != nil {
		t.Fatal(err)
	}
	include := regexp.MustCompile(`<vz-include src="([^"]+)"></vz-include>`)
	manifest, err := site.DecodeManifest(site.StackHTML, []byte(starter.Files[site.ManifestPath]))
	if err != nil {
		t.Fatal(err)
	}
	var pages []site.RenderedPage
	for _, route := range manifest.Routes {
		body := include.ReplaceAllStringFunc(starter.Files[route.Page], func(tag string) string {
			return starter.Files[include.FindStringSubmatch(tag)[1]]
		})
		doc, err := sitehtml.Parse(body)
		if err != nil {
			t.Fatalf("%s: %v", route.Page, err)
		}
		pages = append(pages, site.RenderedPage{Route: route.Path, Document: doc})
	}
	checked := site.CheckPages(pages)
	issues, manifests := checked.Issues, checked.Manifests
	if len(issues) != 0 {
		t.Fatalf("issues %+v", issues)
	}
	if len(manifests) != 1 || manifests[0].ID != "contato" || !manifests[0].HasIdentity() || manifests[0].ConsentText == "" {
		t.Fatalf("manifests %+v", manifests)
	}
	for _, gap := range checked.OpenGaps(pages, starter.Settings) {
		if gap.Code != site.CodeLinkEmpty {
			t.Fatalf("only the WhatsApp number is left to fill: %+v", gap)
		}
	}
}

func TestStartersAreListedInAStableOrder(t *testing.T) {
	all, _ := Starters()
	keys := make([]string, 0, len(all))
	for _, s := range all {
		keys = append(keys, string(s.Stack)+"/"+s.Key)
	}
	if !slices.IsSorted(keys) {
		t.Fatalf("got %v", keys)
	}
}

func TestEveryStarterShipsItsPreviewImage(t *testing.T) {
	all, err := Starters()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range all {
		if len(s.Preview) < 3 || s.Preview[0] != 0xFF || s.Preview[1] != 0xD8 {
			t.Fatalf("%s/%s needs a jpeg %s; regenerate with cmd/sitestarterpreviews", s.Stack, s.Key, StarterPreviewFile)
		}
	}
	sources, err := StarterSources()
	if err != nil || len(sources) != len(all) || sources[0].Preview != nil {
		t.Fatalf("the sources load without previews: %d %v", len(sources), err)
	}
}
