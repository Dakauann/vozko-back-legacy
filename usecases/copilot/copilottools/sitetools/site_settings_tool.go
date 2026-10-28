package sitetools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"vozko/domain/copilot"
	"vozko/domain/shared"
	"vozko/domain/site"
	"vozko/domain/tools"
	"vozko/usecases/copilot/copilottools"
	site_usecase "vozko/usecases/site"
)

const (
	sectionForms       = "forms"
	settingsSections   = "pages,integrations,forms,redirects,consent"
	settingsUnknown    = "section %q não existe; use uma de: pages, integrations, forms, redirects, consent"
	settingsFormOnly   = "form_id só vale com section forms"
	settingsBadJSON    = "value precisa ser JSON válido: o conteúdo inteiro da seção (ou do formulário, com form_id), ou null para limpar"
	settingsInvalid    = "as configurações não foram salvas: %v"
	settingsReference  = "as configurações não foram salvas: no formulário %s, %s"
	settingsMoved      = "as configurações mudaram duas vezes enquanto você editava e não foram salvas; leia de novo"
	settingsFailed     = "as configurações não puderam ser salvas agora; tente de novo"
	settingsActionRead = "ler"
	settingsActionSet  = "mudar"
)

var referenceWords = map[error]string{
	site_usecase.ErrFormFunnelUnknown:   "o funil não existe neste workspace",
	site_usecase.ErrFormStageUnknown:    "a etapa não existe neste funil",
	site_usecase.ErrFormOwnerUnknown:    "o dono não existe neste workspace",
	site_usecase.ErrFormWorkflowUnknown: "o workflow não existe neste workspace",
	site_usecase.ErrFormChannelUnknown:  "o canal de follow_up não existe neste workspace",
	site.ErrFormChannelRequired:         "um formulário com workflow precisa de follow_up",
}

type siteSettingsArgs struct {
	Section string  `json:"section" req:"true" enum:"pages,integrations,forms,redirects,consent" desc:"a seção"`
	FormID  string  `json:"form_id,omitempty" desc:"com forms: só esse formulário (o id do data-vz-form)"`
	Value   *string `json:"value,omitempty" desc:"para mudar: o JSON novo da seção ou do formulário, ou null; sem value, lê"`
}

type siteSettingsTool struct {
	siteScope
	deps Deps
}

func NewSiteSettingsTool(deps Deps) copilot.Tool {
	return &siteSettingsTool{deps: deps}
}

func (t *siteSettingsTool) Meta() copilot.Meta { return editMeta() }

func (t *siteSettingsTool) NeedsApproval(mode copilot.Mode) bool { return mode == copilot.ModeAsk }

var routingSections = []string{"redirects", "integrations"}

func (t *siteSettingsTool) NeedsApprovalFor(_ copilot.Mode, args map[string]interface{}) bool {
	var a siteSettingsArgs
	copilottools.BindArgs(args, &a)
	return a.Value != nil && copilottools.Contains(routingSections, strings.TrimSpace(a.Section))
}

func (t *siteSettingsTool) Definition() tools.Definition {
	return copilottools.Definition("site_settings",
		"Lê ou muda uma seção das configurações da página aberta. pages: por rota, title, description, og_media_id, canonical, noindex. integrations: meta_pixel, ga4, gtm, google_ads, tiktok_pixel, clarity. "+
			"forms: por id do data-vz-form, funnel_id, stage_id, owner {kind unassigned|user|roulette, id}, workflow_id, follow_up {kind whatsapp|unofficial_whatsapp|email, id}, success {kind message|redirect|whatsapp, message, route, channel}, notify. "+
			"redirects: lista de {from, to, status}. consent: banner, text, categories (analytics, marketing). Ids vêm das ferramentas de leitura; nunca invente. Sem value, lê; com value, troca a seção inteira (ou o formulário de form_id). Mudar redirects ou integrations sempre pede aprovação, em qualquer modo.",
		siteSettingsArgs{})
}

type settingsChange struct {
	section string
	formID  string
	value   *json.RawMessage
}

func settingsChangeOf(args map[string]interface{}) (settingsChange, error) {
	var a siteSettingsArgs
	copilottools.BindArgs(args, &a)
	c := settingsChange{section: strings.TrimSpace(a.Section), formID: strings.TrimSpace(a.FormID)}
	if !copilottools.Contains(strings.Split(settingsSections, ","), c.section) {
		return c, fmt.Errorf(settingsUnknown, c.section)
	}
	if c.formID != "" && c.section != sectionForms {
		return c, errors.New(settingsFormOnly)
	}
	if a.Value == nil {
		return c, nil
	}
	raw := json.RawMessage(strings.TrimSpace(*a.Value))
	if !json.Valid(raw) {
		return c, errors.New(settingsBadJSON)
	}
	c.value = &raw
	return c, nil
}

func (t *siteSettingsTool) Validate(ctx context.Context, cc copilot.Context, args map[string]interface{}) error {
	if _, err := settingsChangeOf(args); err != nil {
		return fmt.Errorf("%w: %v", copilottools.ErrInvalidArgs, err)
	}
	if _, err := t.deps.open(ctx, cc, forEditing); err != nil {
		return fmt.Errorf("%w: %v", copilottools.ErrInvalidArgs, err)
	}
	return nil
}

