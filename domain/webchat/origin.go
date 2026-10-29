package webchat

import (
	"strings"

	"vozko/domain/publicform"
)

func (w *Widget) AcceptSiteOrigins(origins []string) {
	w.siteOrigins = append([]string(nil), origins...)
}

func (w *Widget) origins() publicform.OriginAllowList {
	return publicform.AllowOriginPatterns(w.AllowedOrigins).With(publicform.AllowExactOrigins(w.siteOrigins))
}

func PreviewFrameAncestors(preview string, editors []string) string {
	return frameAncestors(publicform.AllowExactOrigins(append([]string{preview}, editors...)))
}

func frameAncestors(origins publicform.OriginAllowList) string {
	if origins.Empty() {
		return "'none'"
	}
	return strings.Join(origins.Origins(), " ")
}
