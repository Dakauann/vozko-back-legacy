package perception

import (
	"fmt"
	"math"
	"strings"

	"vozko/domain/studio"
)

type DepthReport struct {
	Light    *string   `json:"light"`
	Shadows  []string  `json:"shadows"`
	Covered  []string  `json:"covered"`
	Findings []Finding `json:"findings"`
	Shapes   []Shape   `json:"shapes"`
}

var (
	heavyAlpha        = 0.6
	hardAlpha         = 0.45
	hardBlurShare     = 0.5
	clipSlackPx       = 2.0
	neutralChroma     = 0.08
	colorfulGround    = 0.35
	groundLumLow      = 0.08
	groundLumHigh     = 0.8
	turnedDegrees     = 5.0
	sameLightDegrees  = 40.0
	coverSamples      = 12
	reportCover       = 0.25
	hiddenCover       = 0.5
	depthBackground   = 0.5
	arrowMinPx        = 40.0
	arrowScale        = 3.0
	totallyHidden     = 0.9
	neutralShadowTint = 0.45
)

var kindNames = map[studio.LayerType]string{studio.LayerText: "texto", studio.LayerImage: "imagem", studio.LayerShape: "forma", studio.LayerIcon: "ícone"}

func chroma(c Rgba) float64 {
	return (math.Max(c.R, math.Max(c.G, c.B)) - math.Min(c.R, math.Min(c.G, c.B))) / 255
}

func groundUnder(scene *Scene, index int, cast *shadowCast, t studio.Transform) *Rgba {
	if cast.distance == 0 {
		return nil
	}
	canvas := scene.Canvas
	radians := radiansOf(t.Rotation)
	type side struct{ nx, ny, half, facing float64 }
	sides := []side{
		{nx: math.Cos(radians), ny: math.Sin(radians), half: t.W * canvas.Width / 2},
		{nx: -math.Sin(radians), ny: math.Cos(radians), half: t.H * canvas.Height / 2},
	}
	for i, s := range sides {
		facing := (s.nx*cast.dx + s.ny*cast.dy) / cast.distance
		if facing >= 0 {
			sides[i].facing = facing
		} else {
			sides[i] = side{nx: -s.nx, ny: -s.ny, half: s.half, facing: -facing}
		}
	}
	edge := sides[1]
	if sides[0].facing >= sides[1].facing {
		edge = sides[0]
	}
	x := t.X*canvas.Width + edge.nx*edge.half + cast.dx
	y := t.Y*canvas.Height + edge.ny*edge.half + cast.dy
	return backdrop(scene, index, x/canvas.Width, y/canvas.Height)
}

type cutSide struct {
	name string
	edge Shape
}

func cutSides(cast *shadowCast, bounds Box, canvas Canvas) []cutSide {
	reach := cast.blur/2 + math.Max(cast.spread, 0)
	left, right, top, bottom := bounds.Left/canvas.Width, bounds.Right/canvas.Width, bounds.Top/canvas.Height, bounds.Bottom/canvas.Height
	type candidate struct {
		name             string
		overflow, inside float64
		edge             Shape
	}
	candidates := []candidate{
		{"esquerda", -(bounds.Left + cast.dx - reach), bounds.Left, line("segment", UnitPoint{0, top}, UnitPoint{0, bottom}, ToneBad)},
		{"direita", bounds.Right + cast.dx + reach - canvas.Width, canvas.Width - bounds.Right, line("segment", UnitPoint{1, top}, UnitPoint{1, bottom}, ToneBad)},
		{"topo", -(bounds.Top + cast.dy - reach), bounds.Top, line("segment", UnitPoint{left, 0}, UnitPoint{right, 0}, ToneBad)},
		{"base", bounds.Bottom + cast.dy + reach - canvas.Height, canvas.Height - bounds.Bottom, line("segment", UnitPoint{left, 1}, UnitPoint{right, 1}, ToneBad)},
	}
	var cut []cutSide
	for _, c := range candidates {
		if c.overflow >= clipSlackPx && c.inside >= 0 {
			cut = append(cut, cutSide{name: c.name, edge: c.edge})
		}
	}
	return cut
}

