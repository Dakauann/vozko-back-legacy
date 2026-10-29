package perception

import (
	"errors"
	"fmt"
	"regexp"

	"vozko/domain/studio"
)

type View string

const (
	ViewRender    View = "render"
	ViewWireframe View = "wireframe"
	ViewGuides    View = "guides"
	ViewDepth     View = "depth"
)

var ErrUnknownView = errors.New("perception: unknown look view")

func (v View) known() bool {
	return v == ViewRender || v == ViewWireframe || v == ViewGuides || v == ViewDepth
}

type Layers struct {
	Depth  bool `json:"depth"`
	Layout bool `json:"layout"`
}

type DepthData struct {
	Light    *string  `json:"light"`
	Shadows  []string `json:"shadows"`
	Covered  []string `json:"covered"`
	Findings []string `json:"findings"`
}

type LayoutData struct {
	Margins  string   `json:"margins"`
	Levels   *string  `json:"levels"`
	Reading  *string  `json:"reading"`
	Balance  string   `json:"balance"`
	Findings []string `json:"findings"`
}

type sceneReport struct {
	report   any
	findings []Finding
	shapes   []Shape
}

type LookResult struct {
	Data   map[string]any
	Shapes []Shape
}

type VideoLookResult struct {
	Data   map[string]any
	Shapes [][]Shape
}

type MotionData struct {
	Moves    []string `json:"moves"`
	Variety  *string  `json:"variety,omitempty"`
	Findings []string `json:"findings"`
}

func depthData(report DepthReport) DepthData {
	return DepthData{Light: report.Light, Shadows: report.Shadows, Covered: report.Covered, Findings: findingLines(report.Findings)}
}

func layoutData(report LayoutReport) LayoutData {
	return LayoutData{Margins: report.Margins, Levels: report.Levels, Reading: report.Reading, Balance: report.Balance, Findings: findingLines(report.Findings)}
}

func reportOf(view View, scene *Scene, certain bool) (sceneReport, error) {
	switch view {
	case ViewWireframe:
		return sceneReport{findings: []Finding{}, shapes: []Shape{}}, nil
	case ViewDepth:
		depth := sceneDepth(scene)
		return sceneReport{report: depthData(depth), findings: depth.Findings, shapes: depth.Shapes}, nil
	}
	if err := scene.measured(); err != nil {
		return sceneReport{}, err
	}
	layout := sceneLayout(scene, certain)
	if view == ViewGuides {
		return sceneReport{report: layoutData(layout), findings: layout.Findings, shapes: layout.Shapes}, nil
	}
	return sceneReport{findings: append(sceneDepth(scene).Findings, layout.Findings...), shapes: []Shape{}}, nil
}

func reportKey(view View) string {
	switch view {
	case ViewDepth:
		return "depth"
	case ViewGuides:
		return "layout"
	}
	return ""
}

func ImageLook(view View, surface *Surface, certain bool) (LookResult, error) {
	if !view.known() {
		return LookResult{}, ErrUnknownView
	}
	found, err := reportOf(view, imageScene(surface), certain)
	if err != nil {
		return LookResult{}, err
	}
	if key := reportKey(view); key != "" {
		return LookResult{Data: map[string]any{key: found.report}, Shapes: found.shapes}, nil
	}
	data := map[string]any{}
	if view == ViewRender {
		if alerts := findingLines(found.findings); len(alerts) > 0 {
			data["alerts"] = alerts
		}
	}
	return LookResult{Data: data, Shapes: found.shapes}, nil
}

var levelPrefix = regexp.MustCompile(`^(erro|aviso): `)

func framed(report any, at float64) map[string]any {
	entry := map[string]any{"at_ms": at}
	switch r := report.(type) {
	case DepthData:
		entry["light"], entry["shadows"], entry["covered"], entry["findings"] = r.Light, r.Shadows, r.Covered, r.Findings
	case LayoutData:
		entry["margins"], entry["levels"], entry["reading"], entry["balance"], entry["findings"] = r.Margins, r.Levels, r.Reading, r.Balance, r.Findings
	}
	return entry
}

