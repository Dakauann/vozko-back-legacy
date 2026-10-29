package perception

import (
	"vozko/domain/studio"
)

type Element struct {
	ID        string
	Layer     *studio.Layer
	Media     bool
	Box       Box
	Transform studio.Transform
	Opacity   float64
	Rotation  float64
	Text      *TextFacts
}

type Scene struct {
	Canvas     Canvas
	Background *Rgba
	Elements   []Element
	Still      bool
}

type EdgeName string

const (
	EdgeLeft   EdgeName = "left"
	EdgeCenter EdgeName = "center"
	EdgeRight  EdgeName = "right"
	EdgeTop    EdgeName = "top"
	EdgeMiddle EdgeName = "middle"
	EdgeBottom EdgeName = "bottom"
)

var edgeOrder = []EdgeName{EdgeLeft, EdgeCenter, EdgeRight, EdgeTop, EdgeMiddle, EdgeBottom}

func edgeAt(edge EdgeName, b Box) float64 {
	switch edge {
	case EdgeLeft:
		return b.Left
	case EdgeCenter:
		return (b.Left + b.Right) / 2
	case EdgeRight:
		return b.Right
	case EdgeTop:
		return b.Top
	case EdgeMiddle:
		return (b.Top + b.Bottom) / 2
	}
	return b.Bottom
}

func textAnchor(layer *studio.Layer) EdgeName {
	if layer.Align == "" {
		return EdgeLeft
	}
	return EdgeName(layer.Align)
}

func gradientColor(g *studio.Gradient) *Rgba {
	var stops []Rgba
	for _, value := range []string{g.From, g.Via, g.To} {
		if c := parseColor(value); c != nil {
			stops = append(stops, *c)
		}
	}
	if len(stops) == 0 {
		return nil
	}
	var sum Rgba
	for _, c := range stops {
		sum.R += c.R
		sum.G += c.G
		sum.B += c.B
		sum.A += c.A
	}
	n := float64(len(stops))
	return &Rgba{R: jsRound(sum.R / n), G: jsRound(sum.G / n), B: jsRound(sum.B / n), A: sum.A / n}
}

func paintOf(layer *studio.Layer) *Rgba {
	if layer.Gradient != nil {
		return gradientColor(layer.Gradient)
	}
	return parseColor(layer.Fill)
}

func canvasPaint(background string, gradient *studio.Gradient) *Rgba {
	if gradient != nil {
		return gradientColor(gradient)
	}
	return parseColor(background)
}

func cornersOf(t studio.Transform, canvas Canvas) []Point {
	return boxCorners(t, canvas)
}

func insideElement(el Element, canvas Canvas, x, y float64) bool {
	return insideConvex(Point{x * canvas.Width, y * canvas.Height}, cornersOf(el.Transform, canvas))
}

func covers(el Element) bool {
	if el.Opacity < 0.99 {
		return false
	}
	if el.Media {
		return true
	}
	if el.Layer == nil || el.Layer.Type != studio.LayerShape || el.Layer.Shape != studio.ShapeRect {
		return false
	}
	paint := paintOf(el.Layer)
	return paint != nil && paint.A >= 0.99
}

func strokedShape(layer *studio.Layer) bool {
	return layer.Shape == studio.ShapeLine || layer.Shape == studio.ShapeArrow
}

func backdrop(scene *Scene, index int, x, y float64) *Rgba {
	color := scene.Background
	for _, el := range scene.Elements[:index] {
		if !insideElement(el, scene.Canvas, x, y) {
			continue
		}
		if el.Media {
			if el.Opacity >= 0.99 {
				color = nil
			}
			continue
		}
		layer := el.Layer
		if layer == nil || layer.Type != studio.LayerShape || strokedShape(layer) {
			continue
		}
		paint := paintOf(layer)
		if paint == nil {
			continue
		}
		alpha := paint.A * el.Opacity
		switch {
		case alpha >= 0.99:
			solid := *paint
			solid.A = 1
			color = &solid
		case color != nil:
			mixed := mixColors(*color, *paint, alpha)
			color = &mixed
		default:
			color = nil
		}
	}
	return color
}

func isText(el Element) bool {
	return el.Layer != nil && el.Layer.Type == studio.LayerText && jsTrim(el.Layer.Text) != ""
}

func imageScene(surface *Surface) *Scene {
	canvas := Canvas{Width: surface.Canvas.Width, Height: surface.Canvas.Height}
	background := surface.Canvas.Background
	if background == "" {
		background = "#ffffff"
	}
	scene := &Scene{Canvas: canvas, Background: canvasPaint(background, surface.Canvas.Gradient), Still: true}
	for i := range surface.Layers {
		layer := &surface.Layers[i]
		if layer.Hidden {
			continue
		}
		scene.Elements = append(scene.Elements, Element{
			ID: layer.ID, Layer: layer, Media: layer.Type == studio.LayerImage, Box: unitBoxOf(layer.Transform, canvas),
			Transform: layer.Transform, Opacity: layer.Transform.Opacity, Rotation: layer.Transform.Rotation, Text: surface.Texts[layer.ID],
		})
	}
	return scene
}

func frameScene(video *Video, frame *Frame) *Scene {
	scene := &Scene{Canvas: video.Canvas, Background: parseColor(video.Background)}
	for _, posed := range frame.Elements {
		el := Element{ID: posed.ID, Media: posed.Media, Box: unitBoxOf(posed.Transform, video.Canvas), Transform: posed.Transform, Opacity: posed.Opacity, Rotation: posed.Transform.Rotation, Text: posed.Text}
		if posed.Layer != nil {
			el.Layer = &video.Layers[*posed.Layer]
		}
		scene.Elements = append(scene.Elements, el)
	}
	return scene
}

func (s *Scene) measured() error {
	for _, el := range s.Elements {
		if isText(el) && el.Text == nil {
			return ErrUnmeasured
		}
	}
	return nil
}