func arrowOf(cast *shadowCast, bounds Box, canvas Canvas, tone Tone) Shape {
	length := math.Max(arrowMinPx, cast.distance*arrowScale)
	cx := (bounds.Left + bounds.Right) / 2
	cy := (bounds.Top + bounds.Bottom) / 2
	return line("arrow", UnitPoint{cx / canvas.Width, cy / canvas.Height}, UnitPoint{(cx + cast.dx/cast.distance*length) / canvas.Width, (cy + cast.dy/cast.distance*length) / canvas.Height}, tone)
}

func heavyCast(layer *studio.Layer, cast *shadowCast, canvas Canvas) string {
	if cast.kind != castDrop {
		return ""
	}
	if layer.Type == studio.LayerText && layer.Shadow != nil && len(layer.Effects) == 0 {
		if heavyTextShadow(layer.Shadow, layer.FontSize*canvas.Height) {
			return "pesada para o tamanho da letra"
		}
		return ""
	}
	if cast.alpha >= heavyAlpha {
		return fmt.Sprintf("quase opaca (alfa %s)", num(rounded(cast.alpha, 2)))
	}
	if cast.alpha >= hardAlpha && cast.blur < hardBlurShare*cast.distance {
		return fmt.Sprintf("dura e escura (alfa %s, blur %s para %s px de deslocamento)", num(rounded(cast.alpha, 2)), num(cast.blur), num(rounded(cast.distance, 0)))
	}
	return ""
}

func castLine(cast *shadowCast, ground *Rgba) string {
	over := ""
	if ground != nil {
		over = " sobre " + hexOf(*ground)
	}
	if cast.kind == castGlow {
		return fmt.Sprintf("%s: brilho, blur %s, alfa %s, %s%s", cast.layerID, num(cast.blur), num(rounded(cast.alpha, 2)), hexOf(cast.color), over)
	}
	share := ""
	if cast.side > 0 {
		share = fmt.Sprintf(" (%s%% do lado menor)", num(rounded(100*cast.distance/cast.side, 0)))
	}
	return fmt.Sprintf("%s: %s %s°, %s px%s, blur %s, alfa %s, %s%s", cast.layerID, directionWord(cast.angle), num(rounded(cast.angle, 0)), num(rounded(cast.distance, 0)), share, num(cast.blur), num(rounded(cast.alpha, 2)), hexOf(cast.color), over)
}

func lightLine(casts []*shadowCast) *string {
	light := lightDirection(casts)
	if light == nil {
		return nil
	}
	verb := "sombras caem"
	if light.count == 1 {
		verb = "sombra cai"
	}
	text := fmt.Sprintf("%d %s %s (%s°), concordância %s", light.count, verb, directionWord(light.angle), num(rounded(light.angle, 0)), num(rounded(light.agreement, 2)))
	return &text
}

func turnedHint(layer *studio.Layer, cast *shadowCast, light float64) string {
	if len(layer.Effects) == 0 || math.Abs(layer.Transform.Rotation) < turnedDegrees {
		return ""
	}
	for _, effect := range layer.Effects {
		if effect.ID != cast.effectID {
			continue
		}
		local := math.Atan2(effect.Y, effect.X) * 180 / math.Pi
		if angleApart(local, light) <= sameLightDegrees {
			return fmt.Sprintf("; a sombra gira junto com a camada (rotation %s): gire x e y da sombra ao contrário para ela cair como as outras", num(rounded(layer.Transform.Rotation, 1)))
		}
		return ""
	}
	return ""
}

type placedCast struct {
	layer  *studio.Layer
	cast   *shadowCast
	ground *Rgba
	bounds Box
}

