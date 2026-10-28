package sitetools

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"vozko/domain/site"
	site_usecase "vozko/usecases/site"
)

var (
	exportedDefault = regexp.MustCompile(`export\s+default\s+function\s+([A-Za-z_]\w*)`)
	exportedNamed   = regexp.MustCompile(`export\s+(?:function|const|class)\s+([A-Za-z_]\w*)`)
	sectionOpen     = regexp.MustCompile(`<section\b[^>]*>`)
	sectionID       = regexp.MustCompile(`\bid=["']([^"']+)["']`)
	formID          = regexp.MustCompile(`data-vz-form=["']([^"']*)["']`)
)

type fileOutline struct {
	exports  []string
	sections []string
	unnamed  int
	forms    []string
}

func outlineOf(content string) fileOutline {
	var o fileOutline
	for _, m := range exportedDefault.FindAllStringSubmatch(content, -1) {
		o.exports = append(o.exports, m[1])
	}
	for _, m := range exportedNamed.FindAllStringSubmatch(content, -1) {
		if !slices.Contains(o.exports, m[1]) {
			o.exports = append(o.exports, m[1])
		}
	}
	for _, tag := range sectionOpen.FindAllString(content, -1) {
		if m := sectionID.FindStringSubmatch(tag); m != nil {
			o.sections = append(o.sections, m[1])
		} else {
			o.unnamed++
		}
	}
	for _, m := range formID.FindAllStringSubmatch(content, -1) {
		o.forms = append(o.forms, m[1])
	}
	return o
}

func sizeLabel(bytes int) string {
	if bytes < 1000 {
		return strconv.Itoa(bytes) + " B"
	}
	return strconv.FormatFloat(float64(bytes)/1000, 'f', 1, 64) + "k"
}

func fileLine(path string, file site.File) string {
	o := outlineOf(file.Content)
	parts := []string{path + " " + sizeLabel(len(file.Content)) + " r" + strconv.FormatUint(file.Rev, 10)}
	if len(o.exports) > 0 {
		parts = append(parts, "exporta "+strings.Join(o.exports, ", "))
	}
	if len(o.sections) > 0 || o.unnamed > 0 {
		names := append([]string(nil), o.sections...)
		if o.unnamed > 0 {
			names = append(names, strconv.Itoa(o.unnamed)+" sem id")
		}
		parts = append(parts, "seções "+strings.Join(names, ", "))
	}
	if len(o.forms) > 0 {
		parts = append(parts, "form "+strings.Join(o.forms, ", "))
	}
	return strings.Join(parts, "; ")
}

func routeLines(view *site_usecase.View) []string {
	raw, ok := view.Files[site.ManifestPath]
	if !ok {
		return nil
	}
	manifest, err := site.DecodeManifest(view.Project.Stack, []byte(raw.Content))
	if err != nil {
		return []string{"site.json inválido"}
	}
	lines := make([]string, 0, len(manifest.Routes))
	for _, r := range manifest.Routes {
		lines = append(lines, r.Path+" "+r.Page)
	}
	return lines
}

func formDestination(f site.FormSettings) string {
	var parts []string
	if f.FunnelID != "" {
		parts = append(parts, "funil")
	}
	if f.Owner != nil {
		parts = append(parts, "dono "+string(f.Owner.Kind))
	}
	if f.WorkflowID != "" {
		parts = append(parts, "workflow")
	}
	if f.Success != nil {
		parts = append(parts, "sucesso "+string(f.Success.Kind))
	}
	if f.Notify {
		parts = append(parts, "avisa a equipe")
	}
	if len(parts) == 0 {
		return "sem destino"
	}
	return strings.Join(parts, ", ")
}

func formLines(view *site_usecase.View, settings site.Settings) []string {
	found := map[string]string{}
	for _, path := range slices.Sorted(maps.Keys(view.Files)) {
		for _, id := range outlineOf(view.Files[path].Content).forms {
			if _, seen := found[id]; !seen {
				found[id] = path
			}
		}
	}
	var lines []string
	for _, id := range slices.Sorted(maps.Keys(found)) {
		destination := "sem configuração"
		if f, ok := settings.Forms[id]; ok {
			destination = formDestination(f)
		}
		lines = append(lines, fmt.Sprintf("%s (%s): %s", id, found[id], destination))
	}
	for _, id := range slices.Sorted(maps.Keys(settings.Forms)) {
		if _, ok := found[id]; !ok {
			lines = append(lines, id+": configurado, mas nenhum arquivo tem esse data-vz-form")
		}
	}
	return lines
}

func settingsLines(settings site.Settings) []string {
	var seo []string
	for _, route := range slices.Sorted(maps.Keys(settings.Pages)) {
		meta := settings.Pages[route]
		mark := route
		if meta.NoIndex {
			mark += " noindex"
		}
		if strings.TrimSpace(meta.Title) == "" {
			mark += " sem título"
		}
		seo = append(seo, mark)
	}
	integrations := []string{}
	for _, kind := range settings.Integrations.Enabled() {
		integrations = append(integrations, string(kind))
	}
	lines := []string{"seo: " + orNone(seo), "integrações: " + orNone(integrations)}
	banner := "desligado"
	if settings.Consent.Banner {
		banner = "ligado"
	}
	lines = append(lines, "banner de consentimento: "+banner)
	if len(settings.Redirects) > 0 {
		lines = append(lines, "redirecionamentos: "+strconv.Itoa(len(settings.Redirects)))
	}
	return lines
}

func orNone(values []string) string {
	if len(values) == 0 {
		return "nenhum"
	}
	return strings.Join(values, ", ")
}

func projectMap(view *site_usecase.View) map[string]interface{} {
	files := make([]string, 0, len(view.Files))
	for _, path := range slices.Sorted(maps.Keys(view.Files)) {
		files = append(files, fileLine(path, view.Files[path]))
	}
	settings := settingsOf(view)
	body := map[string]interface{}{
		"name":     view.Project.Name,
		"stack":    view.Project.Stack,
		"status":   view.Project.Status,
		"files":    files,
		"routes":   routeLines(view),
		"settings": settingsLines(settings),
	}
	if forms := formLines(view, settings); len(forms) > 0 {
		body["forms"] = forms
	}
	return body
}

func unsavedIn(view *site_usecase.View, screen editorRead) []string {
	var paths []string
	for _, path := range slices.Sorted(maps.Keys(screen.Files)) {
		stored, ok := view.Files[path]
		if !ok || screen.Files[path] != stored.Rev {
			paths = append(paths, path)
		}
	}
	for _, path := range slices.Sorted(maps.Keys(view.Files)) {
		if _, ok := screen.Files[path]; !ok {
			paths = append(paths, path)
		}
	}
	return paths
}
