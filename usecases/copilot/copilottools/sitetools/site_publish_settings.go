package sitetools

import (
	"context"
	"fmt"
	"strings"

	"vozko/domain/copilot"
	"vozko/domain/sitepublish"
)

const (
	fieldSettings          = "settings"
	publishSettingsSame    = "nenhuma configuração muda"
	publishSettingsUnknown = "não foi possível comparar as configurações com a publicação no ar"
)

func redirectLine(c sitepublish.RedirectChange) string {
	switch c.Action {
	case sitepublish.ChangeAdded:
		return fmt.Sprintf("redirecionamento novo %s para %s (%d)", c.From, c.To, c.Status)
	case sitepublish.ChangeRemoved:
		return fmt.Sprintf("redirecionamento %s removido", c.From)
	}
	return fmt.Sprintf("redirecionamento %s agora vai para %s (antes %s), status %d", c.From, c.To, c.PreviousTo, c.Status)
}

func integrationLine(c sitepublish.IntegrationChange) string {
	switch c.Action {
	case sitepublish.ChangeAdded:
		return fmt.Sprintf("integração %s nova: %s", c.Kind, c.ID)
	case sitepublish.ChangeRemoved:
		return fmt.Sprintf("integração %s removida", c.Kind)
	}
	return fmt.Sprintf("integração %s trocada para %s (antes %s)", c.Kind, c.ID, c.PreviousID)
}

func fieldsLine(subject string, action sitepublish.ChangeAction, fields []string) string {
	switch action {
	case sitepublish.ChangeAdded:
		return subject + " configurado: " + strings.Join(fields, ", ")
	case sitepublish.ChangeRemoved:
		return subject + " sem configuração"
	}
	return subject + " muda " + strings.Join(fields, ", ")
}

func settingsChangeLines(c sitepublish.SettingsChanges) []string {
	var lines []string
	for _, r := range c.Redirects {
		lines = append(lines, redirectLine(r))
	}
	for _, i := range c.Integrations {
		lines = append(lines, integrationLine(i))
	}
	for _, f := range c.Forms {
		lines = append(lines, fieldsLine("formulário "+f.FormID, f.Action, f.Fields))
	}
	if c.Consent != nil {
		lines = append(lines, fieldsLine("aviso de cookies", sitepublish.ChangeChanged, c.Consent.Fields))
	}
	for _, p := range c.Pages {
		lines = append(lines, fieldsLine("SEO de "+p.Route, p.Action, p.Fields))
	}
	if c.Site != nil {
		lines = append(lines, fieldsLine("identidade do site", sitepublish.ChangeChanged, c.Site.Fields))
	}
	return lines
}

func (t *sitePublishTool) settings(ctx context.Context, cc copilot.Context, projectID string) string {
	changes, err := t.deps.Publisher.SettingsChanges(ctx, actorOf(cc), projectID)
	if err != nil {
		return publishSettingsUnknown
	}
	lines := settingsChangeLines(changes)
	if len(lines) == 0 {
		return publishSettingsSame
	}
	return strings.Join(capLines(lines), "; ")
}
