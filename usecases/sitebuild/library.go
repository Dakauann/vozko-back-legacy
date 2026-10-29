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
	Group  SectionGroup
}

type SectionGroup string

const (
	GroupSections SectionGroup = "sections"
	GroupForms    SectionGroup = "forms"
	GroupFields   SectionGroup = "fields"
	GroupButtons  SectionGroup = "buttons"
	GroupMedia    SectionGroup = "media"
	GroupPlatform SectionGroup = "platform"
	GroupSaved    SectionGroup = "saved"
)

var sectionCatalog = []struct {
	key   string
	group SectionGroup
	title SectionTitle
}{
	{"hero", GroupSections, SectionTitle{PT: "Topo com chamada", EN: "Hero", ES: "Portada", DE: "Hero-Bereich"}},
	{"beneficios", GroupSections, SectionTitle{PT: "Benefícios", EN: "Benefits", ES: "Beneficios", DE: "Vorteile"}},
	{"depoimentos", GroupSections, SectionTitle{PT: "Depoimentos", EN: "Testimonials", ES: "Testimonios", DE: "Kundenstimmen"}},
	{"faq", GroupSections, SectionTitle{PT: "Perguntas frequentes", EN: "FAQ", ES: "Preguntas frecuentes", DE: "Häufige Fragen"}},
	{"precos", GroupSections, SectionTitle{PT: "Planos e preços", EN: "Pricing", ES: "Planes y precios", DE: "Preise"}},
	{"formulario", GroupForms, SectionTitle{PT: "Orçamento", EN: "Quote", ES: "Presupuesto", DE: "Angebot"}},
	{"form-contato", GroupForms, SectionTitle{PT: "Contato", EN: "Contact", ES: "Contacto", DE: "Kontakt"}},
	{"form-agendamento", GroupForms, SectionTitle{PT: "Agendamento", EN: "Appointment request", ES: "Solicitud de cita", DE: "Terminanfrage"}},
	{"cta-whatsapp", GroupSections, SectionTitle{PT: "Chamada para o WhatsApp", EN: "WhatsApp call to action", ES: "Llamada a WhatsApp", DE: "WhatsApp-Aufruf"}},
	{"galeria", GroupSections, SectionTitle{PT: "Galeria", EN: "Gallery", ES: "Galería", DE: "Galerie"}},
	{"equipe", GroupSections, SectionTitle{PT: "Equipe", EN: "Team", ES: "Equipo", DE: "Team"}},
	{"rodape", GroupSections, SectionTitle{PT: "Rodapé", EN: "Footer", ES: "Pie de página", DE: "Fußzeile"}},
	{"campo-texto", GroupFields, SectionTitle{PT: "Texto", EN: "Text", ES: "Texto", DE: "Text"}},
	{"campo-whatsapp", GroupFields, SectionTitle{PT: "WhatsApp", EN: "WhatsApp", ES: "WhatsApp", DE: "WhatsApp"}},
	{"campo-email", GroupFields, SectionTitle{PT: "E-mail", EN: "Email", ES: "Correo", DE: "E-Mail"}},
	{"campo-selecao", GroupFields, SectionTitle{PT: "Seleção", EN: "Dropdown", ES: "Selección", DE: "Auswahl"}},
	{"campo-caixa", GroupFields, SectionTitle{PT: "Caixa de marcar", EN: "Checkbox", ES: "Casilla", DE: "Kontrollkästchen"}},
	{"botao", GroupButtons, SectionTitle{PT: "Botão", EN: "Button", ES: "Botón", DE: "Schaltfläche"}},
	{"botao-contorno", GroupButtons, SectionTitle{PT: "Botão com contorno", EN: "Outline button", ES: "Botón con borde", DE: "Umrandete Schaltfläche"}},
	{"link-texto", GroupButtons, SectionTitle{PT: "Link", EN: "Link", ES: "Enlace", DE: "Link"}},
	{"imagem", GroupMedia, SectionTitle{PT: "Imagem", EN: "Image", ES: "Imagen", DE: "Bild"}},
	{"video", GroupMedia, SectionTitle{PT: "Vídeo", EN: "Video", ES: "Vídeo", DE: "Video"}},
	{"botao-whatsapp", GroupPlatform, SectionTitle{PT: "Botão de WhatsApp", EN: "WhatsApp button", ES: "Botón de WhatsApp", DE: "WhatsApp-Schaltfläche"}},
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
		lib.infos = append(lib.infos, SectionInfo{Key: entry.key, Title: entry.title, Stacks: slices.Clone(sectionStacks), Kind: SectionFromLibrary, Group: entry.group})
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
		out[i] = SectionInfo{Key: info.Key, Title: info.Title, Stacks: slices.Clone(info.Stacks), Kind: info.Kind, Group: info.Group}
	}
	return out
}
