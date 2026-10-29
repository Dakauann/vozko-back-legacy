package perception

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"vozko/domain/studio"
)

const MaxCheckLines = 12

var (
	storyRatio       = 1.7
	edgeTolerance    = 0.005
	coveredShare     = 0.5
	overlapLimit     = 0.15
	minFontError     = 0.026
	minFontWarning   = 0.037
	largeFont        = 0.045
	largeBoldFont    = 0.035
	darkLuminance    = 0.05
	outlineShare     = 0.05
	wordsPerSecond   = 3.3
	readingLead      = 0.4
	minReading       = 1.0
	subtitleRun      = 3
	subtitleGapMS    = 300.0
	marginShare      = 0.03
	nearMin          = 0.003
	nearCenterMax    = 0.015
	restMS           = 500.0
	coveringMedia    = 0.6
	edgeInset        = 0.005
	heavyShadowAlpha = 0.75
	heavyShadowBlur  = 0.35
	heavyShadowShift = 0.12
	safeSide         = 0.06
	safeTop          = 0.14
	safeBottom       = 0.35
	qrMinModulePx    = 3.0
	framesPerSecond  = 30.0
)

type checkScene struct {
	*Scene
	safe   *Box
	prefix string
}

type check struct {
	level Level
	key   string
	text  string
}

func storySafe() *Box {
	return &Box{Left: safeSide, Top: safeTop, Right: 1 - safeSide, Bottom: 1 - safeBottom}
}

func edgePoints() [][2]float64 {
	far := 1 - edgeInset
	return [][2]float64{{edgeInset, edgeInset}, {0.5, edgeInset}, {far, edgeInset}, {edgeInset, 0.5}, {far, 0.5}, {edgeInset, far}, {0.5, far}, {far, far}}
}

func outlineInk(layer *studio.Layer, fontPx float64) *Rgba {
	if layer.StrokeWidth >= outlineShare*fontPx {
		return parseColor(layer.Stroke)
	}
	return nil
}

func fontWeight(layer *studio.Layer) int {
	if layer.FontWeight == 0 {
		return 400
	}
	return layer.FontWeight
}

func neededContrast(layer *studio.Layer, fontPx, canvasHeight float64) float64 {
	if fontPx >= largeFont*canvasHeight || (fontWeight(layer) >= 700 && fontPx >= largeBoldFont*canvasHeight) {
		return 3
	}
	return 4.5
}

func textShadowFor(fontPx float64) studio.Shadow {
	return studio.Shadow{Color: studio.TextShadow.Color, Blur: jsRound(studio.TextShadow.BlurShare * fontPx), X: 0, Y: math.Max(1, jsRound(studio.TextShadow.OffsetShare*fontPx))}
}

func heavyTextShadow(shadow *studio.Shadow, fontPx float64) bool {
	shade := parseColor(shadow.Color)
	if shade == nil || shade.A < heavyShadowAlpha {
		return false
	}
	return shadow.Blur > heavyShadowBlur*fontPx || jsHypot(shadow.X, shadow.Y) > heavyShadowShift*fontPx
}

func near(distance, max float64) bool {
	return distance > nearMin && distance <= max
}

func textName(el Element) string {
	return fmt.Sprintf("o texto %s (\"%s\")", el.ID, snippet(el.Layer))
}

func layoutChecks(scene checkScene, el Element, name string) []check {
	var out []check
	push := func(key, text string) {
		out = append(out, check{level: LevelWarning, key: key + ":" + el.ID, text: scene.prefix + text})
	}
	canvas := scene.Canvas
	if math.Abs(el.Rotation) < 0.5 {
		ink := inkBox(canvas, el)
		gapPx := math.Min(math.Min(math.Min(ink.Left*canvas.Width, (1-ink.Right)*canvas.Width), ink.Top*canvas.Height), (1-ink.Bottom)*canvas.Height)
		wanted := marginShare * math.Min(canvas.Width, canvas.Height)
		if gapPx >= 0 && gapPx < wanted {
			push("margin", fmt.Sprintf("%s encosta na borda da arte (%s px de respiro); deixe pelo menos %s px", name, pxNum(gapPx), pxNum(wanted)))
		}
		offCenter := math.Abs(edgeAt(EdgeCenter, ink) - 0.5)
		if textAnchor(el.Layer) == EdgeCenter && near(offCenter, nearCenterMax) {
			push("center", fmt.Sprintf("%s está quase no centro, %s px fora; centralize com x 0.5 ou afaste de propósito", name, pxNum(offCenter*canvas.Width)))
		}
	}
	for _, lines := range el.Text.Paragraphs {
		n := len(lines)
		if n > 1 && !strings.Contains(lines[n-1], " ") && strings.Contains(lines[n-2], " ") {
			push("orphan", fmt.Sprintf("%s termina com uma palavra sozinha na última linha (\"%s\"); ajuste w ou font_size, ou quebre a frase em outro ponto", name, lines[n-1]))
			break
		}
	}
	return out
}

