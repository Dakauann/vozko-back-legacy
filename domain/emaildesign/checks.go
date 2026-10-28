package emaildesign

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"vozko/domain/wcag"
)

const (
	NormalTextContrast        = wcag.NormalTextMinimum
	LargeTextContrast         = wcag.LargeTextMinimum
	RecommendedMaxWidth       = 640
	RecommendedButtonRunes    = 30
	RecommendedParagraphRunes = 500
	RecommendedMaxActions     = 3
)

type Check struct {
	Code    string  `json:"code"`
	BlockID string  `json:"block_id,omitempty"`
	Ratio   float64 `json:"ratio,omitempty"`
	Minimum float64 `json:"minimum,omitempty"`
}

type DesignReview struct {
	Hints  []Hint  `json:"hints"`
	Checks []Check `json:"checks"`
}

func Review(doc Document, subject string) DesignReview {
	review := DesignReview{Hints: Lint(doc, subject), Checks: Checks(doc)}
	if review.Hints == nil {
		review.Hints = []Hint{}
	}
	if review.Checks == nil {
		review.Checks = []Check{}
	}
	return review
}

func luminance(color string) (float64, bool) {
	if !validColor(color) {
		return 0, false
	}
	hex := normalizeColor(color)
	parts := [3]uint8{}
	for i := range parts {
		value, err := strconv.ParseUint(hex[1+2*i:3+2*i], 16, 8)
		if err != nil {
			return 0, false
		}
		parts[i] = uint8(value)
	}
	return wcag.RelativeLuminance(parts[0], parts[1], parts[2]), true
}

func ContrastRatio(foreground, background string) float64 {
	a, okA := luminance(foreground)
	b, okB := luminance(background)
	if !okA || !okB {
		return 0
	}
	return wcag.Ratio(a, b)
}

type checker struct {
	settings  Settings
	checks    []Check
	actions   map[string]bool
	lastLevel int
}

func Checks(doc Document) []Check {
	c := &checker{settings: doc.Settings.resolved(), actions: map[string]bool{}}
	if doc.Settings.Width > RecommendedMaxWidth {
		c.checks = append(c.checks, Check{Code: "width_wide"})
	}
	for _, block := range doc.Blocks {
		c.block(block, c.settings.ContentBackgroundColor)
	}
	if len(c.actions) > RecommendedMaxActions {
		c.checks = append(c.checks, Check{Code: "many_buttons"})
	}
	return c.checks
}

func (c *checker) contrast(blockID, foreground, background string, minimum float64) {
	ratio := ContrastRatio(foreground, background)
	if ratio == 0 || ratio >= minimum {
		return
	}
	c.checks = append(c.checks, Check{Code: "low_contrast", BlockID: blockID, Ratio: math.Floor(ratio*100) / 100, Minimum: minimum})
}

func (c *checker) block(b Block, around string) {
	background := around
	if b.BackgroundColor != "" && validColor(b.BackgroundColor) {
		background = normalizeColor(b.BackgroundColor)
	}
	switch {
	case b.Kind == KindHeading && b.Heading != nil:
		c.heading(b, background)
	case b.Kind == KindText && b.Text != nil:
		c.text(b, background)
	case b.Kind == KindButton && b.Button != nil:
		c.button(b)
	case b.Kind == KindSocial && b.Social != nil:
		c.contrast(b.ID, c.settings.LinkColor, background, NormalTextContrast)
	case b.Kind == KindFooter && b.Footer != nil:
		c.contrast(b.ID, c.settings.TextColor, background, NormalTextContrast)
	case b.Kind == KindColumns && b.Columns != nil:
		for _, column := range b.Columns.Columns {
			for _, child := range column.Blocks {
				c.block(child, background)
			}
		}
	}
}

func (c *checker) heading(b Block, background string) {
	minimum := LargeTextContrast
	if b.Heading.Level >= 3 {
		minimum = NormalTextContrast
	}
	c.contrast(b.ID, colorOr(b.Heading.Color, c.settings.TextColor), background, minimum)
	if c.lastLevel > 0 && b.Heading.Level > c.lastLevel+1 {
		c.checks = append(c.checks, Check{Code: "heading_order", BlockID: b.ID})
	}
	c.lastLevel = b.Heading.Level
}

func (c *checker) text(b Block, background string) {
	c.contrast(b.ID, c.settings.TextColor, background, NormalTextContrast)
	linked := false
	for _, p := range b.Text.Paragraphs {
		runes := 0
		for _, run := range p.Runs {
			runes += utf8.RuneCountInString(stripMergeTags(run.Text))
			linked = linked || run.Link != nil
		}
		if runes > RecommendedParagraphRunes {
			c.checks = append(c.checks, Check{Code: "long_paragraph", BlockID: b.ID})
			break
		}
	}
	if linked {
		c.contrast(b.ID, c.settings.LinkColor, background, NormalTextContrast)
	}
}

func (c *checker) button(b Block) {
	c.actions[strings.TrimSpace(b.Button.Href)] = true
	c.contrast(b.ID, colorOr(b.Button.TextColor, c.settings.ButtonTextColor), colorOr(b.Button.Color, c.settings.ButtonColor), NormalTextContrast)
	if utf8.RuneCountInString(stripMergeTags(b.Button.Label)) > RecommendedButtonRunes {
		c.checks = append(c.checks, Check{Code: "button_label_long", BlockID: b.ID})
	}
}
