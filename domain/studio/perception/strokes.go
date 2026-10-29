package perception

import (
	"math"

	"vozko/domain/studio"
)

type strokeKind string

const (
	strokeLine   strokeKind = "line"
	strokeArrow  strokeKind = "arrow"
	strokeBar    strokeKind = "bar"
	strokeBorder strokeKind = "border"
)

var strokeNouns = map[strokeKind]string{strokeLine: "a linha", strokeArrow: "a seta", strokeBar: "a barra", strokeBorder: "a borda de"}

var (
	inkInset        = 0.2
	crossShare      = 0.5
	thinShare       = 0.012
	longRatio       = 6.0
	borderShareLow  = 0.05
	borderShareHigh = 0.95
	minStrokePx     = 1.0
)

type stroke struct {
	el        Element
	kind      strokeKind
	polygon   []Point
	thickness float64
}

type strokeCrossing struct {
	stroke Element
	text   Element
	kind   strokeKind
	area   []Point
}

func visibleColor(color string) bool {
	parsed := parseColor(color)
	return parsed != nil && parsed.A > 0.01
}

func filled(layer *studio.Layer) bool {
	paint := paintOf(layer)
	return paint != nil && paint.A > 0.01
}

func strokeOf(el Element, canvas Canvas) *stroke {
	layer := el.Layer
	if el.Opacity <= 0 || layer == nil || layer.Type != studio.LayerShape {
		return nil
	}
	t := el.Transform
	width := layer.StrokeWidth
	if strokedShape(layer) && width > 0 && visibleColor(layer.Stroke) {
		thickness := math.Max(minStrokePx, width)
		thin := t
		thin.H = thickness / canvas.Height
		return &stroke{el: el, kind: strokeKind(layer.Shape), polygon: boxCorners(thin, canvas), thickness: thickness}
	}
	if layer.Shape != studio.ShapeRect {
		return nil
	}
	if filled(layer) {
		short, long := t.W*canvas.Width, t.H*canvas.Height
		if long < short {
			short, long = long, short
		}
		if short > thinShare*math.Min(canvas.Width, canvas.Height) || long < longRatio*short {
			return nil
		}
		return &stroke{el: el, kind: strokeBar, polygon: boxCorners(t, canvas), thickness: math.Max(minStrokePx, short)}
	}
	if width > 0 && visibleColor(layer.Stroke) {
		return &stroke{el: el, kind: strokeBorder, polygon: boxCorners(t, canvas), thickness: width}
	}
	return nil
}

func innerInk(ink Box, canvas Canvas) []Point {
	left, right, top, bottom := ink.Left*canvas.Width, ink.Right*canvas.Width, ink.Top*canvas.Height, ink.Bottom*canvas.Height
	inset := (bottom - top) * inkInset
	return []Point{{left + inset, top + inset}, {right - inset, top + inset}, {right - inset, bottom - inset}, {left + inset, bottom - inset}}
}

func crossingArea(s *stroke, inner []Point) []Point {
	left, top, right, bottom := inner[0][0], inner[0][1], inner[2][0], inner[2][1]
	if right <= left || bottom <= top {
		return nil
	}
	shared := convexIntersection(s.polygon, inner)
	if s.kind == strokeBorder {
		share := convexArea(shared) / convexArea(inner)
		if share > borderShareLow && share < borderShareHigh {
			return inner
		}
		return nil
	}
	if convexArea(shared)/s.thickness >= crossShare*math.Min(right-left, bottom-top) {
		return shared
	}
	return nil
}

func strokeCrossings(scene *Scene) []strokeCrossing {
	var strokes []*stroke
	for _, el := range scene.Elements {
		if s := strokeOf(el, scene.Canvas); s != nil {
			strokes = append(strokes, s)
		}
	}
	if len(strokes) == 0 {
		return nil
	}
	var crossings []strokeCrossing
	for _, text := range scene.Elements {
		if !isText(text) || text.Opacity <= 0 || !isUpright(text.Transform) {
			continue
		}
		inner := innerInk(inkBox(scene.Canvas, text), scene.Canvas)
		for _, s := range strokes {
			if s.el.ID == text.ID && s.el.Layer == text.Layer {
				continue
			}
			if found := crossingArea(s, inner); found != nil {
				crossings = append(crossings, strokeCrossing{stroke: s.el, text: text, kind: s.kind, area: found})
			}
		}
	}
	return crossings
}
