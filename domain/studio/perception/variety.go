package perception

import (
	"fmt"
	"strings"

	"vozko/domain/mediagen"
	"vozko/domain/studio"
)

type varietyResult struct {
	line     *string
	findings []Finding
}

type travel struct {
	axis string
	word string
}

type clipMotion struct {
	id       string
	easings  []string
	entrance []string
	travel   []travel
}

var (
	entranceMS      = 1000.0
	travelMin       = 0.02
	convergentClips = 3
)

var travelWords = map[string][2]string{"horizontal": {"para a direita", "para a esquerda"}, "vertical": {"para baixo", "para cima"}}

var enterFrom = map[mediagen.MotionEdge]travel{
	mediagen.EdgeLeft:   {axis: "horizontal", word: "para a direita"},
	mediagen.EdgeRight:  {axis: "horizontal", word: "para a esquerda"},
	mediagen.EdgeTop:    {axis: "vertical", word: "para baixo"},
	mediagen.EdgeBottom: {axis: "vertical", word: "para cima"},
}

var leaveTo = map[mediagen.MotionEdge]travel{
	mediagen.EdgeLeft:   enterFrom[mediagen.EdgeRight],
	mediagen.EdgeRight:  enterFrom[mediagen.EdgeLeft],
	mediagen.EdgeTop:    enterFrom[mediagen.EdgeBottom],
	mediagen.EdgeBottom: enterFrom[mediagen.EdgeTop],
}

type familyLane struct {
	name   string
	family string
	keys   []mediagen.Keyframe
}

func plural(count int, one, many string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, one)
	}
	return fmt.Sprintf("%d %s", count, many)
}

