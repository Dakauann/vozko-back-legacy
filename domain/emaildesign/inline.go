package emaildesign

import (
	"errors"
	"fmt"

	"vozko/domain/inlinemarkup"
)

var ErrInlineMarkup = errors.New("emaildesign: invalid inline markup")

func ParseInline(markup string) ([]Paragraph, error) {
	parsed, err := inlinemarkup.Parse(markup)
	if err != nil {
		var invalid *inlinemarkup.Error
		if errors.As(err, &invalid) {
			return nil, fmt.Errorf("%w: %s", ErrInlineMarkup, invalid.Reason)
		}
		return nil, err
	}
	out := make([]Paragraph, 0, len(parsed))
	for _, p := range parsed {
		runs := make([]Run, 0, len(p.Runs))
		for _, r := range p.Runs {
			run := Run{Text: r.Text, Bold: r.Bold, Italic: r.Italic, Underline: r.Underline}
			if r.Link != nil {
				run.Link = &Link{Href: r.Link.Href}
			}
			runs = append(runs, run)
		}
		out = append(out, Paragraph{Runs: runs})
	}
	return out, nil
}

func FormatInline(paragraphs []Paragraph) string {
	shared := make([]inlinemarkup.Paragraph, 0, len(paragraphs))
	for _, p := range paragraphs {
		runs := make([]inlinemarkup.Run, 0, len(p.Runs))
		for _, r := range p.Runs {
			run := inlinemarkup.Run{Text: r.Text, Bold: r.Bold, Italic: r.Italic, Underline: r.Underline}
			if r.Link != nil {
				run.Link = &inlinemarkup.Link{Href: r.Link.Href}
			}
			runs = append(runs, run)
		}
		shared = append(shared, inlinemarkup.Paragraph{Runs: runs})
	}
	return inlinemarkup.Format(shared)
}
