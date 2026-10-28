package sitetools

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"unicode/utf8"

	"vozko/domain/site"
	"vozko/domain/sitehtml"
	"vozko/domain/strictjson"
	"vozko/usecases/siterender"
)

const (
	maxCapturedTexts    = 300
	maxCapturedImages   = 150
	maxCapturedOverflow = 3
	maxCapturedErrors   = 20
	maxCapturedRunes    = 500
	maxCapturedPage     = 1_000_000
	maxCapturedSlices   = 64
	maxCapturedPixels   = 100_000
	maxCapturedFontPx   = 1000
	maxCapturedWeight   = 1000
	maxCapturedStatus   = 999
	maxCapturedBytes    = 1 << 40
	maxCapturedMetric   = 600_000
)

var (
	errLookUnreadable = errors.New("sitetools: the look reply is unreadable")
	markTag           = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
)

type lookViewportReport struct {
	W         int  `json:"w"`
	PageH     int  `json:"page_h"`
	Slices    int  `json:"slices"`
	Truncated bool `json:"truncated"`
}

type lookMark struct {
	N    int    `json:"n"`
	Node string `json:"node"`
	Tag  string `json:"tag"`
}

type capturedText struct {
	Node   string  `json:"node"`
	FG     [3]int  `json:"fg"`
	BG     [3]int  `json:"bg"`
	Px     float64 `json:"px"`
	Weight int     `json:"weight"`
}

type capturedImage struct {
	Node     string `json:"node"`
	NaturalW int    `json:"naturalW"`
	NaturalH int    `json:"naturalH"`
	ShownW   int    `json:"shownW"`
	ShownH   int    `json:"shownH"`
}

type capturedMetrics struct {
	LCP float64 `json:"lcp"`
	CLS float64 `json:"cls"`
	TBT float64 `json:"tbt"`
}

type capturedRequest struct {
	URL    string `json:"url"`
	Status int    `json:"status"`
}

type capturedFacts struct {
	ScrollWidth    int               `json:"scrollWidth"`
	Overflow       []string          `json:"overflow"`
	Texts          []capturedText    `json:"texts"`
	Images         []capturedImage   `json:"images"`
	Metrics        capturedMetrics   `json:"metrics"`
	ConsoleErrors  []string          `json:"consoleErrors"`
	FailedRequests []capturedRequest `json:"failedRequests"`
	TransferBytes  int64             `json:"transferBytes"`
}

type viewportFacts struct {
	W     int           `json:"w"`
	Facts capturedFacts `json:"facts"`
}

type lookReply struct {
	Viewports []lookViewportReport `json:"viewports"`
	Marks     []lookMark           `json:"marks,omitempty"`
	Facts     []viewportFacts      `json:"facts"`
}

func decodeLook(data []byte, order lookOrder) (lookReply, error) {
	var got lookReply
	if strictjson.Decode(data, &got) != nil {
		return lookReply{}, errLookUnreadable
	}
	if err := got.valid(order); err != nil {
		return lookReply{}, err
	}
	return got, nil
}

func within(value, low, high int) bool {
	return value >= low && value <= high
}

func withinFloat(value, low, high float64) bool {
	return value >= low && value <= high
}

func validNode(node string) bool {
	_, _, err := sitehtml.ParseNodeID(node)
	return err == nil
}

func optionalNode(node string) bool {
	return node == "" || validNode(node)
}

func plainRunes(value string, limit int) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= limit
}

func (r lookReply) valid(order lookOrder) error {
	if len(r.Viewports) != len(order.Viewports) || len(r.Facts) != len(order.Viewports) {
		return fmt.Errorf("%w: %d viewports for %d asked", errLookUnreadable, len(r.Viewports), len(order.Viewports))
	}
	for i, asked := range order.Viewports {
		v := r.Viewports[i]
		if v.W != asked.W || r.Facts[i].W != asked.W || !within(v.PageH, 0, maxCapturedPage) || !within(v.Slices, 0, maxCapturedSlices) {
			return fmt.Errorf("%w: viewport %d", errLookUnreadable, i)
		}
		if err := r.Facts[i].Facts.valid(); err != nil {
			return err
		}
	}
	limit := 0
	if order.Marks != nil {
		limit = order.Marks.Limit
	}
	if len(r.Marks) > limit {
		return fmt.Errorf("%w: %d marks", errLookUnreadable, len(r.Marks))
	}
	for _, m := range r.Marks {
		if !within(m.N, 1, limit) || !validNode(m.Node) || !markTag.MatchString(m.Tag) {
			return fmt.Errorf("%w: mark %d", errLookUnreadable, m.N)
		}
	}
	return nil
}

