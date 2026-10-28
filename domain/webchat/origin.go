package webchat

import "vozko/domain/publicform"

func (w *Widget) AcceptSiteOrigins(origins []string) {
	w.siteOrigins = append([]string(nil), origins...)
}

func (w *Widget) origins() publicform.OriginAllowList {
	return publicform.AllowOriginPatterns(w.AllowedOrigins).With(publicform.AllowExactOrigins(w.siteOrigins))
}