func groundUnderText(behind *Rgba, layer *studio.Layer) (*Rgba, *Rgba) {
	var highlight *Rgba
	if layer.Highlight != nil {
		highlight = parseColor(layer.Highlight.Color)
	}
	if highlight == nil {
		return behind, nil
	}
	switch {
	case behind != nil:
		mixed := mixColors(*behind, *highlight, highlight.A)
		return &mixed, highlight
	case highlight.A >= 0.99:
		return highlight, highlight
	}
	return nil, highlight
}

func textChecks(scene checkScene, index int, certain bool) []check {
	el := scene.Elements[index]
	if !isText(el) {
		return nil
	}
	layer := el.Layer
	canvas := scene.Canvas
	fontPx := layer.FontSize * canvas.Height
	name := textName(el)
	var out []check
	push := func(level Level, key, text string) {
		out = append(out, check{level: level, key: key + ":" + el.ID, text: scene.prefix + text})
	}
	own := cornersOf(el.Transform, canvas)
	for _, above := range scene.Elements[index+1:] {
		if covers(above) && sharedArea(own, cornersOf(above.Transform, canvas)) >= coveredShare*convexArea(own) {
			push(LevelError, "covered:"+above.ID, fmt.Sprintf("%s fica escondido atrás de %s; coloque %s numa faixa ou camada abaixo do texto", name, above.ID, above.ID))
			break
		}
	}
	letters := inkBox(canvas, el)
	outside := letters.Left < -edgeTolerance || letters.Top < -edgeTolerance || letters.Right > 1+edgeTolerance || letters.Bottom > 1+edgeTolerance
	if outside {
		push(LevelError, "outside", name+" sai da arte; traga a caixa para dentro do quadro")
	} else if safe := scene.safe; safe != nil && (letters.Left < safe.Left || letters.Top < safe.Top || letters.Right > safe.Right || letters.Bottom > safe.Bottom) {
		push(LevelWarning, "safe", name+" entra na faixa que a interface do Instagram cobre (topo 14%, base 35%, laterais 6%)")
	}
	share := fontPx / canvas.Width
	minimum := rounded(minFontWarning*canvas.Width/canvas.Height, 3)
	if share < minFontError {
		push(LevelError, "small", fmt.Sprintf("%s está pequeno demais para ler no celular (%s px numa arte de %s px); use font_size de pelo menos %s", name, pxNum(fontPx), num(canvas.Width), num(minimum)))
	} else if share < minFontWarning {
		push(LevelWarning, "small", fmt.Sprintf("%s está pequeno para o celular (%s px); prefira font_size de pelo menos %s", name, pxNum(fontPx), num(minimum)))
	}
	cx := (el.Box.Left + el.Box.Right) / 2
	cy := (el.Box.Top + el.Box.Bottom) / 2
	ground, highlight := groundUnderText(backdrop(scene.Scene, index, cx, cy), layer)
	ink := paintOf(layer)
	stroke := outlineInk(layer, fontPx)
	if ground != nil && ink != nil {
		ratio := contrastRatio(*ink, *ground)
		if stroke != nil {
			ratio = math.Max(ratio, contrastRatio(*stroke, *ground))
		}
		needed := neededContrast(layer, fontPx, canvas.Height)
		if ratio < needed {
			floor := 3.0
			if needed == 3 {
				floor = 2.2
			}
			level := LevelWarning
			if ratio < floor {
				level = LevelError
			}
			push(level, "contrast", fmt.Sprintf("contraste baixo em %s: %s:1 contra o fundo %s, precisa de %s:1; mude a cor do texto ou ponha uma caixa atrás", name, num(rounded(ratio, 1)), hexOf(*ground), num(needed)))
		}
	} else if ground == nil && stroke == nil && highlight == nil && layer.Shadow == nil {
		push(LevelWarning, "busy", name+" está sobre imagem ou vídeo sem caixa, contorno ou sombra; confira a leitura com studio_look")
	}
	if shadow := layer.Shadow; shadow != nil {
		if shade := parseColor(shadow.Color); shade != nil {
			lost := ground != nil && luminance(*ground) < darkLuminance && luminance(*shade) < darkLuminance
			suggested := textShadowFor(fontPx)
			if lost {
				push(LevelWarning, "shadow", fmt.Sprintf("a sombra escura de %s não aparece sobre o fundo escuro e só suja as letras; tire com clear [\"shadow\"]", name))
			} else if heavyTextShadow(shadow, fontPx) {
				push(LevelWarning, "shadow", fmt.Sprintf("sombra pesada em %s (opaca, blur %s); use cor como %s com blur perto de %s e y perto de %s", name, num(shadow.Blur), suggested.Color, num(suggested.Blur), num(suggested.Y)))
			}
		}
	}
	if found := clippedFinding(canvas, el, certain); found != nil {
		push(found.level, found.key, name+" "+found.text)
	}
	if outside {
		return out
	}
	return append(out, layoutChecks(scene, el, name)...)
}