func shadowFindings(scene *Scene) ([]placedCast, []Finding, []Shape) {
	canvas := scene.Canvas
	var findings []Finding
	var shapes []Shape
	push := func(kind, id, text string) {
		findings = append(findings, Finding{Kind: kind, Level: LevelWarning, IDs: []string{id}, Text: text})
	}
	var placed []placedCast
	for index, el := range scene.Elements {
		if el.Layer == nil {
			continue
		}
		layer := *el.Layer
		layer.ID = el.ID
		layer.Transform = el.Transform
		bounds := boxBounds(layer.Transform, canvas)
		for _, cast := range shadowCasts(&layer, canvas) {
			ground := groundUnder(scene, index, cast, layer.Transform)
			cut := cutSides(cast, bounds, canvas)
			for _, side := range cut {
				shapes = append(shapes, side.edge)
			}
			if len(cut) > 0 {
				names := make([]string, len(cut))
				for i, side := range cut {
					names[i] = side.name
				}
				noun := "sombra"
				if cast.kind == castGlow {
					noun = "luz"
				}
				push("shadow_clipped", layer.ID, fmt.Sprintf("a %s de %s é cortada pela borda da arte (%s) enquanto a camada fica dentro; afaste da borda ou deixe a camada sangrar de propósito", noun, layer.ID, strings.Join(names, ", ")))
			}
			if weight := heavyCast(&layer, cast, canvas); weight != "" {
				push("shadow_heavy", layer.ID, fmt.Sprintf("a sombra de %s está %s; sombra suave usa alfa de 0.15 a 0.35 e blur de cerca de 2 vezes o deslocamento", layer.ID, weight))
			}
			if cast.kind == castDrop && ground != nil {
				g := *ground
				switch {
				case !shadowShows(cast.color, cast.alpha, g):
					push("shadow_invisible", layer.ID, fmt.Sprintf("a sombra de %s (%s) quase não aparece sobre %s; sobre fundo escuro, separe com um contorno claro, um brilho ou uma superfície mais clara", layer.ID, hexOf(cast.color), hexOf(g)))
				case luminance(cast.color) > luminance(g):
					push("shadow_light", layer.ID, fmt.Sprintf("a sombra de %s é mais clara que o fundo onde cai (%s sobre %s); sombra escurece, para clarear use um brilho", layer.ID, hexOf(cast.color), hexOf(g)))
				case chroma(cast.color) < neutralChroma && chroma(g) >= colorfulGround && luminance(g) >= groundLumLow && luminance(g) <= groundLumHigh:
					tint := Rgba{R: g.R * neutralShadowTint, G: g.G * neutralShadowTint, B: g.B * neutralShadowTint}
					push("shadow_neutral", layer.ID, fmt.Sprintf("a sombra de %s é cinza neutra sobre um fundo colorido (%s) e suja a cor; use o tom escuro do próprio fundo, como %s", layer.ID, hexOf(g), hexOf(tint)))
				}
			}
			owned := layer
			placed = append(placed, placedCast{layer: &owned, cast: cast, ground: ground, bounds: bounds})
		}
	}
	casts := make([]*shadowCast, len(placed))
	for i, p := range placed {
		casts[i] = p.cast
	}
	outliers := lightOutliers(casts)
	isOutlier := func(cast *shadowCast) bool {
		for _, o := range outliers {
			if o.cast == cast {
				return true
			}
		}
		return false
	}
	for _, p := range placed {
		if p.cast.kind == castDrop && p.cast.distance > 0 {
			tone := ToneOK
			if isOutlier(p.cast) {
				tone = ToneBad
			}
			shapes = append(shapes, arrowOf(p.cast, p.bounds, canvas, tone))
		}
	}
	for _, outlier := range outliers {
		var layer *studio.Layer
		for _, p := range placed {
			if p.cast == outlier.cast {
				layer = p.layer
				break
			}
		}
		push("shadow_direction", layer.ID, fmt.Sprintf("a sombra de %s cai %s (%s°), contra a luz das outras, que caem %s (%s°)%s", layer.ID, directionWord(outlier.cast.angle), num(rounded(outlier.cast.angle, 0)), directionWord(outlier.light), num(rounded(outlier.light, 0)), turnedHint(layer, outlier.cast, outlier.light)))
	}
	return placed, findings, shapes
}

