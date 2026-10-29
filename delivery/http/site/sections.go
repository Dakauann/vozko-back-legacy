package sitehttp

import (
	"net/http"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/site"
	"vozko/usecases/sitebuild"
)

// @Summary		Biblioteca de seções
// @Description	Peças prontas da biblioteca (kind library), cada uma num grupo: sections (topo, benefícios, depoimentos, perguntas, planos, WhatsApp, galeria, equipe, rodapé), forms (orçamento, contato e agendamento ligados aos campos do CRM), fields (texto, WhatsApp, e-mail, seleção, caixa de marcar), buttons (botão, botão com contorno, link), media (imagem, vídeo) e platform (botão de WhatsApp), seguidas das seções salvas pelo workspace (kind snippet, grupo saved, título igual ao nome nos quatro idiomas), para inserir numa página com a operação insert_section e o campo library_key. Cada seção traz o título em pt, en, es e de e as tecnologias em que pode entrar; stack filtra pelas que cabem numa tecnologia.
// @Tags			Páginas
// @Produce		json
// @Param			stack	query		string	false	"tecnologia da página"	Enums(react, html)
// @Success		200		{object}	SectionsResponse
// @Failure		400		{object}	response.ErrorResponse	"section_stack_invalid"
// @Failure		503		{object}	response.ErrorResponse	"sections_unavailable"
// @Security		BearerAuth
// @Router			/sites/sections [get]
func (h *Handler) Sections(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	list, err := h.sections.List(r.Context(), workspaceID, site.Stack(r.URL.Query().Get("stack")))
	if err != nil {
		writeError(w, err, "Failed to list the sections")
		return
	}
	out := SectionsResponse{Sections: make([]SectionResponse, 0, len(list))}
	for _, s := range list {
		out.Sections = append(out.Sections, presentSection(s))
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

func presentSection(s sitebuild.SectionInfo) SectionResponse {
	stacks := make([]string, 0, len(s.Stacks))
	for _, stack := range s.Stacks {
		stacks = append(stacks, string(stack))
	}
	return SectionResponse{
		Key: s.Key, Kind: string(s.Kind), Group: string(s.Group), Stacks: stacks,
		Title: SectionTitleResponse{PT: s.Title.PT, EN: s.Title.EN, ES: s.Title.ES, DE: s.Title.DE},
	}
}
