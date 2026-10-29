package perception

import (
	"encoding/json"
	"fmt"

	"vozko/domain/studio"
)

const (
	MaxScenes        = 100
	MarksUnavailable = "as marcas dos problemas não puderam ser desenhadas nesta imagem; os relatórios em texto continuam valendo"
)

type Facts struct {
	Approximate bool              `json:"approximate"`
	View        View              `json:"view,omitempty"`
	Artboards   []Surface         `json:"artboards,omitempty"`
	Multiple    bool              `json:"multiple,omitempty"`
	Surface     *Surface          `json:"surface,omitempty"`
	Legibility  []LegibilityEntry `json:"legibility,omitempty"`
	Targets     []GuideTarget     `json:"targets,omitempty"`
	Unmarked    bool              `json:"unmarked,omitempty"`
	Oversized   bool              `json:"oversized,omitempty"`
	Video       *Video            `json:"video,omitempty"`
}

type GuideTarget struct {
	ID        string           `json:"id"`
	Transform studio.Transform `json:"transform"`
}

type GuideData struct {
	Angles []AngleGroup `json:"angles"`
	Notes  []string     `json:"notes"`
}

type ShapesRequest struct {
	Approximate bool      `json:"approximate"`
	Layers      Layers    `json:"layers"`
	Surfaces    []Surface `json:"surfaces,omitempty"`
	Video       *Video    `json:"video,omitempty"`
}

func DecodeFacts(raw []byte) (*Facts, error) {
	var facts Facts
	if err := strict(raw, &facts); err != nil {
		return nil, err
	}
	return &facts, nil
}

func DecodeShapesRequest(raw []byte) (*ShapesRequest, error) {
	var request ShapesRequest
	if err := strict(raw, &request); err != nil {
		return nil, err
	}
	scenes := len(request.Surfaces)
	if request.Video != nil {
		scenes += len(request.Video.Times)
	}
	if (len(request.Surfaces) > 0) == (request.Video != nil) || scenes == 0 || scenes > MaxScenes || (!request.Layers.Depth && !request.Layers.Layout) {
		return nil, ErrMalformed
	}
	return &request, nil
}

func (e *LegibilityEntry) UnmarshalJSON(raw []byte) error {
	var wire struct {
		Layer    json.RawMessage `json:"layer"`
		Plain    *Contrast       `json:"plain"`
		Outlined *Contrast       `json:"outlined,omitempty"`
	}
	if err := strict(raw, &wire); err != nil {
		return err
	}
	var layer studio.Layer
	if err := json.Unmarshal(wire.Layer, &layer); err != nil {
		return ErrMalformed
	}
	*e = LegibilityEntry{Layer: layer, Plain: wire.Plain, Outlined: wire.Outlined}
	return nil
}

func (f *Facts) certain() bool {
	return !f.Approximate
}

func (f *Facts) usable() error {
	if f.Oversized {
		return ErrOversized
	}
	return nil
}

func Checks(f *Facts) ([]string, error) {
	if err := f.usable(); err != nil {
		return nil, err
	}
	if f.Video != nil {
		return VideoChecks(f.Video, f.certain())
	}
	if f.Artboards == nil {
		return nil, ErrIncomplete
	}
	lines := []string{}
	for i := range f.Artboards {
		surface := &f.Artboards[i]
		found, err := ImageChecks(surface, f.certain())
		if err != nil {
			return nil, err
		}
		for _, line := range append(found, ImageQRChecks(surface)...) {
			if f.Multiple {
				line = fmt.Sprintf("prancheta %s: %s", surface.ID, line)
			}
			lines = append(lines, line)
		}
	}
	return lines, nil
}

func ArtboardAlerts(f *Facts) ([][]string, error) {
	if err := f.usable(); err != nil {
		return nil, err
	}
	alerts := make([][]string, len(f.Artboards))
	for i := range f.Artboards {
		found, err := ImageLook(ViewRender, &f.Artboards[i], f.certain())
		if err != nil {
			return nil, err
		}
		if lines, ok := found.Data["alerts"].([]string); ok {
			alerts[i] = lines
		}
	}
	return alerts, nil
}

func VideoAlerts(f *Facts) (map[string]any, error) {
	if err := f.usable(); err != nil {
		return nil, err
	}
	if f.Video == nil {
		return nil, ErrIncomplete
	}
	found, err := VideoLook(ViewRender, f.Video, f.certain())
	if err != nil {
		return nil, err
	}
	return found.Data, nil
}

func Look(f *Facts) (map[string]any, error) {
	data, err := look(f)
	if err == nil && f.Unmarked {
		data["marks_unavailable"] = MarksUnavailable
	}
	return data, err
}

func look(f *Facts) (map[string]any, error) {
	if err := f.usable(); err != nil {
		return nil, err
	}
	view := f.View
	if view == "" {
		view = ViewRender
	}
	if f.Video != nil {
		found, err := VideoLook(view, f.Video, f.certain())
		if err != nil {
			return nil, err
		}
		return found.Data, nil
	}
	if f.Surface == nil {
		return nil, ErrIncomplete
	}
	found, err := ImageLook(view, f.Surface, f.certain())
	if err != nil {
		return nil, err
	}
	data := found.Data
	if view != ViewWireframe {
		if lines := Legibility(f.Surface.Canvas.Height, f.Legibility); len(lines) > 0 {
			data["legibility"] = lines
		}
	}
	if view == ViewGuides {
		data["guides"] = Guides(f.Targets)
	}
	return data, nil
}

func Shapes(r *ShapesRequest) ([][]Shape, error) {
	certain := !r.Approximate
	var out [][]Shape
	for i := range r.Surfaces {
		shapes, err := ImageShapes(&r.Surfaces[i], r.Layers, certain)
		if err != nil {
			return nil, err
		}
		out = append(out, shapes)
	}
	if r.Video != nil {
		for _, at := range r.Video.Times {
			shapes, err := FrameShapes(r.Video, at, r.Layers, certain)
			if err != nil {
				return nil, err
			}
			out = append(out, shapes)
		}
	}
	return out, nil
}
