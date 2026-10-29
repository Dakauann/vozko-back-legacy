package perception

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"vozko/domain/mediagen"
	"vozko/domain/studio"
)

type MotionReport struct {
	Moves    []string  `json:"moves"`
	Variety  *string   `json:"variety"`
	Findings []Finding `json:"findings"`
}

type motionSample struct {
	atMS     float64
	elements []Element
	light    *float64
}

var (
	jumpPerFrame     = 0.1
	motionEdgeSlack  = 0.005
	flashStep        = 0.1
	flashDark        = 0.8
	flashesPerSecond = 3
	movingOverlap    = 0.15
	linearMove       = 0.15
	linearMinMS      = 200.0
	visibleOpacity   = 0.05
	lightPoints      = func() [][2]float64 {
		var points [][2]float64
		for _, x := range []float64{0.2, 0.5, 0.8} {
			for _, y := range []float64{0.2, 0.5, 0.8} {
				points = append(points, [2]float64{x, y})
			}
		}
		return points
	}()
)

func seconds(ms float64) string {
	return num(rounded(ms/1000, 1))
}

func frameLight(scene *Scene) *float64 {
	sum, count := 0.0, 0
	for _, p := range lightPoints {
		if color := backdrop(scene, len(scene.Elements), p[0], p[1]); color != nil {
			sum += luminance(*color)
			count++
		}
	}
	if count == 0 {
		return nil
	}
	light := sum / float64(count)
	return &light
}

func motionCenter(b Box) (float64, float64) {
	return (b.Left + b.Right) / 2, (b.Top + b.Bottom) / 2
}

func outsideFrame(b Box) bool {
	return b.Left < -motionEdgeSlack || b.Top < -motionEdgeSlack || b.Right > 1+motionEdgeSlack || b.Bottom > 1+motionEdgeSlack
}

func motionName(el Element) string {
	if el.Layer != nil && el.Layer.Type == studio.LayerText {
		return fmt.Sprintf("%s (\"%s\")", el.ID, snippet(el.Layer))
	}
	return el.ID
}

type trackEntry struct {
	atMS float64
	el   Element
}

type elementTracks struct {
	order []string
	byID  map[string][]trackEntry
}

func tracksOf(samples []motionSample) elementTracks {
	tracks := elementTracks{byID: map[string][]trackEntry{}}
	for _, sample := range samples {
		for _, el := range sample.elements {
			if _, seen := tracks.byID[el.ID]; !seen {
				tracks.order = append(tracks.order, el.ID)
			}
			tracks.byID[el.ID] = append(tracks.byID[el.ID], trackEntry{atMS: sample.atMS, el: el})
		}
	}
	return tracks
}

func jumps(tracks elementTracks) []Finding {
	frameMS := 1000 / framesPerSecond
	var out []Finding
	for _, id := range tracks.order {
		track := tracks.byID[id]
		for i := 1; i < len(track); i++ {
			a, b := track[i-1], track[i]
			ax, ay := motionCenter(a.el.Box)
			bx, by := motionCenter(b.el.Box)
			distance := jsHypot(bx-ax, by-ay)
			perFrame := distance * frameMS / math.Max(frameMS, b.atMS-a.atMS)
			if perFrame > jumpPerFrame && a.el.Opacity > visibleOpacity && b.el.Opacity > visibleOpacity {
				out = append(out, Finding{Kind: "motion_jump", Level: LevelWarning, IDs: []string{b.el.ID}, Text: fmt.Sprintf("%s salta %s%% do quadro em %s ms perto de %s s; espace os keyframes ou use uma transição se o corte é de propósito", motionName(b.el), pxNum(distance*100), pxNum(b.atMS-a.atMS), seconds(a.atMS))})
				break
			}
		}
	}
	return out
}

func excursions(tracks elementTracks) []Finding {
	var out []Finding
	for _, id := range tracks.order {
		track := tracks.byID[id]
		first := -1
		for i, entry := range track {
			if outsideFrame(entry.el.Box) {
				first = i
				break
			}
		}
		if first <= 0 {
			continue
		}
		last := first
		for last+1 < len(track) && outsideFrame(track[last+1].el.Box) {
			last++
		}
		if last+1 >= len(track) {
			continue
		}
		el := track[first].el
		out = append(out, Finding{Kind: "leaves_frame", Level: LevelWarning, IDs: []string{el.ID}, Text: fmt.Sprintf("%s sai do quadro de %s s a %s s no meio da animação e volta; mantenha o caminho dentro da tela", motionName(el), seconds(track[first].atMS), seconds(track[last].atMS))})
	}
	return out
}