var edgeNames = map[EdgeName]string{EdgeLeft: "na borda esquerda", EdgeRight: "na borda direita", EdgeCenter: "no centro"}

func listed(ids []string) string {
	if len(ids) < 2 {
		return strings.Join(ids, "")
	}
	return strings.Join(ids[:len(ids)-1], ", ") + " e " + ids[len(ids)-1]
}

func alignmentChecks(scene checkScene) []check {
	order := make(map[string]int, len(scene.Elements))
	for i, el := range scene.Elements {
		order[el.ID] = i
	}
	var out []check
	for _, miss := range alignmentMisses(layoutItems(scene.Scene)) {
		words, named := edgeNames[miss.edge]
		var texts []*layoutItem
		for _, member := range miss.members {
			if member.text {
				texts = append(texts, member)
			}
		}
		if !named || !miss.off.text || len(texts) == 0 {
			continue
		}
		ids := []string{miss.off.el.ID}
		for _, item := range texts {
			ids = append(ids, item.el.ID)
		}
		stableSort(ids, func(a, b string) bool { return order[a] < order[b] })
		out = append(out, check{level: LevelWarning, key: "align:" + strings.Join(ids, ":"), text: fmt.Sprintf("%sos textos %s estão quase alinhados %s (%s px de diferença); alinhe pelo mesmo ponto", scene.prefix, listed(ids), words, pxNum(miss.distance*scene.Canvas.Width))})
	}
	return out
}

func overlapChecks(scene checkScene) []check {
	type placed struct {
		el      Element
		corners []Point
	}
	var texts []placed
	for _, el := range scene.Elements {
		if isText(el) {
			texts = append(texts, placed{el: el, corners: cornersOf(el.Transform, scene.Canvas)})
		}
	}
	var out []check
	for i, a := range texts {
		for _, b := range texts[i+1:] {
			if overlapShare(a.corners, b.corners) >= overlapLimit {
				out = append(out, check{level: LevelError, key: "overlap:" + a.el.ID + ":" + b.el.ID, text: fmt.Sprintf("%sos textos %s e %s se sobrepõem; afaste um do outro ou mostre em momentos diferentes", scene.prefix, a.el.ID, b.el.ID)})
			}
		}
	}
	return out
}

type qrCode struct {
	id     string
	layer  *studio.Layer
	sidePx float64
	size   int
}

func qrChecks(codes []qrCode, prefix string) []check {
	var out []check
	for _, code := range codes {
		pixels := code.sidePx / float64(code.size+code.layer.QR.QuietZone*2)
		if pixels >= qrMinModulePx {
			continue
		}
		out = append(out, check{level: LevelWarning, key: "qr:" + code.id, text: fmt.Sprintf("%so QR Code %s tem módulos de %s px; aumente o código para pelo menos %s px por módulo para ler bem", prefix, code.id, num(rounded(pixels, 1)), num(qrMinModulePx))})
	}
	return out
}

func strokeChecks(scene checkScene) []check {
	var out []check
	for _, cross := range strokeCrossings(scene.Scene) {
		out = append(out, check{level: LevelWarning, key: "stroke:" + cross.stroke.ID + ":" + cross.text.ID, text: fmt.Sprintf("%s%s %s atravessa o texto %s (\"%s\"); mova um dos dois ou, se for um risco de propósito, deixe assim", scene.prefix, strokeNouns[cross.kind], cross.stroke.ID, cross.text.ID, snippet(cross.text.Layer))})
	}
	return out
}

