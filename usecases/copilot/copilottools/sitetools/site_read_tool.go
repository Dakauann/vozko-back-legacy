package sitetools

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"vozko/domain/copilot"
	"vozko/domain/site"
	"vozko/domain/tools"
	"vozko/usecases/copilot/copilottools"
	site_usecase "vozko/usecases/site"
)

const (
	inspectLimit         = 40 * time.Second
	maxReadLines         = 300
	maxReadBytes         = 24000
	readNoFile           = "o arquivo %q não existe neste site; os caminhos estão no mapa (site_read sem path)"
	readBadRange         = "from e to são linhas a partir de 1, com from até to"
	readPastEnd          = "o arquivo %q tem %d linhas; peça um trecho dentro delas"
	readRangeLabel       = "%d-%d de %d"
	readTruncatedEnd     = "o trecho foi cortado em %d bytes; peça as linhas seguintes a partir de %d"
	readNoReferences     = "as referências do workspace não puderam ser lidas agora"
	bookingReferenceName = "%d min, %s"
)

type siteReadArgs struct {
	Path       string `json:"path,omitempty" desc:"um caminho do mapa; sem path, o mapa do site"`
	From       int    `json:"from,omitempty" desc:"com path: primeira linha, a partir de 1"`
	To         int    `json:"to,omitempty" desc:"com path: última linha; até 300 por leitura"`
	References bool   `json:"references,omitempty" desc:"sem path: inclui os ids e nomes de números de WhatsApp, funis, etapas, pessoas, times, fluxos, caixas de e-mail, chats e agendamentos, e se a agenda Google está conectada"`
}

type siteReadTool struct {
	siteScope
	deps Deps
}

func NewSiteReadTool(deps Deps) copilot.Tool {
	return &siteReadTool{deps: deps}
}

func (t *siteReadTool) Meta() copilot.Meta { return readMeta() }

func (t *siteReadTool) Definition() tools.Definition {
	return copilottools.Definition("site_read",
		"Lê o site aberto como ele está guardado agora, inclusive o que o usuário mudou. Sem path devolve o mapa: arquivos (tamanho, rev, componentes exportados, seções, formulários), "+
			"rotas, formulários com o destino dos leads, um resumo das configurações, a rota e a seleção na tela do usuário e open_gaps (o que impede publicar). "+
			"Com path devolve o texto daquele arquivo, por linhas, para copiar o old exato de edit_file. Nunca devolve o site inteiro de uma vez.",
		siteReadArgs{})
}

func (t *siteReadTool) Execute(ctx context.Context, cc copilot.Context, args map[string]interface{}) copilot.Result {
	var a siteReadArgs
	if err := copilottools.DecodeArgs(args, &a); err != nil {
		return failure(err.Error())
	}
	view, err := t.deps.open(ctx, cc, forReading)
	if err != nil {
		return failure(err.Error())
	}
	if path := strings.TrimSpace(a.Path); path != "" {
		return readFile(view.Files, path, a.From, a.To)
	}
	body := asPageData(projectMap(view))
	if screen, ok := readEditor(ctx, cc); ok {
		route := screen.Route
		if route == "" {
			route = site.HomeRoute
		}
		body["screen_route"] = route
		body["selection"] = screen.Selection.line()
		if unsaved := unsavedIn(view, screen); len(unsaved) > 0 {
			body["unsaved_in_editor"] = unsaved
		}
	}
	if a.References {
		body["references"] = t.references(ctx, view)
	}
	inspectCtx, cancel := bounded(ctx, inspectLimit)
	defer cancel()
	return copilot.Result{Status: copilot.StatusOK, Data: withOpenGaps(body, t.deps.inspect(inspectCtx, cc, view))}
}

func (t *siteReadTool) references(ctx context.Context, view *site_usecase.View) interface{} {
	if t.deps.References == nil {
		return map[string]string{"unread": readNoReferences}
	}
	catalog, err := t.deps.References.Catalog(ctx, view.Project.WorkspaceID)
	if err != nil {
		return map[string]string{"unread": readNoReferences}
	}
	catalog.BookingBlocks = bookingReferences(settingsOf(view))
	return catalog
}

func bookingReferences(settings site.Settings) []site_usecase.ReferenceItem {
	out := []site_usecase.ReferenceItem{}
	for _, id := range slices.Sorted(maps.Keys(settings.Bookings)) {
		b := settings.Bookings[id]
		out = append(out, site_usecase.ReferenceItem{ID: id, Name: fmt.Sprintf(bookingReferenceName, b.DurationMinutes, b.Hours.Timezone)})
	}
	return out
}

func readFile(files map[string]site.File, path string, from, to int) copilot.Result {
	file, ok := files[path]
	if !ok {
		return failure(fmt.Sprintf(readNoFile, path))
	}
	lines := strings.SplitAfter(file.Content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	total := len(lines)
	if from == 0 {
		from = 1
	}
	if to == 0 {
		to = from + maxReadLines - 1
	}
	if from < 1 || to < from {
		return failure(readBadRange)
	}
	if total > 0 && from > total {
		return failure(fmt.Sprintf(readPastEnd, path, total))
	}
	to = min(to, total, from+maxReadLines-1)
	var text strings.Builder
	last := from - 1
	for i := from - 1; i < to; i++ {
		if text.Len()+len(lines[i]) > maxReadBytes {
			break
		}
		text.WriteString(lines[i])
		last = i + 1
	}
	body := map[string]interface{}{
		"path":  path,
		"rev":   file.Rev,
		"lines": fmt.Sprintf(readRangeLabel, from, last, total),
		"text":  text.String(),
	}
	if last < to {
		body["note"] = fmt.Sprintf(readTruncatedEnd, maxReadBytes, last+1)
	}
	return copilot.Result{Status: copilot.StatusOK, Data: asPageData(body)}
}
