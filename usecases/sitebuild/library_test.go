package sitebuild

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"vozko/domain/site"
	"vozko/domain/siteform"
	"vozko/domain/sitehtml"
)

var librarySections = []string{"hero", "beneficios", "depoimentos", "faq", "precos", "formulario", "cta-whatsapp", "galeria", "equipe", "rodape"}

func library(t *testing.T) *Library {
	t.Helper()
	lib, err := Sections()
	if err != nil {
		t.Fatal(err)
	}
	return lib
}

func TestTheLibraryListsEverySectionWithFourTitlesAndBothStacks(t *testing.T) {
	lib := library(t)
	var keys []string
	for _, info := range lib.List() {
		keys = append(keys, info.Key)
		if info.Title.PT == "" || info.Title.EN == "" || info.Title.ES == "" || info.Title.DE == "" {
			t.Fatalf("%s needs a title in pt, en, es and de: %+v", info.Key, info.Title)
		}
		if !slices.Equal(info.Stacks, []site.Stack{site.StackHTML, site.StackReact}) {
			t.Fatalf("%s stacks = %v", info.Key, info.Stacks)
		}
		for _, stack := range info.Stacks {
			if body, ok := lib.Section(stack, info.Key); !ok || strings.TrimSpace(body) == "" {
				t.Fatalf("%s has no %s source", info.Key, stack)
			}
		}
	}
	if !slices.Equal(keys, librarySections) {
		t.Fatalf("keys = %v", keys)
	}
	if _, ok := lib.Section(site.StackHTML, "nada"); ok {
		t.Fatal("an unknown key is not a section")
	}
	if _, ok := lib.Section(site.Stack("vue"), "hero"); ok {
		t.Fatal("an unknown stack has no sections")
	}
}

func sectionPage(section string) string {
	return `<!DOCTYPE html><html lang="pt-BR"><head><meta charset="utf-8"><title>Página</title></head><body><main>` + section + `</main><a href="/privacidade">Política de Privacidade</a></body></html>`
}

func TestEverySectionPassesThePublishingRules(t *testing.T) {
	lib := library(t)
	for _, key := range librarySections {
		t.Run(key, func(t *testing.T) {
			body, _ := lib.Section(site.StackHTML, key)
			doc, err := sitehtml.Parse(sectionPage(body))
			if err != nil {
				t.Fatal(err)
			}
			pages := []site.RenderedPage{{Route: "/", Document: doc}}
			checked := site.CheckPages(pages)
			issues, manifests := checked.Issues, checked.Manifests
			if structural, _ := site.Split(issues); len(structural) > 0 {
				t.Fatalf("structural issues %+v", structural)
			}
			for _, gap := range checked.OpenGaps(pages, site.Settings{}) {
				if gap.Code != site.CodeLinkEmpty || !whatsAppAnchor(doc, gap.Detail) {
					t.Fatalf("open gap %+v", gap)
				}
			}
			if key == "formulario" {
				if len(manifests) != 1 || !manifests[0].HasIdentity() || strings.TrimSpace(manifests[0].ConsentText) == "" {
					t.Fatalf("the form section needs an identity field and consent: %+v", manifests)
				}
				checkFormContract(t, manifests[0])
			} else if len(manifests) != 0 {
				t.Fatalf("only the form section carries a form: %+v", manifests)
			}
			project := site.NewProject(site.StackHTML, map[string]string{
				site.ManifestPath:  `{"name":"Página","lang":"pt-BR","routes":[{"path":"/","page":"pages/index.html"}]}`,
				site.ThemePath:     "@import \"tailwindcss\";\n",
				"pages/index.html": body,
			})
			if structural, _ := site.Split(site.Validate(project)); len(structural) > 0 {
				t.Fatalf("source issues %+v", structural)
			}
		})
	}
}

func whatsAppAnchor(doc *sitehtml.Document, elementPath string) bool {
	i, ok := doc.ByPath(elementPath)
	return ok && doc.HasAttr(i, "data-vz-whatsapp")
}

func checkFormContract(t *testing.T, m siteform.Manifest) {
	t.Helper()
	values, err := m.Validate(map[string]string{"lead.name": "Ana Souza", "lead.phone": "(11) 98888-7777", "lead.email": "ana@exemplo.com.br", "consent": "true"})
	if err != nil || values.Phone == "" || values.Email == "" {
		t.Fatalf("a filled form must validate: %+v %v", values, err)
	}
	if _, err := m.Validate(map[string]string{"lead.name": "Ana", "lead.phone": "(11) 98888-7777"}); err == nil {
		t.Fatal("consent is required")
	}
}

var (
	dynamicClass = regexp.MustCompile(`class="[^"]*[{}$][^"]*"`)
	imageTag     = regexp.MustCompile(`<img\b[^>]*>`)
	literalImage = regexp.MustCompile(`\bsrc="data:image/svg\+xml,[^"]+"`)
	altText      = regexp.MustCompile(`\balt="[^"]+"`)
)

func TestEverySectionStaysEditableByHandAndByElo(t *testing.T) {
	lib := library(t)
	for _, key := range librarySections {
		t.Run(key, func(t *testing.T) {
			body, _ := lib.Section(site.StackHTML, key)
			for _, forbidden := range []string{"<script", " style=", "{{", "${", "<svg", "http://", "https://"} {
				if strings.Contains(body, forbidden) && !(forbidden == "http://" && strings.Count(body, "http://") == strings.Count(body, "http://www.w3.org/2000/svg")) {
					t.Fatalf("%s must not contain %q", key, forbidden)
				}
			}
			if dynamicClass.MatchString(body) {
				t.Fatalf("%s classes must be literal", key)
			}
			for _, img := range imageTag.FindAllString(body, -1) {
				if !literalImage.MatchString(img) || !altText.MatchString(img) {
					t.Fatalf("%s image needs a literal neutral placeholder and alt text: %s", key, img)
				}
			}
			if !strings.HasPrefix(strings.TrimSpace(body), "<") || strings.Count(body, "<section")+strings.Count(body, "<footer") != 1 {
				t.Fatalf("%s must be exactly one section or footer root", key)
			}
		})
	}
}