func blinking(doc *studio.VideoDocument, at float64) string {
	for _, clip := range visualClips(doc) {
		opacity := 0
		if clip.Keyframes != nil {
			opacity = len(clip.Keyframes.Opacity)
		}
		if float64(clip.StartMS) <= at && at <= float64(clip.StartMS+clip.DurationMS) && opacity > 2 {
			return clip.ID
		}
	}
	return ""
}

func flashes(samples []motionSample, doc *studio.VideoDocument) []Finding {
	type change struct {
		atMS float64
		up   bool
	}
	var changes []change
	for i := 1; i < len(samples); i++ {
		a, b := samples[i-1].light, samples[i].light
		if a == nil || b == nil || math.Abs(*b-*a) < flashStep || math.Min(*a, *b) >= flashDark {
			continue
		}
		changes = append(changes, change{atMS: samples[i].atMS, up: *b > *a})
	}
	var flashTimes []float64
	for i := 1; i < len(changes); i++ {
		if changes[i].up != changes[i-1].up {
			flashTimes = append(flashTimes, changes[i].atMS)
		}
	}
	worst := 0
	for _, at := range flashTimes {
		count := 0
		for _, other := range flashTimes {
			if other >= at && other < at+1000 {
				count++
			}
		}
		worst = max(worst, count)
	}
	if worst <= flashesPerSecond {
		return nil
	}
	culprit := blinking(doc, flashTimes[0])
	ids := []string{}
	named := ""
	if culprit != "" {
		ids = []string{culprit}
		named = " (" + culprit + ")"
	}
	return []Finding{{Kind: "flash", Level: LevelError, IDs: ids, Text: fmt.Sprintf("a tela pisca %d vezes por segundo perto de %s s%s; mais de 3 flashes por segundo podem causar crises em pessoas sensíveis", worst, seconds(flashTimes[0]), named)}}
}

func crossings(samples []motionSample) []Finding {
	var keys []string
	touching := map[string][]float64{}
	seen := map[string][]float64{}
	for _, sample := range samples {
		var texts []Element
		for _, el := range sample.elements {
			if el.Layer != nil && el.Layer.Type == studio.LayerText && jsTrim(el.Layer.Text) != "" {
				texts = append(texts, el)
			}
		}
		for i, a := range texts {
			for _, b := range texts[i+1:] {
				pair := []string{a.ID, b.ID}
				sort.Strings(pair)
				key := strings.Join(pair, ",")
				seen[key] = append(seen[key], sample.atMS)
				if intersection(a.Box, b.Box) >= movingOverlap*math.Min(area(a.Box), area(b.Box)) {
					if _, known := touching[key]; !known {
						keys = append(keys, key)
					}
					touching[key] = append(touching[key], sample.atMS)
				}
			}
		}
	}
	var out []Finding
	for _, key := range keys {
		times := touching[key]
		together := seen[key]
		if containsTime(times, together[0]) || containsTime(times, together[len(together)-1]) {
			continue
		}
		out = append(out, Finding{Kind: "moving_overlap", Level: LevelWarning, IDs: strings.Split(key, ","), Text: fmt.Sprintf("os textos %s se cruzam no meio do movimento, de %s s a %s s; desencontre os tempos ou os caminhos", strings.Replace(key, ",", " e ", 1), seconds(times[0]), seconds(times[len(times)-1]))})
	}
	return out
}

func containsTime(times []float64, at float64) bool {
	for _, t := range times {
		if t == at {
			return true
		}
	}
	return false
}

func positionLanes(clip studio.Clip) [][]mediagen.Keyframe {
	if clip.Keyframes == nil {
		return [][]mediagen.Keyframe{nil, nil}
	}
	return [][]mediagen.Keyframe{clip.Keyframes.X, clip.Keyframes.Y}
}

func linearMoves(clips []studio.Clip) []Finding {
	var out []Finding
	for _, clip := range clips {
		for _, keys := range positionLanes(clip) {
			moved := false
			for i := 1; i < len(keys); i++ {
				if keys[i-1].Easing == mediagen.EaseLinear && math.Abs(keys[i].Value-keys[i-1].Value) >= linearMove && float64(keys[i].AtMS-keys[i-1].AtMS) >= linearMinMS {
					moved = true
					break
				}
			}
			if !moved {
				continue
			}
			out = append(out, Finding{Kind: "linear_motion", Level: LevelWarning, IDs: []string{clip.ID}, Text: fmt.Sprintf("%s anda %s%% do quadro com easing linear, que parece mecânico; easeOut na entrada e easeIn na saída soam naturais, a menos que seja um movimento contínuo de propósito", clip.ID, pxNum(math.Abs(keys[len(keys)-1].Value-keys[0].Value)*100))})
			break
		}
	}
	return out
}

