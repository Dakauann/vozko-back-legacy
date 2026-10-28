package sitebuild

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
	"sync"

	"vozko/domain/site"
)

//go:embed sections/*.html
var sectionFiles embed.FS

type SectionTitle struct {
	PT string `json:"pt"`
	EN string `json:"en"`
	ES string `json:"es"`
	DE string `json:"de"`
}

type SectionKind string

const (
	SectionFromLibrary SectionKind = "library"
	SectionSnippet     SectionKind = "snippet"
)

type SectionInfo struct {
	Key    string
	Title  SectionTitle
	Stacks []site.Stack
	Kind   SectionKind
}

var sectionCatalog = []struct {
	key   string
	title SectionTitle
}{
	{"hero", SectionTitle{PT: "Topo com chamada", EN: "Hero", ES: "Portada", DE: "Hero-Bereich"}},
	{"beneficios", SectionTitle{PT: "Benefícios", EN: "Benefits", ES: "Beneficios", DE: "Vorteile"}},
	{"depoimentos", SectionTitle{PT: "Depoimentos", EN: "Testimonials", ES: "Testimonios", DE: "Kundenstimmen"}},
	{"faq", SectionTitle{PT: "Perguntas frequentes", EN: "FAQ", ES: "Preguntas frecuentes", DE: "Häufige Fragen"}},
	{"precos", SectionTitle{PT: "Planos e preços", EN: "Pricing", ES: "Planes y precios", DE: "Preise"}},
	{"formulario", SectionTitle{PT: "Formulário de contato", EN: "Contact form", ES: "Formulario de contacto", DE: "Kontaktformular"}},
	{"cta-whatsapp", SectionTitle{PT: "Chamada para o WhatsApp", EN: "WhatsApp call to action", ES: "Llamada a WhatsApp", DE: "WhatsApp-Aufruf"}},
	{"galeria", SectionTitle{PT: "Galeria", EN: "Gallery", ES: "Galería", DE: "Galerie"}},
	{"equipe", SectionTitle{PT: "Equipe", EN: "Team", ES: "Equipo", DE: "Team"}},
	{"rodape", SectionTitle{PT: "Rodapé", EN: "Footer", ES: "Pie de página", DE: "Fußzeile"}},
}

var sectionStacks = []site.Stack{site.StackHTML, site.StackReact}

type Library struct {
	sources map[string]string
	infos   []SectionInfo
}

var loadSections = sync.OnceValues(func() (*Library, error) {
	return newLibrary(sectionFiles)
})

func Sections() (*Library, error) {
	return loadSections()
}

func newLibrary(files fs.FS) (*Library, error) {
	entries, err := fs.Glob(files, "sections/*.html")
	if err != nil {
		return nil, err
	}
	lib := &Library{sources: map[string]string{}}
	for _, entry := range sectionCatalog {
		raw, err := fs.ReadFile(files, "sections/"+entry.key+".html")
		if err != nil || strings.TrimSpace(string(raw)) == "" {
			return nil, fmt.Errorf("sitebuild: the section %s has no source", entry.key)
		}
		lib.sources[entry.key] = string(raw)
		lib.infos = append(lib.infos, SectionInfo{Key: entry.key, Title: entry.title, Stacks: slices.Clone(sectionStacks), Kind: SectionFromLibrary})
	}
	for _, file := range entries {
		if _, ok := lib.sources[strings.TrimSuffix(path.Base(file), ".html")]; !ok {
			return nil, fmt.Errorf("sitebuild: the section file %s has no catalog entry", file)
		}
	}
	return lib, nil
}

func (l *Library) Section(stack site.Stack, key string) (string, bool) {
	if !slices.Contains(sectionStacks, stack) {
		return "", false
	}
	source, ok := l.sources[key]
	return source, ok
}

func (l *Library) List() []SectionInfo {
	out := make([]SectionInfo, len(l.infos))
	for i, info := range l.infos {
		out[i] = SectionInfo{Key: info.Key, Title: info.Title, Stacks: slices.Clone(info.Stacks), Kind: info.Kind}
	}
	return out
}