func (t *siteSettingsTool) Describe(_ context.Context, _ copilot.Context, args map[string]interface{}) []copilot.Field {
	c, _ := settingsChangeOf(args)
	action := settingsActionRead
	if c.value != nil {
		action = settingsActionSet
	}
	fields := []copilot.Field{{Key: "section", Value: c.section}, {Key: "action", Value: action}}
	if c.formID != "" {
		fields = append(fields, copilot.Field{Key: "form_id", Value: c.formID})
	}
	if c.value != nil {
		fields = append(fields, copilot.Field{Key: "value", Value: clip(string(*c.value), 400)})
	}
	return fields
}

func sectionOf(settings site.Settings, c settingsChange) (json.RawMessage, error) {
	raw, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}
	var sections map[string]json.RawMessage
	if err := json.Unmarshal(raw, &sections); err != nil {
		return nil, err
	}
	value := sections[c.section]
	if c.formID == "" {
		if value == nil {
			return json.RawMessage("null"), nil
		}
		return value, nil
	}
	form, ok := settings.Forms[c.formID]
	if !ok {
		return json.RawMessage("null"), nil
	}
	return json.Marshal(form)
}

func applyChange(settings site.Settings, c settingsChange) ([]byte, error) {
	raw, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}
	var sections map[string]json.RawMessage
	if err := json.Unmarshal(raw, &sections); err != nil {
		return nil, err
	}
	null := bytes.Equal(bytes.TrimSpace(*c.value), []byte("null"))
	switch {
	case c.formID != "":
		forms := map[string]json.RawMessage{}
		if existing := sections[sectionForms]; existing != nil {
			if err := json.Unmarshal(existing, &forms); err != nil {
				return nil, err
			}
		}
		if null {
			delete(forms, c.formID)
		} else {
			forms[c.formID] = *c.value
		}
		encoded, err := json.Marshal(forms)
		if err != nil {
			return nil, err
		}
		sections[sectionForms] = encoded
		if len(forms) == 0 {
			delete(sections, sectionForms)
		}
	case null:
		delete(sections, c.section)
	default:
		sections[c.section] = *c.value
	}
	return json.Marshal(sections)
}

func settingsFailure(err error) string {
	var ref *site_usecase.FormReferenceError
	if errors.As(err, &ref) {
		for known, words := range referenceWords {
			if errors.Is(ref.Err, known) {
				return fmt.Sprintf(settingsReference, ref.Form, words)
			}
		}
	}
	switch {
	case errors.Is(err, site.ErrSettingsInvalid):
		return fmt.Sprintf(settingsInvalid, err)
	case errors.Is(err, site_usecase.ErrProjectArchived):
		return siteArchived
	case errors.Is(err, site_usecase.ErrProjectSuspended):
		return siteSuspended
	}
	log.Printf("[copilot] site_settings could not save: %v", err)
	return settingsFailed
}

func (t *siteSettingsTool) Execute(ctx context.Context, cc copilot.Context, args map[string]interface{}) copilot.Result {
	c, err := settingsChangeOf(args)
	if err != nil {
		return failure(err.Error())
	}
	purpose := forReading
	if c.value != nil {
		purpose = forEditing
	}
	view, err := t.deps.open(ctx, cc, purpose)
	if err != nil {
		return failure(err.Error())
	}
	current, err := t.deps.Storage.GetSettings(ctx, cc.WorkspaceID, view.Project.ID)
	if err != nil || current == nil {
		return failure(siteUnavailable)
	}
	if c.value == nil {
		value, err := sectionOf(current.Settings, c)
		if err != nil {
			return failure(siteUnavailable)
		}
		return copilot.Result{Status: copilot.StatusOK, Data: asPageData(map[string]interface{}{"section": c.section, "form_id": c.formID, "value": value})}
	}
	saved, err := t.save(ctx, cc, view.Project.ID, current, c)
	if err != nil {
		return failure(settingsFailure(err))
	}
	value, err := sectionOf(saved.Settings, c)
	if err != nil {
		return failure(siteUnavailable)
	}
	return copilot.Result{Status: copilot.StatusOK, Data: asPageData(map[string]interface{}{"saved": true, "section": c.section, "form_id": c.formID, "value": value})}
}

func (t *siteSettingsTool) save(ctx context.Context, cc copilot.Context, projectID string, current *site_usecase.StoredSettings, c settingsChange) (*site_usecase.StoredSettings, error) {
	for attempt := 0; ; attempt++ {
		raw, err := applyChange(current.Settings, c)
		if err != nil {
			return nil, err
		}
		saved, err := t.deps.Storage.PutSettings(ctx, cc.WorkspaceID, projectID, current.Version, raw)
		if !errors.Is(err, shared.ErrVersionConflict) {
			return saved, err
		}
		if attempt > 0 {
			return nil, errors.New(settingsMoved)
		}
		current, err = t.deps.Storage.GetSettings(ctx, cc.WorkspaceID, projectID)
		if err != nil || current == nil {
			return nil, errors.New(settingsFailed)
		}
	}
}