func VideoLook(view View, video *Video, certain bool) (VideoLookResult, error) {
	if !view.known() {
		return VideoLookResult{}, ErrUnknownView
	}
	type frameReport struct {
		at float64
		sceneReport
	}
	frames := make([]frameReport, len(video.Times))
	for i, at := range video.Times {
		scene, err := video.sceneAt(at)
		if err != nil {
			return VideoLookResult{}, err
		}
		found, err := reportOf(view, scene, certain)
		if err != nil {
			return VideoLookResult{}, err
		}
		frames[i] = frameReport{at: at, sceneReport: found}
	}
	motion, err := video.MotionReport()
	if err != nil {
		return VideoLookResult{}, err
	}
	data := map[string]any{}
	if key := reportKey(view); key != "" {
		entries := make([]map[string]any, len(frames))
		for i, f := range frames {
			entries[i] = framed(f.report, f.at)
		}
		data[key] = entries
	}
	if video.Window != nil {
		data["motion_report"] = MotionData{Moves: motion.Moves, Variety: motion.Variety, Findings: findingLines(motion.Findings)}
	}
	var frameAlerts []string
	if view == ViewRender {
		for _, f := range frames {
			stamp := fmt.Sprintf("${1}: em %s s: ", num(rounded(f.at/1000, 1)))
			for _, alert := range findingLines(f.findings) {
				frameAlerts = append(frameAlerts, levelPrefix.ReplaceAllString(alert, stamp))
			}
		}
	}
	alerts := frameAlerts
	if video.Window == nil {
		alerts = append(findingLines(motion.Findings), frameAlerts...)
	}
	if len(alerts) > 0 {
		data["alerts"] = alerts
	}
	shapes := make([][]Shape, len(frames))
	for i, f := range frames {
		shapes[i] = f.shapes
	}
	return VideoLookResult{Data: data, Shapes: shapes}, nil
}

func sceneShapes(scene *Scene, layers Layers, certain bool) ([]Shape, error) {
	shapes := []Shape{}
	if layers.Depth {
		shapes = append(shapes, sceneDepth(scene).Shapes...)
	}
	if layers.Layout {
		if err := scene.measured(); err != nil {
			return nil, err
		}
		shapes = append(shapes, sceneLayout(scene, certain).Shapes...)
	}
	return shapes, nil
}

func ImageShapes(surface *Surface, layers Layers, certain bool) ([]Shape, error) {
	return sceneShapes(imageScene(surface), layers, certain)
}

func FrameShapes(video *Video, at float64, layers Layers, certain bool) ([]Shape, error) {
	scene, err := video.sceneAt(at)
	if err != nil {
		return nil, err
	}
	return sceneShapes(scene, layers, certain)
}

func ImageLayout(surface *Surface, certain bool) (LayoutReport, error) {
	scene := imageScene(surface)
	if err := scene.measured(); err != nil {
		return LayoutReport{}, err
	}
	return sceneLayout(scene, certain), nil
}

func ImageDepth(surface *Surface) DepthReport {
	return sceneDepth(imageScene(surface))
}

func FrameLayout(video *Video, at float64, certain bool) (LayoutReport, error) {
	scene, err := video.sceneAt(at)
	if err != nil {
		return LayoutReport{}, err
	}
	if err := scene.measured(); err != nil {
		return LayoutReport{}, err
	}
	return sceneLayout(scene, certain), nil
}

func FrameDepth(video *Video, at float64) (DepthReport, error) {
	scene, err := video.sceneAt(at)
	if err != nil {
		return DepthReport{}, err
	}
	return sceneDepth(scene), nil
}

type Contrast struct {
	Low     float64 `json:"low"`
	Typical float64 `json:"typical"`
}

type LegibilityEntry struct {
	Layer    studio.Layer `json:"layer"`
	Plain    *Contrast    `json:"plain"`
	Outlined *Contrast    `json:"outlined,omitempty"`
}

func Legibility(canvasHeight float64, entries []LegibilityEntry) []string {
	measured := 0
	lines := []string{}
	for i := range entries {
		entry := &entries[i]
		layer := &entry.Layer
		if layer.Type != studio.LayerText || jsTrim(layer.Text) == "" || (layer.Highlight != nil && parseColor(layer.Highlight.Color) != nil) {
			continue
		}
		if paintOf(layer) == nil {
			continue
		}
		fontPx := layer.FontSize * canvasHeight
		contrast := entry.Plain
		if outlineInk(layer, fontPx) != nil {
			contrast = entry.Outlined
		}
		if contrast == nil {
			continue
		}
		measured++
		needed := neededContrast(layer, fontPx, canvasHeight)
		if contrast.Low >= needed {
			continue
		}
		level := LevelWarning
		if contrast.Typical < needed {
			level = LevelError
		}
		shadow := ""
		if layer.Shadow != nil || len(layer.Effects) > 0 {
			shadow = " (a sombra do texto não entra na medida)"
		}
		lines = append(lines, fmt.Sprintf("%s: o texto %s (\"%s\") lê mal sobre o que está atrás: medido nos pixels, %s:1 nos 10%% piores pontos e %s:1 no típico, precisa de %s:1%s; uma caixa atrás, um véu escuro, um contorno grosso ou outra cor resolvem", level, layer.ID, snippet(layer), num(contrast.Low), num(contrast.Typical), num(needed), shadow))
	}
	if len(lines) > 0 || measured == 0 {
		return lines
	}
	noun := "textos"
	if measured == 1 {
		noun = "texto"
	}
	return []string{fmt.Sprintf("ok: contraste medido nos pixels de %d %s, todos acima do mínimo", measured, noun)}
}
