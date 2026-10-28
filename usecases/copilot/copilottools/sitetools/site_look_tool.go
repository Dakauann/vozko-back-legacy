package sitetools

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"vozko/domain/copilot"
	"vozko/domain/imageguard"
	"vozko/domain/site"
	"vozko/domain/sitehtml"
	"vozko/domain/tools"
	"vozko/usecases/copilot/copilottools"
	"vozko/usecases/copilot/copilottools/screencall"
	site_usecase "vozko/usecases/site"
	"vozko/usecases/siterender"
)

const (
	lookLimit       = 90 * time.Second
	lookBlind       = "o modelo desta conversa não enxerga imagens, então nada foi visto; diga isso ao usuário sem descrever a página e sugira trocar, no seletor de modelos, para um modelo com visão"
	lookNoRoute     = "route %q não é uma rota desta página; as rotas estão no mapa (site_read)"
	lookNoPreview   = "a prévia da página não está disponível agora, então nada foi visto; tente de novo em instantes"
	lookBuildFails  = "a página não compila, então nada foi visto; corrija com site_edit: %s"
	lookStale       = "focus_node não está mais na página; leia de novo e use um node atual"
	lookBadFocus    = "focus_node não é um node; use um node do mapa (site_read) ou dos marks"
	lookNoImage     = "o editor não devolveu uma imagem legível da página; tente de novo"
	lookBoth        = "both"
	lookDesktop     = "desktop"
	lookMobile      = "mobile"
	lookDetailRunes = 48
	lookGapLines    = 5
	focusMissing    = "focus_missing"
)

var lookImageLimits = imageguard.Limits{MaxBytes: copilot.MaxScreenImageBytes, MinWidth: 16, MinHeight: 16, MaxWidth: 2048, MaxHeight: 2048}

type siteLookArgs struct {
	Route     string `json:"route,omitempty" desc:"rota a ver; padrão a rota na tela do usuário"`
	Device    string `json:"device,omitempty" enum:"both,desktop,mobile" desc:"both (padrão): 1440 e 390 px lado a lado"`
	Marks     bool   `json:"marks,omitempty" desc:"numera os elementos e liga cada número ao node na resposta"`
	FocusNode string `json:"focus_node,omitempty" desc:"aproxima um node para ler detalhes"`
}

type lookViewport struct {
	W int `json:"w"`
	H int `json:"h"`
}

type lookMarks struct {
	Limit int        `json:"limit"`
	MinW  int        `json:"min_w"`
	MinH  int        `json:"min_h"`
	Ranks [][]string `json:"ranks"`
}

type lookOrder struct {
	PreviewURL string         `json:"preview_url"`
	Route      string         `json:"route"`
	Viewports  []lookViewport `json:"viewports"`
	FocusNode  string         `json:"focus_node,omitempty"`
	Marks      *lookMarks     `json:"marks,omitempty"`
}

type siteLookTool struct {
	siteScope
	deps Deps
}

func NewSiteLookTool(deps Deps) copilot.Tool {
	return &siteLookTool{deps: deps}
}

func (t *siteLookTool) Meta() copilot.Meta { return readMeta() }

func (t *siteLookTool) Definition() tools.Definition {
	return copilottools.Definition("site_look",
		"Mostra para você uma imagem da página aberta renderizada pelo editor do usuário, como o visitante vê: computador e celular, a página inteira em faixas na ordem, depois de rolar uma vez (entradas e imagens carregadas). "+
			"A resposta traz viewports, marks (número, node e tag) e checks (erros do console, estouro horizontal, contraste, imagens pequenas, desempenho), e open_gaps. Use antes de dizer que terminou.",
		siteLookArgs{})
}

func viewportsFor(device string) []lookViewport {
	desktop := lookViewport{W: siterender.Desktop.Width, H: siterender.Desktop.Height}
	mobile := lookViewport{W: siterender.Mobile.Width, H: siterender.Mobile.Height}
	switch device {
	case lookDesktop:
		return []lookViewport{desktop}
	case lookMobile:
		return []lookViewport{mobile}
	}
	return []lookViewport{desktop, mobile}
}

func previewFailure(err error, template string) string {
	var failed *site_usecase.BuildFailedError
	if errors.As(err, &failed) {
		return fmt.Sprintf(template, strings.Join(buildLines(failed.Errors), "; "))
	}
	var broken *site_usecase.IssuesError
	if errors.As(err, &broken) {
		return fmt.Sprintf(template, strings.Join(ruleLines(broken.Issues), "; "))
	}
	return lookNoPreview
}

func (t *siteLookTool) Execute(ctx context.Context, cc copilot.Context, args map[string]interface{}) copilot.Result {
	var a siteLookArgs
	if err := copilottools.DecodeArgs(args, &a); err != nil {
		return failure(err.Error())
	}
	if !cc.SeesImages {
		return failure(lookBlind)
	}
	focus := strings.TrimSpace(a.FocusNode)
	if focus != "" {
		if _, _, err := sitehtml.ParseNodeID(focus); err != nil {
			return failure(lookBadFocus)
		}
	}
	view, err := t.deps.open(ctx, cc, forReading)
	if err != nil {
		return failure(err.Error())
	}
	route := strings.TrimSpace(a.Route)
	if route == "" {
		route = site.HomeRoute
		if screen, ok := readEditor(ctx, cc); ok && screen.Route != "" {
			route = screen.Route
		}
	}
	routes, err := routesOf(view)
	if err != nil {
		return failure(err.Error())
	}
	if !slices.Contains(routes, route) {
		return failure(fmt.Sprintf(lookNoRoute, route))
	}
	lookCtx, cancel := bounded(ctx, lookLimit)
	defer cancel()
	preview, err := t.deps.Storage.Preview(lookCtx, cc.WorkspaceID, view.Project.ID)
	if err != nil || preview.URL == "" {
		return failure(previewFailure(err, lookBuildFails))
	}
	order := lookOrder{PreviewURL: preview.URL, Route: route, Viewports: viewportsFor(a.Device), FocusNode: focus}
	if a.Marks {
		order.Marks = &lookMarks{Limit: site.MaxMarks, MinW: site.MinMarkWidth, MinH: site.MinMarkHeight, Ranks: site.MarkRanks()}
	}
	reply, err := screencall.Run(lookCtx, cc, copilot.ScreenLook, order, editorWords)
	if err != nil {
		if reply.Error != nil && reply.Error.Code == focusMissing {
			return failure(lookStale)
		}
		return screencall.Result(cc, err)
	}
	if len(reply.Images) != 1 {
		return failure(lookNoImage)
	}
	if _, err := imageguard.JPEGDataURL(reply.Images[0], lookImageLimits); err != nil {
		return failure(lookNoImage)
	}
	captured, err := decodeLook(reply.Data, order)
	if err != nil {
		return failure(siteUnreadable)
	}
	checks, omitted := captured.checks(route, order.Viewports)
	body := map[string]interface{}{"route": route, "viewports": captured.viewportLines(), "checks": lookCheckLines(checks)}
	if marks := captured.markLines(); len(marks) > 0 {
		body["marks"] = marks
	}
	if omitted > 0 {
		body["omitted_checks"] = omitted
	}
	in := t.deps.inspect(lookCtx, cc, view)
	return copilot.Result{Status: copilot.StatusOK, Data: withFewOpenGaps(body, in, lookGapLines), Images: []string{reply.Images[0]}}
}

func lookCheckLines(checks []site.Check) []string {
	lines := make([]string, 0, len(checks))
	for _, c := range checks {
		c.Route = ""
		lines = append(lines, checkLineWithin(c, lookDetailRunes))
	}
	return lines
}
