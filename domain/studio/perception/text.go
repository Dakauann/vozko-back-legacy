package perception

import (
	"fmt"
	"math"
	"strings"

	"vozko/domain/studio"
)

type MeasuredWord struct {
	Text  string  `json:"text"`
	Width float64 `json:"width"`
}

type TextFacts struct {
	Paragraphs [][]string   `json:"paragraphs"`
	WidestWord MeasuredWord `json:"widestWord"`
	WidestLine float64      `json:"widestLine"`
	LineHeight float64      `json:"lineHeight"`
	Visible    int          `json:"visible"`
	Fitting    float64      `json:"fitting"`
}

var (
	fitSlack       = 1.08
	hiddenWordsMax = 40
	widthScale     = 1000.0
)

func (f *TextFacts) lines() []string {
	var all []string
	for _, paragraph := range f.Paragraphs {
		all = append(all, paragraph...)
	}
	return all
}

func inkCorners(canvas Canvas, el Element) []Point {
	facts := el.Text
	t := el.Transform
	boxWidth := t.W * canvas.Width
	width := math.Min(boxWidth, facts.WidestLine)
	shift := 0.0
	switch textAnchor(el.Layer) {
	case EdgeLeft:
		shift = (width - boxWidth) / 2
	case EdgeRight:
		shift = (boxWidth - width) / 2
	}
	radians := radiansOf(t.Rotation)
	placed := t
	placed.X = (t.X*canvas.Width + shift*math.Cos(radians)) / canvas.Width
	placed.Y = (t.Y*canvas.Height + shift*math.Sin(radians)) / canvas.Height
	placed.W = width / canvas.Width
	placed.H = float64(facts.Visible) * facts.LineHeight / canvas.Height
	return boxCorners(placed, canvas)
}

func inkBox(canvas Canvas, el Element) Box {
	return unitBounds(inkCorners(canvas, el), canvas)
}

type clipped struct {
	level Level
	key   string
	text  string
}

func fitAdvice(fitting float64) string {
	if fitting > 0 {
		return fmt.Sprintf("use font_size %s (o maior que mostra tudo nesta caixa)", num(fitting))
	}
	return "diminua font_size"
}

func hiddenLinesText(hidden []string, facts *TextFacts) string {
	words := jsCollapse(strings.Join(hidden, " "))
	quoted := words
	if jsLength(words) > hiddenWordsMax {
		quoted = jsHead(words, hiddenWordsMax) + "…"
	}
	count := "a linha"
	if len(hidden) != 1 {
		count = fmt.Sprintf("%d linhas", len(hidden))
	}
	return fmt.Sprintf("esconde %s \"%s\": a caixa só mostra %d de %d linhas e o resto some da arte; aumente h, alargue w, %s ou encurte o texto", count, quoted, facts.Visible, len(facts.lines()), fitAdvice(facts.Fitting))
}

func wideWordText(word MeasuredWord, boxPx float64, canvas Canvas, fitting float64) string {
	width := math.Ceil(word.Width/canvas.Width*widthScale) / widthScale
	return fmt.Sprintf("tem a palavra \"%s\" mais larga que a caixa (%s px numa caixa de %s px), e a arte parte a palavra no meio; %s ou alargue w para pelo menos %s", word.Text, pxNum(word.Width), pxNum(boxPx), fitAdvice(fitting), num(width))
}

func clippedFinding(canvas Canvas, el Element, certain bool) *clipped {
	facts := el.Text
	all := facts.lines()
	var hidden []string
	if facts.Visible < len(all) {
		for _, line := range all[facts.Visible:] {
			if jsTrim(line) != "" {
				hidden = append(hidden, line)
			}
		}
	}
	boxPx := el.Transform.W * canvas.Width
	wide := facts.WidestWord.Width > boxPx
	if !wide && len(hidden) == 0 {
		return nil
	}
	if !wide {
		level := LevelWarning
		if certain {
			level = LevelError
		}
		return &clipped{level: level, key: "fit", text: hiddenLinesText(hidden, facts)}
	}
	level := LevelWarning
	if certain || facts.WidestWord.Width > boxPx*fitSlack {
		level = LevelError
	}
	return &clipped{level: level, key: "word", text: wideWordText(facts.WidestWord, boxPx, canvas, facts.Fitting)}
}

func snippet(layer *studio.Layer) string {
	words := jsTrim(jsCollapse(layer.Text))
	if jsLength(words) > 24 {
		return jsHead(words, 24) + "…"
	}
	return words
}
