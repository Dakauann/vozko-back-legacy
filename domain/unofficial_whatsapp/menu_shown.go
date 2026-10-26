package unofficial_whatsapp

import "vozko/domain/conversation"

type ShownMenu struct {
	Style   string
	Request conversation.SendInteractiveRequest
	Options []conversation.InteractiveOption
}

func ShowMenu(req conversation.SendInteractiveRequest) ShownMenu {
	style, fits := FitMenu(req.Style, len(req.Options))
	showing := req.WithHeaderInBody()
	showing.Body, showing.Footer = SanitizeOutboundText(showing.Body), SanitizeOutboundText(showing.Footer)
	return ShownMenu{Style: style, Request: showing, Options: req.Options[:fits]}
}

func (m ShownMenu) Content() conversation.InteractiveContent {
	return m.Request.ShowingAs(m.Style, m.Options)
}

func (m ShownMenu) ProviderOptions() []InteractiveOption {
	options := make([]InteractiveOption, 0, len(m.Options))
	for _, opt := range m.Options {
		options = append(options, InteractiveOption{ID: opt.ID, Title: opt.Title, Description: opt.Description, Section: opt.Section})
	}
	return options
}

func ShownInteractive(req conversation.SendInteractiveRequest) conversation.InteractiveContent {
	return ShowMenu(req).Content()
}
