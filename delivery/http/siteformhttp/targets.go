package siteformhttp

import (
	"context"
	"net/http"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/siteform"
	"vozko/usecases/siteforms"
)

const formTargetsList = "/sites/forms/targets"

type FormTargetLister interface {
	FormTargets(ctx context.Context, workspaceID string) (siteforms.FormTargets, error)
}

type FormTargetResponse struct {
	Target string          `json:"target" example:"lead.phone"`
	Label  string          `json:"label,omitempty" example:"Cidade"`
	Custom bool            `json:"custom"`
	Kinds  []siteform.Kind `json:"kinds" swaggertype:"array,string" example:"tel,text"`
}

type FormTargetsResponse struct {
	Targets    []FormTargetResponse `json:"targets"`
	Settable   []siteform.Kind      `json:"settable" swaggertype:"array,string" example:"text,email,tel"`
	Insertable []siteform.Kind      `json:"insertable" swaggertype:"array,string" example:"text,email,tel,textarea"`
}

// @Summary		Campos do lead que um formulário de página preenche
// @Description	O que cada campo de um formulário de página pode preencher no lead: nome, WhatsApp, e-mail, consentimento, aceite de e-mails e os campos personalizados do lead deste workspace (os sensíveis ficam de fora), com os tipos de campo que cada um aceita, o primeiro sendo o sugerido. `settable` são os tipos que set_form_field pode gravar num campo existente e `insertable` os que insert_form_field cria. As operações recusam o resto (form_field_unknown, form_field_type_invalid). Exige `sites:read`.
// @Tags			Páginas
// @Produce		json
// @Success		200	{object}	FormTargetsResponse
// @Failure		403	{object}	response.ErrorResponse
// @Failure		500	{object}	response.ErrorResponse	"internal_error"
// @Security		BearerAuth
// @Router			/sites/forms/targets [get]
func (h *Handler) FormTargets(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	targets, err := h.forms.FormTargets(r.Context(), workspaceID)
	if err != nil {
		writeError(w, err)
		return
	}
	out := FormTargetsResponse{Targets: make([]FormTargetResponse, 0, len(targets.Targets)), Settable: targets.Settable, Insertable: targets.Insertable}
	for _, target := range targets.Targets {
		out.Targets = append(out.Targets, FormTargetResponse{Target: target.Target, Label: target.Label, Custom: target.Custom, Kinds: target.Kinds})
	}
	response.WriteSuccess(w, http.StatusOK, out)
}
