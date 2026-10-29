package sitenext

import (
	"regexp"
	"strings"

	"vozko/domain/site"
	"vozko/domain/sitepublish"
)

const MaxSteps = 3

type ActionKind string

const (
	ActionPrompt   ActionKind = "prompt"
	ActionSettings ActionKind = "settings"
	ActionPublish  ActionKind = "publish"
)

type Tab string

const (
	TabGeneral      Tab = "general"
	TabDomains      Tab = "domains"
	TabForms        Tab = "forms"
	TabBookings     Tab = "bookings"
	TabIntegrations Tab = "integrations"
	TabConsent      Tab = "consent"
)

type Action struct {
	Kind   ActionKind
	Prompt string
	Tab    Tab
}

type Step struct {
	Code   string
	Label  string
	Action Action
}

type Stage string

const (
	StageDraft           Stage = "draft"
	StageLive            Stage = "live"
	StageLiveWithChanges Stage = "live_with_changes"
	StageClosed          Stage = "closed"
)

func (s Stage) Live() bool {
	return s == StageLive || s == StageLiveWithChanges
}

type Source struct {
	MentionsBooking bool
	HasBooking      bool
	HasWebchat      bool
}

type Facts struct {
	Gaps         []site.Issue
	Forms        []string
	Settings     site.Settings
	Source       Source
	Stage        Stage
	CustomDomain bool
}

const (
	CodeFormOwner         = "form_owner"
	CodeFormFollowUpField = "form_follow_up_field"
	CodeBookingCalendar   = "booking_calendar"
	CodeBookingSettings   = "booking_settings"
	CodePrivacyLink       = "privacy_link"
	CodeFormConsent       = "form_consent"
	CodeFormIdentity      = "form_identity"
	CodeConsentBanner     = "consent_banner"
	CodeEmptyLinks        = "empty_links"
	CodeImageAlt          = "image_alt"
	CodeVisibleWithoutJS  = "visible_without_js"
	CodeFormDestination   = "form_destination"
	CodeAddBooking        = "add_booking"
	CodePublish           = "publish"
	CodePublishChanges    = "publish_changes"
	CodeSiteChat          = "site_chat"
	CodeFavicon           = "favicon"
	CodeCustomDomain      = "custom_domain"
)

func settings(code, label string, tab Tab) Step {
	return Step{Code: code, Label: label, Action: Action{Kind: ActionSettings, Tab: tab}}
}

func prompt(code, label, text string) Step {
	return Step{Code: code, Label: label, Action: Action{Kind: ActionPrompt, Prompt: text}}
}

func publish(code, label string) Step {
	return Step{Code: code, Label: label, Action: Action{Kind: ActionPublish}}
}

var catalog = []Step{
	settings(CodeFormOwner, "Escolher quem recebe os leads", TabForms),
	settings(CodeFormFollowUpField, "Ajustar o retorno do formulário", TabForms),
	settings(CodeBookingCalendar, "Conectar o Google Agenda", TabBookings),
	settings(CodeBookingSettings, "Configurar o agendamento", TabBookings),
	prompt(CodePrivacyLink, "Adicionar a política de privacidade", "Adicione perto do formulário o link da política de privacidade. Se eu ainda não te passei o endereço, me pergunte."),
	prompt(CodeFormConsent, "Pedir consentimento no formulário", "Adicione ao formulário a caixa de consentimento que falta para publicar."),
	prompt(CodeFormIdentity, "Pedir nome e contato no formulário", "Faça o formulário pedir o nome e um contato (telefone ou e-mail) para o lead chegar completo."),
	settings(CodeConsentBanner, "Ativar o aviso de cookies", TabConsent),
	prompt(CodeEmptyLinks, "Preencher os links vazios", "Preencha os links e botões que ainda estão vazios. Se faltar algum endereço, me pergunte numa só pergunta."),
	prompt(CodeImageAlt, "Descrever as imagens", "Escreva textos alternativos curtos e descritivos para as imagens que ainda não têm."),
	prompt(CodeVisibleWithoutJS, "Mostrar o conteúdo sem animação", "Algumas partes do site só aparecem com JavaScript. Ajuste para que o conteúdo fique visível mesmo sem animação."),
	settings(CodeFormDestination, "Escolher para onde vão os leads", TabForms),
	prompt(CodeAddBooking, "Adicionar um bloco de agendamento", "O site fala de agendamento: adicione um bloco de agendamento na seção certa para o visitante marcar o horário direto na página."),
	publish(CodePublish, "Publicar o site"),
	publish(CodePublishChanges, "Publicar as alterações"),
	settings(CodeSiteChat, "Adicionar o chat do site", TabIntegrations),
	settings(CodeFavicon, "Escolher o ícone da aba", TabGeneral),
	settings(CodeCustomDomain, "Conectar um domínio próprio", TabDomains),
}