func sceneChecks(scene checkScene, certain bool) []check {
	var out []check
	for i := range scene.Elements {
		out = append(out, textChecks(scene, i, certain)...)
	}
	out = append(out, overlapChecks(scene)...)
	out = append(out, strokeChecks(scene)...)
	return append(out, alignmentChecks(scene)...)
}

func report(found []check) []string {
	index := map[string]int{}
	var unique []check
	for _, f := range found {
		if at, seen := index[f.key]; seen {
			unique[at] = f
			continue
		}
		index[f.key] = len(unique)
		unique = append(unique, f)
	}
	var ordered []check
	for _, level := range []Level{LevelError, LevelWarning} {
		for _, f := range unique {
			if f.level == level {
				ordered = append(ordered, f)
			}
		}
	}
	lines := []string{}
	for _, f := range ordered[:min(len(ordered), MaxCheckLines)] {
		lines = append(lines, string(f.level)+": "+f.text)
	}
	if len(ordered) > MaxCheckLines {
		lines = append(lines, fmt.Sprintf("e mais %d pontos parecidos", len(ordered)-MaxCheckLines))
	}
	return lines
}

func surfaceQRCodes(surface *Surface) []qrCode {
	var codes []qrCode
	for i := range surface.Layers {
		layer := &surface.Layers[i]
		size, known := surface.QR[layer.ID]
		if layer.Hidden || layer.QR == nil || !known {
			continue
		}
		side := math.Min(layer.Transform.W*surface.Canvas.Width, layer.Transform.H*surface.Canvas.Height)
		codes = append(codes, qrCode{id: layer.ID, layer: layer, sidePx: side, size: size})
	}
	return codes
}

func ImageQRChecks(surface *Surface) []string {
	return report(qrChecks(surfaceQRCodes(surface), ""))
}

func ImageChecks(surface *Surface, certain bool) ([]string, error) {
	ground := imageScene(surface)
	if err := ground.measured(); err != nil {
		return nil, err
	}
	scene := checkScene{Scene: ground}
	if ground.Canvas.Height/ground.Canvas.Width >= storyRatio {
		scene.safe = storySafe()
	}
	return report(sceneChecks(scene, certain)), nil
}

func subtitleTrack(track studio.Track) bool {
	var texts []studio.Clip
	for _, clip := range track.Clips {
		if clip.Layer != nil && clip.Layer.Type == studio.LayerText {
			texts = append(texts, clip)
		}
	}
	stableSort(texts, func(a, b studio.Clip) bool { return a.StartMS < b.StartMS })
	run := 1
	for i := 1; i < len(texts); i++ {
		if float64(texts[i].StartMS-(texts[i-1].StartMS+texts[i-1].DurationMS)) <= subtitleGapMS {
			run++
		} else {
			run = 1
		}
		if run >= subtitleRun {
			return true
		}
	}
	return false
}

func readingChecks(doc *studio.VideoDocument) []check {
	var out []check
	for _, track := range doc.Tracks {
		if track.Kind != studio.TrackVisual || track.Hidden || subtitleTrack(track) {
			continue
		}
		for _, clip := range track.Clips {
			words := 0
			if clip.Layer != nil && clip.Layer.Type == studio.LayerText {
				words = len(jsWords(clip.Layer.Text))
			}
			if words == 0 || clip.Disabled {
				continue
			}
			needed := math.Max(minReading, readingLead+float64(words)/wordsPerSecond)
			if float64(clip.DurationMS) < needed*1000 {
				out = append(out, check{level: LevelWarning, key: "reading:" + clip.ID, text: fmt.Sprintf("o texto %s (\"%s\") fica pouco tempo na tela: %s s, e precisa de cerca de %s s para ler", clip.ID, snippet(clip.Layer), num(rounded(float64(clip.DurationMS)/1000, 1)), num(rounded(needed, 1)))})
			}
		}
	}
	return out
}

func visualClips(doc *studio.VideoDocument) []studio.Clip {
	var clips []studio.Clip
	for _, track := range doc.Tracks {
		if track.Kind != studio.TrackVisual || track.Hidden {
			continue
		}
		for _, clip := range track.Clips {
			if !clip.Disabled && clip.Type != studio.ClipAudio {
				clips = append(clips, clip)
			}
		}
	}
	return clips
}

func checkTimes(doc *studio.VideoDocument, rests map[string]float64) []float64 {
	seen := map[float64]bool{}
	var times []float64
	add := func(at float64) {
		if !seen[at] {
			seen[at] = true
			times = append(times, at)
		}
	}
	for _, clip := range visualClips(doc) {
		if clip.Layer == nil || clip.Layer.Type != studio.LayerText {
			continue
		}
		add(jsRound(float64(clip.StartMS) + float64(clip.DurationMS)/2))
		if rest, known := rests[clip.ID]; known && rest <= float64(clip.DurationMS)-restMS {
			add(float64(clip.StartMS) + rest)
		}
	}
	sort.Float64s(times)
	return times
}