func coverage(polygon []Point, above [][]Point) float64 {
	left, top := math.Inf(1), math.Inf(1)
	right, bottom := math.Inf(-1), math.Inf(-1)
	for _, p := range polygon {
		left, right = math.Min(left, p[0]), math.Max(right, p[0])
		top, bottom = math.Min(top, p[1]), math.Max(bottom, p[1])
	}
	width, height := right-left, bottom-top
	inside, hidden := 0, 0
	samples := float64(coverSamples)
	for i := 0; i < coverSamples; i++ {
		for j := 0; j < coverSamples; j++ {
			point := Point{left + (float64(i)+0.5)/samples*width, top + (float64(j)+0.5)/samples*height}
			if !insideConvex(point, polygon) {
				continue
			}
			inside++
			for _, cover := range above {
				if insideConvex(point, cover) {
					hidden++
					break
				}
			}
		}
	}
	if inside == 0 {
		return 0
	}
	return float64(hidden) / float64(inside)
}

func occlusion(scene *Scene) ([]string, []Finding, []Shape) {
	canvas := scene.Canvas
	canvasArea := canvas.Width * canvas.Height
	polygons := make([][]Point, len(scene.Elements))
	for i, el := range scene.Elements {
		polygons[i] = boxCorners(el.Transform, canvas)
	}
	lines := []string{}
	var findings []Finding
	var shapes []Shape
	for index, el := range scene.Elements {
		own := polygons[index]
		ownArea := convexArea(own)
		if ownArea == 0 || ownArea >= depthBackground*canvasArea {
			continue
		}
		type coverer struct {
			id      string
			polygon []Point
		}
		var coverers []coverer
		for j := index + 1; j < len(scene.Elements); j++ {
			other := scene.Elements[j]
			if intersection(el.Box, other.Box) > 0 && covers(other) && (other.Layer == nil || !other.Layer.Clip) && convexArea(polygons[j]) > ownArea {
				coverers = append(coverers, coverer{id: other.ID, polygon: polygons[j]})
			}
		}
		all := make([][]Point, len(coverers))
		for i, c := range coverers {
			all[i] = c.polygon
		}
		share := coverage(own, all)
		if share < reportCover {
			continue
		}
		var by []string
		for _, c := range coverers {
			if coverage(own, [][]Point{c.polygon}) > 0 {
				by = append(by, c.id)
			}
		}
		kind := "mídia"
		name := ""
		if layer := el.Layer; layer != nil {
			kind = string(layer.Type)
			if word, known := kindNames[layer.Type]; known {
				kind = word
			}
			if layer.Type == studio.LayerText {
				name = fmt.Sprintf(" \"%s\"", snippet(layer))
			}
		}
		verb := "estão"
		if len(by) == 1 {
			verb = "está"
		}
		text := fmt.Sprintf("%s (%s%s) %s%% coberto por %s, que %s acima", el.ID, kind, name, pxNum(share*100), joinIDs(by), verb)
		lines = append(lines, text)
		if share < hiddenCover {
			continue
		}
		unit := make([]UnitPoint, len(own))
		for i, p := range own {
			unit[i] = UnitPoint{p[0] / canvas.Width, p[1] / canvas.Height}
		}
		shapes = append(shapes, Shape{Kind: "area", Points: unit, Tone: ToneBad})
		level := LevelWarning
		if share >= totallyHidden {
			level = LevelError
		}
		findings = append(findings, Finding{Kind: "hidden_layer", Level: level, IDs: append([]string{el.ID}, by...), Text: fmt.Sprintf("%s; leve %s para cima de %s com order_layers se ele deve aparecer", text, el.ID, joinIDs(by))})
	}
	return lines, findings, shapes
}

func sceneDepth(scene *Scene) DepthReport {
	placed, shadowFound, shadowShapes := shadowFindings(scene)
	lines, hiddenFound, hiddenShapes := occlusion(scene)
	casts := make([]*shadowCast, len(placed))
	shadows := make([]string, len(placed))
	for i, p := range placed {
		casts[i] = p.cast
		shadows[i] = castLine(p.cast, p.ground)
	}
	report := DepthReport{Light: lightLine(casts), Shadows: shadows, Covered: lines, Findings: []Finding{}, Shapes: []Shape{}}
	report.Findings = append(append(report.Findings, hiddenFound...), shadowFound...)
	report.Shapes = append(append(report.Shapes, shadowShapes...), hiddenShapes...)
	return report
}
