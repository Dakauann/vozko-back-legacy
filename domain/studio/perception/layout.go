package perception

import (
	"fmt"
	"math"
	"strings"

	"vozko/domain/studio"
)

type LayoutReport struct {
	Margins  string    `json:"margins"`
	Levels   *string   `json:"levels"`
	Reading  *string   `json:"reading"`
	Balance  string    `json:"balance"`
	Findings []Finding `json:"findings"`
	Shapes   []Shape   `json:"shapes"`
}

type shaped struct {
	Finding
	shapes []Shape
}

var (
	tangentShare   = 0.008
	spillLow       = 0.1
	spillHigh      = 0.85
	collideLow     = 0.03
	collideHigh    = 0.7
	sameLevel      = 0.03
	minHeroRatio   = 1.25
	strongHero     = 1.5
	focalFont      = 0.08
	focalMedia     = 0.12
	guideMargin    = 0.01
	readingRows    = 50.0
	textWeight     = 2.0
	mediaContrast  = 0.5
	inkContrastPad = 0.05
)

var edgeWords = map[EdgeName]string{EdgeLeft: "pela esquerda", EdgeCenter: "pelo centro", EdgeRight: "pela direita", EdgeTop: "pelo topo", EdgeMiddle: "pelo meio", EdgeBottom: "pela base"}

var sideWords = []string{"esquerda", "direita", "topo", "base"}

func edgePx(edge EdgeName, distance float64, canvas Canvas) string {
	if isVerticalEdge(edge) {
		return pxNum(distance * canvas.Width)
	}
	return pxNum(distance * canvas.Height)
}

func edgeGuide(edge EdgeName, at float64, members []*layoutItem, tone Tone) Shape {
	vertical := isVerticalEdge(edge)
	low, high := math.Inf(1), math.Inf(-1)
	for _, m := range members {
		if vertical {
			low, high = math.Min(low, m.box.Top), math.Max(high, m.box.Bottom)
		} else {
			low, high = math.Min(low, m.box.Left), math.Max(high, m.box.Right)
		}
	}
	low = math.Max(0, low-guideMargin)
	high = math.Min(1, high+guideMargin)
	if vertical {
		return line("guide", UnitPoint{at, low}, UnitPoint{at, high}, tone)
	}
	return line("guide", UnitPoint{low, at}, UnitPoint{high, at}, tone)
}

func alignmentGuides(miss alignmentMiss, items []*layoutItem) []Shape {
	span := []*layoutItem{miss.neighbor}
	if clearBetween(miss.off, miss.neighbor, miss.edge, items) {
		span = []*layoutItem{miss.off, miss.neighbor}
	}
	return []Shape{edgeGuide(miss.edge, edgeAt(miss.edge, miss.neighbor.box), span, ToneOK), edgeGuide(miss.edge, edgeAt(miss.edge, miss.off.box), []*layoutItem{miss.off}, ToneBad)}
}

func misalignments(items []*layoutItem, canvas Canvas) []shaped {
	var out []shaped
	for _, miss := range alignmentMisses(items) {
		ids := append([]string{miss.off.el.ID}, miss.partners...)
		text := fmt.Sprintf("%s está %s px fora da linha de %s %s; alinhe pelo mesmo ponto ou afaste de propósito", miss.off.el.ID, edgePx(miss.edge, miss.distance, canvas), joinIDs(miss.partners), edgeWords[miss.edge])
		out = append(out, shaped{Finding: Finding{Kind: "misalign", Level: LevelWarning, IDs: ids, Text: text}, shapes: alignmentGuides(miss, items)})
	}
	return out
}

func unitArea(points []Point, canvas Canvas) []Shape {
	if len(points) == 0 {
		return nil
	}
	unit := make([]UnitPoint, len(points))
	for i, p := range points {
		unit[i] = UnitPoint{p[0] / canvas.Width, p[1] / canvas.Height}
	}
	return []Shape{{Kind: "area", Points: unit, Tone: ToneBad}}
}

func strokeOverlaps(scene *Scene) []shaped {
	var out []shaped
	for _, cross := range strokeCrossings(scene) {
		text := fmt.Sprintf("%s %s atravessa o texto %s; mova um dos dois para que não se cruzem, a menos que o risco seja de propósito", strokeNouns[cross.kind], cross.stroke.ID, cross.text.ID)
		out = append(out, shaped{Finding: Finding{Kind: "overlap", Level: LevelWarning, IDs: []string{cross.stroke.ID, cross.text.ID}, Text: text}, shapes: unitArea(cross.area, scene.Canvas)})
	}
	return out
}

