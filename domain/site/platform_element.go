package site

import (
	"html"
	"regexp"
)

type PlatformElement string

const (
	ElementWebchat PlatformElement = "webchat"
	ElementBooking PlatformElement = "booking"

	WebchatTag  = "vz-webchat"
	WebchatAttr = "widget"

	CodeElementUnknown       = "element_unknown"
	CodeElementKeyInvalid    = "element_key_invalid"
	CodeWebchatWidgetUnknown = "webchat_widget_unknown"
	CodeBookingUnknown       = "booking_unknown"
)

var webchatKey = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func ValidWebchatKey(key string) bool {
	return webchatKey.MatchString(key)
}

func PlatformElements() []PlatformElement {
	return []PlatformElement{ElementWebchat, ElementBooking}
}

func (e PlatformElement) Tag() string {
	switch e {
	case ElementWebchat:
		return WebchatTag
	case ElementBooking:
		return BookingTag
	}
	return ""
}

func (e PlatformElement) Attr() string {
	switch e {
	case ElementWebchat:
		return WebchatAttr
	case ElementBooking:
		return BookingAttr
	}
	return ""
}

func (e PlatformElement) ValidKey(key string) bool {
	switch e {
	case ElementWebchat:
		return ValidWebchatKey(key)
	case ElementBooking:
		return formSettingsID.MatchString(key)
	}
	return false
}

func (e PlatformElement) Markup(key string) string {
	return "<" + e.Tag() + " " + e.Attr() + `="` + html.EscapeString(key) + `"></` + e.Tag() + ">"
}

type ElementUse struct {
	Index   int
	Element PlatformElement
	Key     string
}

func (b Batch) PlatformElements() []ElementUse {
	return b.Elements
}

func expandElement(i int, o Operation) ([]Operation, *Failure) {
	if o.Element == "" {
		return nil, failure(i, CodeFieldRequired, "element")
	}
	if o.Element.Tag() == "" {
		return nil, failure(i, CodeElementUnknown, "element")
	}
	if o.Key == "" {
		return nil, failure(i, CodeFieldRequired, "key")
	}
	if !o.Element.ValidKey(o.Key) {
		return nil, failure(i, CodeElementKeyInvalid, "key")
	}
	return []Operation{{Op: OpInsertSection, At: o.At, Rev: o.Rev, Ref: o.Ref, HTML: o.Element.Markup(o.Key)}}, nil
}
