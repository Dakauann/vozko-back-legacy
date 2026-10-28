package sitetools

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"vozko/domain/copilot"
	"vozko/domain/site"
	"vozko/domain/strictjson"
	"vozko/usecases/copilot/copilottools"
	"vozko/usecases/copilot/copilottools/screencall"
)

const (
	watchTimeout     = 6 * time.Second
	editorReadLimit  = 3 * time.Second
	resultOpenGaps   = "open_gaps"
	resultGapsUnread = "open_gaps_unread"
	resultBuild      = "build_errors"
	snapshotOpen     = "\n\n# Página aberta\nO usuário está com a página %s aberta no editor de páginas, ao lado desta conversa (project_id %s).\n"
	snapshotScreen   = "Na tela: rota %s; seleção %s.\n"
	snapshotTools    = "site_read lê o mapa e os arquivos dessa página sem pedir id; site_edit, site_look, site_generate_image, site_settings e site_publish trabalham nela.\n"
	snapshotSilent   = "As pendências não puderam ser lidas no início desta resposta; site_read as lê de novo."
	snapshotGaps     = "Pendências que impedem publicar, lidas no início desta resposta: %d. site_read lista cada uma."
	snapshotNoGaps   = "Lido no início desta resposta: nada impede publicar."
	closingPending   = "a página ainda tem pendências que impedem publicar"
	closingNever     = "Nunca invente links, textos legais nem dados para fechar uma pendência."
	noSelection      = "nenhuma"
)

var gapWords = map[string][]string{
	site.CodeAltMissing:           {"alt", "alternativ", "descrição da imagem"},
	site.CodeLinkEmpty:            {"link", "enlace", "destino", "url"},
	site.CodeConsentRequired:      {"consentimento", "consent", "consentimiento", "einwilligung", "lgpd"},
	site.CodePrivacyLinkMissing:   {"privacidade", "privacy", "privacidad", "datenschutz"},
	site.CodeFormIdentityMissing:  {"telefone", "e-mail", "email", "phone", "teléfono", "contato", "contact"},
	site.CodeConsentBannerMissing: {"banner", "cookie", "consentimento", "consent"},
	site.CodeInvisibleWithoutJS:   {"invisível", "invisible", "animação", "animation", "javascript"},
}

type gappedResult struct {
	body  map[string]interface{}
	in    inspection
	limit int
}

func withOpenGaps(body map[string]interface{}, in inspection) gappedResult {
	return gappedResult{body: body, in: in, limit: maxLines}
}

func withFewOpenGaps(body map[string]interface{}, in inspection, limit int) gappedResult {
	return gappedResult{body: body, in: in, limit: limit}
}

func (g gappedResult) MarshalJSON() ([]byte, error) {
	body := make(map[string]interface{}, len(g.body)+1)
	for key, value := range g.body {
		body[key] = value
	}
	if len(g.in.buildErrors) > 0 {
		body[resultBuild] = g.in.buildErrors
	}
	if g.in.known {
		return copilottools.JSONEndingWith(body, resultOpenGaps, g.in.gapLinesUpTo(g.limit))
	}
	return copilottools.JSONEndingWith(body, resultGapsUnread, g.in.reason)
}

type editorSelection struct {
	Node string `json:"node"`
	Tag  string `json:"tag,omitempty"`
	Text string `json:"text,omitempty"`
}

type editorRead struct {
	Stack     string            `json:"stack"`
	Files     map[string]uint64 `json:"files"`
	Route     string            `json:"route"`
	Selection *editorSelection  `json:"selection"`
}

func decodeEditorRead(data []byte) (editorRead, error) {
	var got editorRead
	if err := strictjson.Decode(data, &got); err != nil || got.Files == nil {
		return editorRead{}, errors.New(siteUnreadable)
	}
	return got, nil
}

func readEditor(ctx context.Context, cc copilot.Context) (editorRead, bool) {
	ctx, cancel := bounded(ctx, editorReadLimit)
	defer cancel()
	reply, err := screencall.Run(ctx, cc, copilot.ScreenRead, map[string]string{}, editorWords)
	if err != nil {
		return editorRead{}, false
	}
	got, err := decodeEditorRead(reply.Data)
	return got, err == nil
}

func (s *editorSelection) line() string {
	if s == nil || strings.TrimSpace(s.Node) == "" {
		return noSelection
	}
	out := s.Node
	if s.Tag != "" {
		out = s.Tag + " " + out
	}
	if text := strings.TrimSpace(s.Text); text != "" {
		out += " " + strconv.Quote(clip(text, 60))
	}
	return out
}

func watching(cc copilot.Context) bool {
	return cc.View.OnSite() && cc.Screen != nil
}

func mentioned(reply string, gap site.Issue) bool {
	return copilottools.ReplyMentions(reply, slices.Concat([]string{gap.Code}, gapWords[gap.Code]))
}

func (t *siteReadTool) ViewSnapshot(ctx context.Context, cc copilot.Context) string {
	if !watching(cc) {
		return ""
	}
	ctx, cancel := bounded(ctx, watchTimeout)
	defer cancel()
	view, err := t.deps.open(ctx, cc, forReading)
	if err != nil {
		return fmt.Sprintf(snapshotOpen, "sem nome", cc.View.ProjectID) + snapshotTools + snapshotSilent
	}
	head := fmt.Sprintf(snapshotOpen, strconv.Quote(view.Project.Name), cc.View.ProjectID)
	if screen, ok := readEditor(ctx, cc); ok {
		route := screen.Route
		if route == "" {
			route = site.HomeRoute
		}
		head += fmt.Sprintf(snapshotScreen, route, screen.Selection.line())
	}
	head += snapshotTools
	in := t.deps.inspect(ctx, cc, view)
	switch {
	case !in.known:
		return head + snapshotSilent
	case len(in.gaps) == 0:
		return head + snapshotNoGaps
	}
	return head + fmt.Sprintf(snapshotGaps, len(in.gaps))
}

func (t *siteReadTool) ClosingNote(ctx context.Context, cc copilot.Context, reply string) string {
	if !watching(cc) {
		return ""
	}
	ctx, cancel := bounded(ctx, watchTimeout)
	defer cancel()
	view, err := t.deps.open(ctx, cc, forReading)
	if err != nil {
		return ""
	}
	in := t.deps.inspect(ctx, cc, view)
	if !in.known {
		return ""
	}
	var left []site.Issue
	for _, gap := range in.gaps {
		if !mentioned(reply, gap) {
			left = append(left, gap)
		}
	}
	if len(left) == 0 {
		return ""
	}
	return copilottools.ClosingNote(closingPending, inspection{gaps: left}.gapLines(), closingNever)
}
