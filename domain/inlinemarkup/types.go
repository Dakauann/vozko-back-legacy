package inlinemarkup

type Paragraph struct {
	Runs []Run `json:"runs"`
}

type Run struct {
	Text      string `json:"text"`
	Bold      bool   `json:"bold,omitempty"`
	Italic    bool   `json:"italic,omitempty"`
	Underline bool   `json:"underline,omitempty"`
	Link      *Link  `json:"link,omitempty"`
}

type Link struct {
	Href string `json:"href"`
}

type Error struct {
	Reason string
}

func (e *Error) Error() string {
	return ErrInvalid.Error() + ": " + e.Reason
}

func (e *Error) Is(target error) bool {
	return target == ErrInvalid
}

func invalid(reason string) error {
	return &Error{Reason: reason}
}