func sideSegment(b Box, side int) Shape {
	ends := [4][2]UnitPoint{
		{{b.Left, b.Top}, {b.Left, b.Bottom}},
		{{b.Right, b.Top}, {b.Right, b.Bottom}},
		{{b.Left, b.Top}, {b.Right, b.Top}},
		{{b.Left, b.Bottom}, {b.Right, b.Bottom}},
	}
	return line("segment", ends[side][0], ends[side][1], ToneBad)
}

func sideGaps(b Box, canvas Canvas) [4]float64 {
	return [4]float64{b.Left * canvas.Width, (1 - b.Right) * canvas.Width, b.Top * canvas.Height, (1 - b.Bottom) * canvas.Height}
}

func marginFindings(items []*layoutItem, canvas Canvas) []shaped {
	wanted := marginShare * math.Min(canvas.Width, canvas.Height)
	var out []shaped
	for _, item := range items {
		gaps := sideGaps(item.box, canvas)
		if bleeds(item.box) > 0 {
			continue
		}
		var words []string
		var shapes []Shape
		for i, gap := range gaps {
			if gap < wanted {
				words = append(words, fmt.Sprintf("%s %s px", sideWords[i], pxNum(gap)))
				shapes = append(shapes, sideSegment(item.box, i))
			}
		}
		if len(words) == 0 {
			continue
		}
		text := fmt.Sprintf("%s encosta na borda da arte (%s); deixe pelo menos %s px ou sangre de verdade", item.el.ID, strings.Join(words, ", "), pxNum(wanted))
		out = append(out, shaped{Finding: Finding{Kind: "crowded_margin", Level: LevelWarning, IDs: []string{item.el.ID}, Text: text}, shapes: shapes})
	}
	return out
}

func marginsLine(items []*layoutItem, canvas Canvas) string {
	var inside []*layoutItem
	for _, item := range items {
		gaps := sideGaps(item.box, canvas)
		if gaps[0] >= 0 && gaps[1] >= 0 && gaps[2] >= 0 && gaps[3] >= 0 {
			inside = append(inside, item)
		}
	}
	if len(inside) == 0 {
		return "menor respiro: nada fica inteiro dentro da arte"
	}
	parts := make([]string, len(sideWords))
	for i, word := range sideWords {
		least := math.Inf(1)
		gaps := make([]float64, len(inside))
		for k, item := range inside {
			gaps[k] = jsRound(sideGaps(item.box, canvas)[i])
			least = math.Min(least, gaps[k])
		}
		var ids []string
		for k, item := range inside {
			if gaps[k] == least {
				ids = append(ids, item.el.ID)
			}
		}
		parts[i] = fmt.Sprintf("%s %s px (%s)", word, num(least), joinIDs(ids))
	}
	return "menor respiro: " + strings.Join(parts, ", ")
}

func middle(low, high float64) float64 {
	return (low + high) / 2
}

func gapPoint(a, b Box, across bool) UnitPoint {
	if across {
		y := middle(b.Bottom, a.Top)
		if a.Bottom < b.Top {
			y = middle(a.Bottom, b.Top)
		}
		return UnitPoint{middle(math.Max(a.Left, b.Left), math.Min(a.Right, b.Right)), y}
	}
	x := middle(b.Right, a.Left)
	if a.Right < b.Left {
		x = middle(a.Right, b.Left)
	}
	return UnitPoint{x, middle(math.Max(a.Top, b.Top), math.Min(a.Bottom, b.Bottom))}
}