func showsFrame(fromMS, toMS float64) bool {
	first := math.Ceil(fromMS*framesPerSecond/1000 - 1e-9)
	return first*1000/framesPerSecond < toMS
}

func gapChecks(doc *studio.VideoDocument) []check {
	type span struct{ start, end float64 }
	var spans []span
	for _, clip := range visualClips(doc) {
		spans = append(spans, span{float64(clip.StartMS), float64(clip.StartMS + clip.DurationMS)})
	}
	stableSort(spans, func(a, b span) bool { return a.start < b.start })
	duration := float64(doc.DurationMS)
	spans = append(spans, span{duration, duration})
	var out []check
	covered := 0.0
	for _, s := range spans {
		gapEnd := math.Min(s.start, duration)
		if gapEnd > covered && showsFrame(covered, gapEnd) {
			out = append(out, check{level: LevelError, key: "gap:" + num(covered), text: fmt.Sprintf("de %s s a %s s não há nenhum clipe visual e o vídeo mostra só o fundo; estenda um clipe ou feche o buraco", num(rounded(covered/1000, 1)), num(rounded(gapEnd/1000, 1)))})
		}
		covered = math.Max(covered, s.end)
	}
	return out
}

func edgeChecks(scene checkScene) []check {
	canvas := scene.Canvas
	frame := cornersOf(studio.Transform{X: 0.5, Y: 0.5, W: 1, H: 1, Opacity: 1}, canvas)
	var main *Element
	for i := range scene.Elements {
		el := scene.Elements[i]
		if el.Media && el.Opacity >= 0.99 && sharedArea(cornersOf(el.Transform, canvas), frame) >= coveringMedia*canvas.Width*canvas.Height {
			main = &scene.Elements[i]
			break
		}
	}
	if main == nil {
		return nil
	}
	for _, p := range edgePoints() {
		coveredHere := false
		for _, el := range scene.Elements {
			if covers(el) && insideElement(el, canvas, p[0], p[1]) {
				coveredHere = true
				break
			}
		}
		if !coveredHere {
			return []check{{level: LevelWarning, key: "edges:" + main.ID, text: fmt.Sprintf("%so fundo aparece nas bordas do quadro: o clipe %s não cobre a tela toda; aumente a escala ou ajuste x, y, w e h", scene.prefix, main.ID)}}
		}
	}
	return nil
}

func storyAspect(doc *studio.VideoDocument) bool {
	return doc.Canvas.Aspect == "story"
}

func (v *Video) checkScene(at float64) (checkScene, error) {
	frame, err := v.frameAt(at)
	if err != nil {
		return checkScene{}, err
	}
	scene := checkScene{Scene: v.sceneOf(frame), prefix: fmt.Sprintf("em %s s: ", num(rounded(at/1000, 1)))}
	if storyAspect(v.Document) {
		scene.safe = storySafe()
	}
	return scene, nil
}

func VideoChecks(video *Video, certain bool) ([]string, error) {
	doc := video.Document
	if doc == nil {
		return nil, ErrIncomplete
	}
	var found []check
	for _, at := range checkTimes(doc, video.Rests) {
		scene, err := video.checkScene(at)
		if err != nil {
			return nil, err
		}
		if err := scene.measured(); err != nil {
			return nil, err
		}
		found = append(found, sceneChecks(scene, certain)...)
	}
	var edges []check
	for _, at := range video.Cuts {
		scene, err := video.checkScene(at)
		if err != nil {
			return nil, err
		}
		edges = append(edges, edgeChecks(scene)...)
	}
	var codes []qrCode
	for _, clip := range visualClips(doc) {
		size, known := video.QR[clip.ID]
		if clip.Layer == nil || clip.Layer.QR == nil || !known {
			continue
		}
		layer := clip.Layer
		codes = append(codes, qrCode{id: clip.ID, layer: layer, sidePx: math.Min(clip.Transform.W*video.Canvas.Width, clip.Transform.H*video.Canvas.Height), size: size})
	}
	all := append(gapChecks(doc), found...)
	all = append(all, edges...)
	all = append(all, readingChecks(doc)...)
	all = append(all, qrChecks(codes, "")...)
	return report(all), nil
}