func spoken(parts []string) string {
	if len(parts) <= 1 {
		return strings.Join(parts, "")
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " e " + parts[len(parts)-1]
}

func laneTravel(axis string, keys []mediagen.Keyframe) []travel {
	far := keys[0]
	for _, key := range keys {
		if abs(key.Value-keys[0].Value) > abs(far.Value-keys[0].Value) {
			far = key
		}
	}
	delta := far.Value - keys[0].Value
	if abs(delta) < travelMin {
		return nil
	}
	words := travelWords[axis]
	if delta > 0 {
		return []travel{{axis: axis, word: words[0]}}
	}
	return []travel{{axis: axis, word: words[1]}}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func familyLanes(clip studio.Clip) []familyLane {
	k := clip.Keyframes
	if k == nil {
		return nil
	}
	all := []familyLane{{"x", "deslize", k.X}, {"y", "deslize", k.Y}, {"scale", "escala", k.Scale}, {"rotation", "giro", k.Rotation}, {"opacity", "fade", k.Opacity}, {"blur", "foco", k.Blur}}
	var lanes []familyLane
	for _, lane := range all {
		if len(lane.keys) >= 2 {
			lanes = append(lanes, lane)
		}
	}
	return lanes
}

func motionOfClip(clip studio.Clip) *clipMotion {
	lanes := familyLanes(clip)
	var easings []string
	for _, lane := range lanes {
		for _, key := range lane.keys[:len(lane.keys)-1] {
			easings = append(easings, string(key.Easing))
		}
	}
	if clip.TransitionIn != nil {
		easings = append(easings, string(clip.TransitionIn.Easing))
	}
	if clip.TransitionOut != nil {
		easings = append(easings, string(clip.TransitionOut.Easing))
	}
	if clip.TextIn != nil {
		easings = append(easings, string(clip.TextIn.Easing))
	}
	if clip.TextOut != nil {
		easings = append(easings, string(clip.TextOut.Easing))
	}
	var entrance []string
	for _, lane := range lanes {
		if float64(lane.keys[0].AtMS) <= entranceMS {
			entrance = append(entrance, lane.family)
		}
	}
	if clip.MotionIn != nil {
		entrance = append(entrance, "deslize")
	}
	if clip.TransitionIn != nil {
		entrance = append(entrance, "transição "+string(clip.TransitionIn.Kind))
	}
	if clip.TextIn != nil {
		entrance = append(entrance, "texto "+string(clip.TextIn.Style))
	}
	if len(clip.TextAnimators) > 0 {
		entrance = append(entrance, "texto por unidade")
	}
	var travels []travel
	for _, lane := range lanes {
		switch lane.name {
		case "x":
			travels = append(travels, laneTravel("horizontal", lane.keys)...)
		case "y":
			travels = append(travels, laneTravel("vertical", lane.keys)...)
		}
	}
	if clip.MotionIn != nil {
		travels = append(travels, enterFrom[clip.MotionIn.Edge])
	}
	if clip.MotionOut != nil {
		travels = append(travels, leaveTo[clip.MotionOut.Edge])
	}
	if len(easings) == 0 && len(entrance) == 0 && len(travels) == 0 {
		return nil
	}
	return &clipMotion{id: clip.ID, easings: uniqueStrings(easings), entrance: uniqueStrings(entrance), travel: travels}
}

func convergence(motions []*clipMotion, easings, signatures []string, travels []travel) []Finding {
	if len(motions) < convergentClips {
		return nil
	}
	travellers := 0
	for _, m := range motions {
		if len(m.travel) > 0 {
			travellers++
		}
	}
	var axes []string
	for _, t := range travels {
		axes = append(axes, t.axis)
	}
	axes = uniqueStrings(axes)
	var clauses []string
	if len(signatures) == 1 && signatures[0] != "" {
		clauses = append(clauses, fmt.Sprintf("entram do mesmo jeito (%s)", signatures[0]))
	}
	if len(easings) == 1 {
		clauses = append(clauses, fmt.Sprintf("usam um só easing (%s)", easings[0]))
	}
	if travellers >= convergentClips && len(axes) == 1 {
		clauses = append(clauses, fmt.Sprintf("têm todo o trajeto no eixo %s", axes[0]))
	}
	if len(clauses) == 0 {
		return nil
	}
	ids := make([]string, len(motions))
	for i, m := range motions {
		ids[i] = m.id
	}
	return []Finding{{Kind: "motion_variety", Level: LevelWarning, IDs: ids, Text: fmt.Sprintf("os %d clipes animados %s; isso tende a ler como preset, pode ser intencional", len(motions), spoken(clauses))}}
}

func motionVariety(clips []studio.Clip) varietyResult {
	var motions []*clipMotion
	for _, clip := range clips {
		if m := motionOfClip(clip); m != nil {
			motions = append(motions, m)
		}
	}
	if len(motions) == 0 {
		return varietyResult{}
	}
	var allEasings, allSignatures, words []string
	var travels []travel
	for _, m := range motions {
		allEasings = append(allEasings, m.easings...)
		allSignatures = append(allSignatures, strings.Join(m.entrance, " + "))
		travels = append(travels, m.travel...)
	}
	easings := uniqueStrings(allEasings)
	signatures := uniqueStrings(allSignatures)
	var kinds []string
	for _, s := range signatures {
		if s != "" {
			kinds = append(kinds, s)
		}
	}
	for _, t := range travels {
		words = append(words, t.word)
	}
	words = uniqueStrings(words)
	first := fmt.Sprintf("variedade de movimento em %s: %s", plural(len(motions), "clipe animado", "clipes animados"), plural(len(easings), "easing", "easings"))
	if len(easings) > 0 {
		first += fmt.Sprintf(" (%s)", strings.Join(easings, ", "))
	}
	second := plural(len(kinds), "tipo de entrada", "tipos de entrada")
	if len(kinds) > 0 {
		second += fmt.Sprintf(" (%s)", strings.Join(kinds, ", "))
	}
	third := "nenhum trajeto"
	if len(words) > 0 {
		third = fmt.Sprintf("%s (%s)", plural(len(words), "direção de trajeto", "direções de trajeto"), strings.Join(words, ", "))
	}
	text := strings.Join([]string{first, second, third}, "; ")
	return varietyResult{line: &text, findings: convergence(motions, easings, signatures, travels)}
}