func tangents(items []*layoutItem, canvas Canvas) []shaped {
	limit := tangentShare * math.Min(canvas.Width, canvas.Height)
	var upright []*layoutItem
	for _, item := range items {
		if item.upright {
			upright = append(upright, item)
		}
	}
	var out []shaped
	for i, a := range upright {
		for _, b := range upright[i+1:] {
			if nestedItems(a, b) {
				continue
			}
			gapX := math.Max(a.box.Left-b.box.Right, b.box.Left-a.box.Right) * canvas.Width
			gapY := math.Max(a.box.Top-b.box.Bottom, b.box.Top-a.box.Bottom) * canvas.Height
			var gap float64
			switch {
			case gapX < 0 && gapY > 0:
				gap = gapY
			case gapY < 0 && gapX > 0:
				gap = gapX
			default:
				continue
			}
			if gap > limit {
				continue
			}
			ring := gapPoint(a.box, b.box, gapX < 0)
			text := fmt.Sprintf("%s e %s quase se encostam (%s px); encoste de vez, sobreponha de propósito ou afaste", b.el.ID, a.el.ID, pxNum(gap))
			out = append(out, shaped{Finding: Finding{Kind: "tangent", Level: LevelWarning, IDs: []string{b.el.ID, a.el.ID}, Text: text}, shapes: []Shape{{Kind: "ring", At: &ring, Tone: ToneBad}}})
		}
	}
	return out
}

func solid(layer *studio.Layer) bool {
	if layer == nil {
		return false
	}
	if layer.Type == studio.LayerImage {
		return true
	}
	return layer.Type == studio.LayerShape && !strokedShape(layer) && paintOf(layer) != nil
}

func panelKind(item *layoutItem) string {
	layer := item.el.Layer
	if layer == nil || item.text || !solid(layer) {
		return ""
	}
	if layer.Type == studio.LayerImage {
		return "image"
	}
	return "shape:" + string(layer.Shape)
}

func percent(share float64) string {
	return pxNum(share * 100)
}

func overlaps(items []*layoutItem, canvas Canvas) []shaped {
	var out []shaped
	for i, a := range items {
		for _, b := range items[i+1:] {
			lower, upper := a, b
			if a.text && b.text {
				if overlapShare(a.corners, b.corners) >= overlapLimit {
					out = append(out, shaped{Finding: Finding{Kind: "overlap", Level: LevelError, IDs: []string{upper.el.ID, lower.el.ID}, Text: fmt.Sprintf("os textos %s e %s se sobrepõem; afaste um do outro", upper.el.ID, lower.el.ID)}, shapes: unitArea(convexIntersection(a.corners, b.corners), canvas)})
				}
				continue
			}
			var textItem, panel *layoutItem
			switch {
			case a.text:
				textItem, panel = a, b
			case b.text:
				textItem, panel = b, a
			}
			if textItem != nil && solid(panel.el.Layer) {
				shared := convexIntersection(textItem.corners, panel.corners)
				share := convexArea(shared) / math.Max(1, convexArea(textItem.corners))
				if share > spillLow && share < spillHigh {
					out = append(out, shaped{Finding: Finding{Kind: "overlap", Level: LevelWarning, IDs: []string{textItem.el.ID, panel.el.ID}, Text: fmt.Sprintf("o texto %s fica %s%% dentro de %s e o resto sai pela borda; ponha o texto inteiro dentro ou inteiro fora", textItem.el.ID, percent(share), panel.el.ID)}, shapes: unitArea(shared, canvas)})
				}
				continue
			}
			kind := panelKind(a)
			if kind == "" || kind != panelKind(b) || nestedItems(a, b) {
				continue
			}
			shared := convexIntersection(a.corners, b.corners)
			share := overlapShare(a.corners, b.corners)
			if share > collideLow && share < collideHigh {
				out = append(out, shaped{Finding: Finding{Kind: "overlap", Level: LevelWarning, IDs: []string{upper.el.ID, lower.el.ID}, Text: fmt.Sprintf("%s invade %s%% de %s; separe os dois ou sobreponha bem mais, de propósito", upper.el.ID, percent(share), lower.el.ID)}, shapes: unitArea(shared, canvas)})
			}
		}
	}
	return out
}

type textLevel struct {
	size float64
	ids  []string
}

func textLevels(items []*layoutItem) []*textLevel {
	var texts []*layoutItem
	for _, item := range items {
		if item.text {
			texts = append(texts, item)
		}
	}
	stableSort(texts, func(a, b *layoutItem) bool { return a.el.Layer.FontSize > b.el.Layer.FontSize })
	var levels []*textLevel
	for _, item := range texts {
		size := item.el.Layer.FontSize
		if n := len(levels); n > 0 && size >= levels[n-1].size*(1-sameLevel) {
			levels[n-1].ids = append(levels[n-1].ids, item.el.ID)
			continue
		}
		levels = append(levels, &textLevel{size: size, ids: []string{item.el.ID}})
	}
	return levels
}