func (v *Video) shadowTurns(clips []studio.Clip) ([]Finding, error) {
	var casts []*shadowCast
	for _, clip := range clips {
		scene, err := v.sceneAt(float64(clip.StartMS) + float64(clip.DurationMS)/2)
		if err != nil {
			return nil, err
		}
		for _, el := range scene.Elements {
			if el.ID != clip.ID {
				continue
			}
			if el.Layer != nil {
				layer := *el.Layer
				layer.ID = clip.ID
				layer.Transform = el.Transform
				casts = append(casts, shadowCasts(&layer, scene.Canvas)...)
			}
			break
		}
	}
	var out []Finding
	for _, o := range lightOutliers(casts) {
		out = append(out, Finding{Kind: "shadow_direction", Level: LevelWarning, IDs: []string{o.cast.layerID}, Text: fmt.Sprintf("a sombra de %s cai %s (%s°), contra a luz dos outros clipes, que caem %s (%s°)", o.cast.layerID, directionWord(o.cast.angle), num(rounded(o.cast.angle, 0)), directionWord(o.light), num(rounded(o.light, 0)))})
	}
	return out, nil
}

type namedLane struct {
	name string
	keys []mediagen.Keyframe
}

func listedLanes(clip studio.Clip) []namedLane {
	k := clip.Keyframes
	if k == nil {
		return nil
	}
	return []namedLane{{"x", k.X}, {"y", k.Y}, {"scale", k.Scale}, {"rotation", k.Rotation}, {"opacity", k.Opacity}, {"blur", k.Blur}}
}

func laneLine(name string, keys []mediagen.Keyframe) string {
	var easings []string
	for _, key := range keys[:len(keys)-1] {
		easings = append(easings, string(key.Easing))
	}
	first, last := keys[0], keys[len(keys)-1]
	return fmt.Sprintf("%s %s→%s de %s a %s s (%s)", name, num(rounded(first.Value, 2)), num(rounded(last.Value, 2)), seconds(float64(first.AtMS)), seconds(float64(last.AtMS)), strings.Join(uniqueStrings(easings), ", "))
}

func moveLines(clips []studio.Clip) []string {
	lines := []string{}
	for _, clip := range clips {
		var parts []string
		for _, lane := range listedLanes(clip) {
			if len(lane.keys) >= 2 {
				parts = append(parts, laneLine(lane.name, lane.keys))
			}
		}
		if clip.MotionIn != nil {
			parts = append(parts, fmt.Sprintf("entra pela %s em %s s", clip.MotionIn.Edge, seconds(float64(clip.MotionIn.DurationMS))))
		}
		if clip.MotionOut != nil {
			parts = append(parts, fmt.Sprintf("sai pela %s em %s s", clip.MotionOut.Edge, seconds(float64(clip.MotionOut.DurationMS))))
		}
		if len(parts) == 0 {
			continue
		}
		label := clip.ID
		if clip.Layer != nil && clip.Layer.Type == studio.LayerText {
			label = fmt.Sprintf("%s (texto \"%s\")", clip.ID, snippet(clip.Layer))
		}
		lines = append(lines, label+": "+strings.Join(parts, "; "))
	}
	return lines
}

func (v *Video) motionSamples() ([]motionSample, error) {
	samples := make([]motionSample, len(v.Motion))
	for i, at := range v.Motion {
		scene, err := v.sceneAt(at)
		if err != nil {
			return nil, err
		}
		samples[i] = motionSample{atMS: at, elements: scene.Elements, light: frameLight(scene)}
	}
	return samples, nil
}

func (v *Video) MotionReport() (MotionReport, error) {
	doc := v.Document
	if doc == nil || v.Motion == nil {
		return MotionReport{}, ErrIncomplete
	}
	var clips []studio.Clip
	for _, clip := range visualClips(doc) {
		if v.Window == nil || (float64(clip.StartMS) < v.Window.ToMS && float64(clip.StartMS+clip.DurationMS) > v.Window.FromMS) {
			clips = append(clips, clip)
		}
	}
	samples, err := v.motionSamples()
	if err != nil {
		return MotionReport{}, err
	}
	turns, err := v.shadowTurns(clips)
	if err != nil {
		return MotionReport{}, err
	}
	tracks := tracksOf(samples)
	variety := motionVariety(clips)
	findings := []Finding{}
	findings = append(findings, flashes(samples, doc)...)
	findings = append(findings, jumps(tracks)...)
	findings = append(findings, excursions(tracks)...)
	findings = append(findings, crossings(samples)...)
	findings = append(findings, linearMoves(clips)...)
	findings = append(findings, turns...)
	findings = append(findings, variety.findings...)
	return MotionReport{Moves: moveLines(clips), Variety: variety.line, Findings: findings}, nil
}
