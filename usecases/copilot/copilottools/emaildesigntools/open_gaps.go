package emaildesigntools

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"vozko/domain/copilot"
	"vozko/domain/emaildesign"
	"vozko/usecases/copilot/copilottools"
)

const (
	watchTimeout     = 4 * time.Second
	resultOpenGaps   = "open_gaps"
	snapshotDesign   = "\n\n# Design aberto\nO usuário está com o design %s aberto no editor de e-mail, ao lado desta conversa (design_id %s).\n"
	snapshotCampaign = "\n\n# Campanha de e-mail aberta\nO usuário está com o conteúdo da campanha %s aberto no editor de e-mail, ao lado desta conversa (campaign_id %s).\n"
	snapshotTools    = "email_read lê esse e-mail como está na tela, sem pedir id; email_edit, email_look e email_generate_image trabalham nele.\n"
	snapshotSilent   = "O editor não respondeu à leitura do início desta resposta; email_read lê o e-mail assim que ele voltar a responder."
	snapshotGaps     = "O que impede salvar e enviar, lido do editor no início desta resposta (block_id, tipo, texto, código):\n"
	snapshotNoGaps   = "Lido do editor no início desta resposta: nada impede salvar e enviar."
	closingPending   = "o e-mail ainda tem lacunas que impedem salvar e enviar"
	closingNever     = "Nunca invente links, nomes ou endereços para fechar uma lacuna."
)

var gapWords = map[string][]string{
	"href_missing":               {"link", "url", "enlace", "destino"},
	"image_src_missing":          {"imagem", "image", "imagen", "foto", "photo", "bild"},
	"text_empty":                 {"texto", "text", "título", "title", "titel"},
	"social_links_empty":         {"rede", "social", "perfil", "profile"},
	"social_url_missing":         {"rede", "social", "perfil", "profile"},
	"footer_company_missing":     {"empresa", "company", "nome", "name", "nombre", "firma"},
	"footer_address_missing":     {"endereço", "address", "dirección", "direccion", "adresse"},
	"footer_missing":             {"rodapé", "footer", "pie", "fußzeile"},
	"unknown_merge_tag":          {"variável", "variable", "merge", "personaliza", "{{"},
	"merge_tag_malformed":        {"variável", "variable", "merge", "personaliza", "{{"},
	"subject_empty":              {"assunto", "subject", "asunto", "betreff"},
	"subject_too_long":           {"assunto", "subject", "asunto", "betreff"},
	"subject_invalid_characters": {"assunto", "subject", "asunto", "betreff"},
}

type gappedResult struct {
	body map[string]interface{}
	gaps []emaildesign.Gap
}

func withOpenGaps(body map[string]interface{}, live liveDesign) gappedResult {
	return gappedResult{body: body, gaps: live.gaps()}
}

func (g gappedResult) MarshalJSON() ([]byte, error) {
	return copilottools.JSONEndingWith(g.body, resultOpenGaps, g.gaps)
}

func gapEntries(gaps []emaildesign.Gap) []string {
	entries := make([]string, 0, len(gaps))
	for _, gap := range gaps {
		var b strings.Builder
		if gap.BlockID == "" {
			b.WriteString("settings")
		} else {
			b.WriteString(gap.BlockID + " (" + string(gap.Kind))
			if gap.Text != "" {
				b.WriteString(` "` + gap.Text + `"`)
			}
			b.WriteString(")")
		}
		b.WriteString(": " + gap.Code)
		if gap.Field != "" {
			b.WriteString(" em " + gap.Field)
		}
		entries = append(entries, b.String())
	}
	return entries
}

func mentioned(reply string, gap emaildesign.Gap) bool {
	return copilottools.ReplyMentions(reply, slices.Concat([]string{gap.Text, gap.Code}, gapWords[gap.Code]))
}

func watching(cc copilot.Context) bool {
	return cc.View.OnEmailDocument() && cc.Screen != nil
}

func (t *emailReadTool) watched(ctx context.Context, cc copilot.Context) (liveDesign, bool) {
	if !watching(cc) {
		return liveDesign{}, false
	}
	ctx, cancel := context.WithTimeout(ctx, watchTimeout)
	defer cancel()
	live, err := readLive(ctx, cc)
	return live, err == nil
}

func openLine(view copilot.View, name string) string {
	shown := strconv.Quote(name)
	if strings.TrimSpace(name) == "" {
		shown = "sem nome"
	}
	if view.OnEmailCampaign() {
		return fmt.Sprintf(snapshotCampaign, shown, view.ProjectID)
	}
	return fmt.Sprintf(snapshotDesign, shown, view.ProjectID)
}

func (t *emailReadTool) ViewSnapshot(ctx context.Context, cc copilot.Context) string {
	if !watching(cc) {
		return ""
	}
	live, ok := t.watched(ctx, cc)
	head := openLine(cc.View, live.Name) + snapshotTools
	if !ok {
		return head + snapshotSilent
	}
	gaps := live.gaps()
	if len(gaps) == 0 {
		return head + snapshotNoGaps
	}
	return head + snapshotGaps + strings.TrimSuffix(copilottools.Bullets(gapEntries(gaps)), "\n")
}

func (t *emailReadTool) ClosingNote(ctx context.Context, cc copilot.Context, reply string) string {
	live, ok := t.watched(ctx, cc)
	if !ok {
		return ""
	}
	gaps := live.gaps()
	var left []emaildesign.Gap
	for _, gap := range gaps {
		if !mentioned(reply, gap) {
			left = append(left, gap)
		}
	}
	if len(left) == 0 {
		return ""
	}
	return copilottools.ClosingNote(closingPending, gapEntries(left), closingNever)
}