func levelsLine(levels []*textLevel) *string {
	if len(levels) == 0 {
		return nil
	}
	parts := make([]string, len(levels))
	for i, level := range levels {
		parts[i] = fmt.Sprintf("%s (%s)", num(rounded(level.size, 3)), joinIDs(level.ids))
		if i > 0 {
			parts[i] += fmt.Sprintf(" %sx", num(rounded(levels[i-1].size/level.size, 2)))
		}
	}
	noun := "níveis"
	if len(levels) == 1 {
		noun = "nível"
	}
	text := fmt.Sprintf("texto em %d %s: %s", len(levels), noun, strings.Join(parts, " > "))
	return &text
}

func hierarchy(levels []*textLevel) []shaped {
	if len(levels) < 2 {
		return nil
	}
	hero, next := levels[0], levels[1]
	ratio := hero.size / next.size
	if ratio >= minHeroRatio {
		return nil
	}
	ids := append(append([]string(nil), next.ids...), hero.ids...)
	text := fmt.Sprintf("os dois maiores tamanhos de texto ficam perto demais: %s (%s) e %s (%s), %sx; do herói ao nível seguinte use pelo menos 1.5x", joinIDs(hero.ids), num(rounded(hero.size, 3)), joinIDs(next.ids), num(rounded(next.size, 3)), num(rounded(ratio, 2)))
	return []shaped{{Finding: Finding{Kind: "hierarchy", Level: LevelWarning, IDs: ids, Text: text}}}
}

func readingLine(items []*layoutItem) *string {
	var texts []*layoutItem
	for _, item := range items {
		if item.text {
			texts = append(texts, item)
		}
	}
	if len(texts) == 0 {
		return nil
	}
	stableSort(texts, func(a, b *layoutItem) bool {
		rowA, rowB := jsRound(a.box.Top*readingRows), jsRound(b.box.Top*readingRows)
		if rowA != rowB {
			return rowA < rowB
		}
		return a.box.Left < b.box.Left
	})
	ids := make([]string, len(texts))
	for i, item := range texts {
		ids[i] = item.el.ID
	}
	text := "ordem de leitura: " + joinIDs(ids)
	return &text
}

func balanceLine(items []*layoutItem, scene *Scene) string {
	ground := 0.5
	if scene.Background != nil {
		ground = luminance(*scene.Background)
	}
	total, sx, sy := 0.0, 0.0, 0.0
	for _, item := range items {
		layer := item.el.Layer
		contrast := inkContrastPad
		switch {
		case layer != nil && layer.Type == studio.LayerImage:
			contrast = mediaContrast
		case layer != nil:
			ink := paintOf(layer)
			if ink == nil {
				ink = parseColor(layer.Stroke)
			}
			if ink != nil {
				contrast = math.Abs(luminance(*ink)-ground) + inkContrastPad
			}
		}
		weight := area(item.box) * item.el.Opacity * contrast
		if item.text {
			weight *= textWeight
		}
		total += weight
		sx += weight * ((item.box.Left + item.box.Right) / 2)
		sy += weight * ((item.box.Top + item.box.Bottom) / 2)
	}
	if total == 0 {
		return "peso visual: nada medido"
	}
	return fmt.Sprintf("peso visual centrado em x %s, y %s (o centro do quadro é 0.5, 0.5)", num(rounded(sx/total, 2)), num(rounded(sy/total, 2)))
}

func clippedTexts(scene *Scene, certain bool) []shaped {
	var out []shaped
	for _, el := range scene.Elements {
		if !isText(el) || el.Opacity <= 0 {
			continue
		}
		found := clippedFinding(scene.Canvas, el, certain)
		if found == nil {
			continue
		}
		out = append(out, shaped{Finding: Finding{Kind: "text_clipped", Level: found.level, IDs: []string{el.ID}, Text: fmt.Sprintf("o texto %s %s", el.ID, found.text)}, shapes: unitArea(cornersOf(el.Transform, scene.Canvas), scene.Canvas)})
	}
	return out
}

func focalIDs(items []*layoutItem, levels []*textLevel) map[string]bool {
	focal := map[string]bool{}
	if len(levels) > 0 {
		hero := levels[0]
		if hero.size >= focalFont && (len(levels) == 1 || hero.size/levels[1].size >= strongHero) {
			for _, id := range hero.ids {
				focal[id] = true
			}
		}
	}
	for _, item := range items {
		if item.el.Media && area(item.box) >= focalMedia {
			focal[item.el.ID] = true
		}
	}
	return focal
}