var gapSteps = map[string]string{
	site.CodeFormDealOwnerRequired:       CodeFormOwner,
	site.CodeFormFollowUpFieldMissing:    CodeFormFollowUpField,
	site.CodeBookingCalendarDisconnected: CodeBookingCalendar,
	site.CodeBookingUnconfigured:         CodeBookingSettings,
	site.CodeBookingIDTaken:              CodeBookingSettings,
	site.CodePrivacyLinkMissing:          CodePrivacyLink,
	site.CodeConsentRequired:             CodeFormConsent,
	site.CodeFormIdentityMissing:         CodeFormIdentity,
	site.CodeConsentBannerMissing:        CodeConsentBanner,
	site.CodeLinkEmpty:                   CodeEmptyLinks,
	site.CodeAltMissing:                  CodeImageAlt,
	site.CodeInvisibleWithoutJS:          CodeVisibleWithoutJS,
}

func Catalog() []Step {
	return append([]Step(nil), catalog...)
}

func Next(f Facts) []Step {
	due := map[string]bool{}
	for _, gap := range f.Gaps {
		if code, ok := gapSteps[gap.Code]; ok {
			due[code] = true
		}
	}
	due[CodeFormDestination] = formWithoutDestination(f)
	due[CodeAddBooking] = f.Source.MentionsBooking && !f.Source.HasBooking && len(f.Settings.Bookings) == 0
	due[CodePublish] = len(f.Gaps) == 0 && f.Stage == StageDraft
	due[CodePublishChanges] = len(f.Gaps) == 0 && f.Stage == StageLiveWithChanges
	due[CodeSiteChat] = !f.Source.HasWebchat && f.Settings.Site.WebchatWidget == ""
	due[CodeFavicon] = f.Settings.Site.FaviconMediaID == ""
	due[CodeCustomDomain] = f.Stage.Live() && !f.CustomDomain
	steps := []Step{}
	for _, step := range catalog {
		if due[step.Code] && len(steps) < MaxSteps {
			steps = append(steps, step)
		}
	}
	return steps
}

func formWithoutDestination(f Facts) bool {
	for _, id := range f.Forms {
		form, ok := f.Settings.Forms[id]
		if !ok || (form.FunnelID == "" && form.WorkflowID == "") {
			return true
		}
	}
	return false
}

var bookingWord = regexp.MustCompile(`(?i)(?:^|[^\p{L}])agend(?:a|ar|e|em|amento|amentos)(?:[^\p{L}]|$)`)

func ScanSource(p site.Project) Source {
	var out Source
	for _, path := range p.Paths() {
		content, _ := p.Content(path)
		out.MentionsBooking = out.MentionsBooking || bookingWord.MatchString(content)
		out.HasBooking = out.HasBooking || strings.Contains(content, "<"+site.BookingTag)
		out.HasWebchat = out.HasWebchat || strings.Contains(content, "<"+site.WebchatTag)
	}
	return out
}

func HasCustomDomain(recs []sitepublish.HostnameRecord) bool {
	for _, rec := range recs {
		if rec.Kind == sitepublish.HostnameCustom && rec.Status != sitepublish.HostnameRemoving && rec.Status != sitepublish.HostnameFailed {
			return true
		}
	}
	return false
}