func validColor(rgb [3]int) bool {
	for _, c := range rgb {
		if !within(c, 0, 255) {
			return false
		}
	}
	return true
}

func (f capturedFacts) valid() error {
	if !within(f.ScrollWidth, 0, maxCapturedPixels) || len(f.Overflow) > maxCapturedOverflow || len(f.Texts) > maxCapturedTexts || len(f.Images) > maxCapturedImages ||
		len(f.ConsoleErrors) > maxCapturedErrors || len(f.FailedRequests) > maxCapturedErrors || f.TransferBytes < 0 || f.TransferBytes > maxCapturedBytes {
		return fmt.Errorf("%w: facts out of bounds", errLookUnreadable)
	}
	if !withinFloat(f.Metrics.LCP, 0, maxCapturedMetric) || !withinFloat(f.Metrics.CLS, 0, maxCapturedMetric) || !withinFloat(f.Metrics.TBT, 0, maxCapturedMetric) {
		return fmt.Errorf("%w: metrics out of bounds", errLookUnreadable)
	}
	for _, node := range f.Overflow {
		if !validNode(node) {
			return fmt.Errorf("%w: overflow node", errLookUnreadable)
		}
	}
	for _, text := range f.Texts {
		if !optionalNode(text.Node) || !validColor(text.FG) || !validColor(text.BG) || !withinFloat(text.Px, 0, maxCapturedFontPx) || !within(text.Weight, 0, maxCapturedWeight) {
			return fmt.Errorf("%w: text sample", errLookUnreadable)
		}
	}
	for _, img := range f.Images {
		if !optionalNode(img.Node) || !within(img.NaturalW, 0, maxCapturedPixels) || !within(img.NaturalH, 0, maxCapturedPixels) || !within(img.ShownW, 0, maxCapturedPixels) || !within(img.ShownH, 0, maxCapturedPixels) {
			return fmt.Errorf("%w: image sample", errLookUnreadable)
		}
	}
	for _, message := range f.ConsoleErrors {
		if !plainRunes(message, maxCapturedRunes) {
			return fmt.Errorf("%w: console error", errLookUnreadable)
		}
	}
	for _, failed := range f.FailedRequests {
		if !plainRunes(failed.URL, maxCapturedRunes) || !within(failed.Status, 0, maxCapturedStatus) {
			return fmt.Errorf("%w: failed request", errLookUnreadable)
		}
	}
	return nil
}

func (f capturedFacts) rendererFacts() siterender.Facts {
	out := siterender.Facts{
		ScrollWidth:   f.ScrollWidth,
		Overflow:      f.Overflow,
		Metrics:       siterender.Metrics{LCP: f.Metrics.LCP, CLS: f.Metrics.CLS, TBT: f.Metrics.TBT},
		ConsoleErrors: f.ConsoleErrors,
		TransferBytes: f.TransferBytes,
	}
	for _, text := range f.Texts {
		out.Texts = append(out.Texts, siterender.TextSample{Node: text.Node, FG: text.FG, BG: text.BG, Px: text.Px, Weight: text.Weight})
	}
	for _, img := range f.Images {
		out.Images = append(out.Images, siterender.ImageSample{Node: img.Node, NaturalW: img.NaturalW, NaturalH: img.NaturalH, ShownW: img.ShownW, ShownH: img.ShownH})
	}
	for _, failed := range f.FailedRequests {
		out.FailedRequests = append(out.FailedRequests, siterender.FailedRequest{URL: failed.URL, Status: failed.Status})
	}
	return out
}

func (r lookReply) checks(route string, viewports []lookViewport) ([]site.Check, int) {
	var all []site.Check
	for i, captured := range r.Facts {
		viewport := siterender.Viewport{Width: viewports[i].W, Height: viewports[i].H}
		all = append(all, siterender.FactChecks(route, viewport, captured.Facts.rendererFacts())...)
	}
	return siterender.Condense(all)
}

func (r lookReply) viewportLines() []string {
	lines := make([]string, 0, len(r.Viewports))
	for _, v := range r.Viewports {
		line := fmt.Sprintf("%d px: página de %d px em %d faixas", v.W, v.PageH, v.Slices)
		if v.Truncated {
			line += ", cortada no fim"
		}
		lines = append(lines, line)
	}
	return lines
}

func (r lookReply) markLines() []string {
	lines := make([]string, 0, len(r.Marks))
	for _, m := range r.Marks {
		lines = append(lines, strconv.Itoa(m.N)+" "+m.Node+" "+m.Tag)
	}
	return lines
}