func bandFindings(scene *Scene, focal map[string]bool) []shaped {
	if !scene.Still {
		return nil
	}
	var out []shaped
	for _, band := range emptyBands(scene, focal) {
		points := []UnitPoint{{band.from, 0}, {band.to, 0}, {band.to, 1}, {band.from, 1}}
		side := "largura"
		if band.axis == "y" {
			points = []UnitPoint{{0, band.from}, {1, band.from}, {1, band.to}, {0, band.to}}
			side = "altura"
		}
		text := fmt.Sprintf("faixa vazia de %s %s a %s (%s%% da %s): só o fundo aparece ali; leve a imagem, o cenário, uma forma ou um padrão até essa área (zona segura só tira dali texto, logo e chamada) ou recomponha para ocupar o quadro", band.axis, num(rounded(band.from, 2)), num(rounded(band.to, 2)), pxNum((band.to-band.from)*100), side)
		out = append(out, shaped{Finding: Finding{Kind: "empty_band", Level: LevelWarning, IDs: []string{}, Text: text}, shapes: []Shape{{Kind: "area", Points: points, Tone: ToneBad}}})
	}
	return out
}

func itemIDs(items []*layoutItem) string {
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.el.ID
	}
	return joinIDs(ids)
}

func symmetryFindings(scene *Scene, items []*layoutItem) []shaped {
	if !scene.Still {
		return nil
	}
	found := symmetry(items)
	if found == nil {
		return nil
	}
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.el.ID
	}
	arrangement := fmt.Sprintf("toda centrada no eixo vertical (%s)", itemIDs(items))
	if len(found.pairs) > 0 {
		pairs := make([]string, len(found.pairs))
		for i, pair := range found.pairs {
			pairs[i] = pair[0].el.ID + " e " + pair[1].el.ID
		}
		axis := ""
		if len(found.axis) > 0 {
			axis = "; no eixo: " + itemIDs(found.axis)
		}
		arrangement = fmt.Sprintf("espelhada entre esquerda e direita (%s%s)", strings.Join(pairs, ", "), axis)
	}
	return []shaped{{Finding: Finding{Kind: "symmetry", Level: LevelWarning, IDs: ids, Text: fmt.Sprintf("a composição está %s; isso tende a ler estático, pode ser intencional", arrangement)}}}
}

func angleFindings(scene *Scene) []shaped {
	var out []shaped
	for _, pair := range nearParallel(angleGroups(scene.Elements)) {
		a, b := pair[0], pair[1]
		ids := append(append([]string(nil), a.IDs...), b.IDs...)
		sortStrings(ids)
		text := fmt.Sprintf("%s a %s° e %s a %s° ficam quase paralelas; iguale o ângulo ou afaste de propósito", joinIDs(a.IDs), num(a.Degrees), joinIDs(b.IDs), num(b.Degrees))
		out = append(out, shaped{Finding: Finding{Kind: "near_parallel", Level: LevelWarning, IDs: ids, Text: text}})
	}
	return out
}

func sceneLayout(scene *Scene, certain bool) LayoutReport {
	canvas := scene.Canvas
	items := layoutItems(scene)
	levels := textLevels(items)
	var all []shaped
	all = append(all, clippedTexts(scene, certain)...)
	all = append(all, overlaps(items, canvas)...)
	all = append(all, strokeOverlaps(scene)...)
	all = append(all, hierarchy(levels)...)
	all = append(all, misalignments(items, canvas)...)
	all = append(all, marginFindings(items, canvas)...)
	all = append(all, tangents(items, canvas)...)
	all = append(all, angleFindings(scene)...)
	all = append(all, bandFindings(scene, focalIDs(items, levels))...)
	all = append(all, symmetryFindings(scene, items)...)
	report := LayoutReport{Margins: marginsLine(items, canvas), Levels: levelsLine(levels), Reading: readingLine(items), Balance: balanceLine(items, scene), Findings: []Finding{}, Shapes: []Shape{}}
	for _, f := range all {
		report.Findings = append(report.Findings, f.Finding)
		report.Shapes = append(report.Shapes, f.shapes...)
	}
	return report
}
