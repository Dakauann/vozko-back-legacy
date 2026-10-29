package perception

import (
	"bytes"
	"encoding/json"
	"errors"

	"vozko/domain/studio"
)

var (
	ErrIncomplete = errors.New("perception: the measured facts miss a frame or field the analysis needs")
	ErrMalformed  = errors.New("perception: the measured facts could not be read")
	ErrOversized  = errors.New("perception: the measured facts did not fit the editor reply")
)

type SurfaceCanvas struct {
	Width      float64          `json:"width"`
	Height     float64          `json:"height"`
	Background string           `json:"background"`
	Gradient   *studio.Gradient `json:"gradient,omitempty"`
}

type Surface struct {
	ID     string                `json:"id,omitempty"`
	Canvas SurfaceCanvas         `json:"canvas"`
	Layers []studio.Layer        `json:"layers"`
	Texts  map[string]*TextFacts `json:"texts"`
	QR     map[string]int        `json:"qr,omitempty"`
}

type PosedElement struct {
	ID        string           `json:"id"`
	Layer     *int             `json:"layer,omitempty"`
	Media     bool             `json:"media"`
	Transform studio.Transform `json:"transform"`
	Opacity   float64          `json:"opacity"`
	Text      *TextFacts       `json:"text,omitempty"`
}

type Frame struct {
	AtMS     float64
	Elements []PosedElement
}

type MotionWindow struct {
	FromMS float64 `json:"fromMs"`
	ToMS   float64 `json:"toMs"`
}

type Video struct {
	Document   *studio.VideoDocument
	Canvas     Canvas
	Background string
	Layers     []studio.Layer
	Frames     []Frame
	Rests      map[string]float64
	Cuts       []float64
	Times      []float64
	Window     *MotionWindow
	Motion     []float64
	QR         map[string]int
	byTime     map[float64]int
}

type wireVideo struct {
	Document   json.RawMessage    `json:"document,omitempty"`
	Canvas     Canvas             `json:"canvas"`
	Background string             `json:"background"`
	Layers     []json.RawMessage  `json:"layers"`
	Frames     []wireFrame        `json:"frames"`
	Rests      map[string]float64 `json:"rests,omitempty"`
	Cuts       []float64          `json:"cuts,omitempty"`
	Times      []float64          `json:"times,omitempty"`
	Window     *MotionWindow      `json:"window,omitempty"`
	Motion     []float64          `json:"motion,omitempty"`
	QR         map[string]int     `json:"qr,omitempty"`
}

type wireFrame struct {
	AtMS     float64           `json:"atMs"`
	Elements []json.RawMessage `json:"elements"`
}

type wireSurface struct {
	ID     string                `json:"id,omitempty"`
	Canvas SurfaceCanvas         `json:"canvas"`
	Layers []json.RawMessage     `json:"layers"`
	Texts  map[string]*TextFacts `json:"texts"`
	QR     map[string]int        `json:"qr,omitempty"`
}

func strict(raw []byte, into any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil || dec.More() {
		return ErrMalformed
	}
	return nil
}

func layersOf(raw []json.RawMessage) ([]studio.Layer, error) {
	layers := make([]studio.Layer, len(raw))
	for i, item := range raw {
		if err := json.Unmarshal(item, &layers[i]); err != nil {
			return nil, ErrMalformed
		}
	}
	return layers, nil
}

func validCanvas(width, height float64) bool {
	return width > 0 && height > 0
}

func DecodeSurface(raw []byte) (*Surface, error) {
	var wire wireSurface
	if err := strict(raw, &wire); err != nil {
		return nil, err
	}
	return wire.surface()
}

func (w wireSurface) surface() (*Surface, error) {
	if !validCanvas(w.Canvas.Width, w.Canvas.Height) {
		return nil, ErrMalformed
	}
	layers, err := layersOf(w.Layers)
	if err != nil {
		return nil, err
	}
	return &Surface{ID: w.ID, Canvas: w.Canvas, Layers: layers, Texts: w.Texts, QR: w.QR}, nil
}

func (s *Surface) UnmarshalJSON(raw []byte) error {
	decoded, err := DecodeSurface(raw)
	if err != nil {
		return err
	}
	*s = *decoded
	return nil
}

func DecodeVideo(raw []byte) (*Video, error) {
	var wire wireVideo
	if err := strict(raw, &wire); err != nil {
		return nil, err
	}
	if !validCanvas(wire.Canvas.Width, wire.Canvas.Height) {
		return nil, ErrMalformed
	}
	layers, err := layersOf(wire.Layers)
	if err != nil {
		return nil, err
	}
	video := &Video{Canvas: wire.Canvas, Background: wire.Background, Layers: layers, Rests: wire.Rests, Cuts: wire.Cuts, Times: wire.Times, Window: wire.Window, Motion: wire.Motion, QR: wire.QR, byTime: map[float64]int{}}
	if len(wire.Document) > 0 {
		var doc studio.VideoDocument
		if err := json.Unmarshal(wire.Document, &doc); err != nil {
			return nil, ErrMalformed
		}
		video.Document = &doc
	}
	var previous []PosedElement
	for _, wf := range wire.Frames {
		frame := Frame{AtMS: wf.AtMS, Elements: make([]PosedElement, len(wf.Elements))}
		for i, item := range wf.Elements {
			var same int
			if err := json.Unmarshal(item, &same); err == nil {
				if same < 0 || same >= len(previous) {
					return nil, ErrMalformed
				}
				frame.Elements[i] = previous[same]
				continue
			}
			if err := strict(item, &frame.Elements[i]); err != nil {
				return nil, err
			}
			if ref := frame.Elements[i].Layer; ref != nil && (*ref < 0 || *ref >= len(layers)) {
				return nil, ErrMalformed
			}
		}
		if _, seen := video.byTime[frame.AtMS]; seen {
			return nil, ErrMalformed
		}
		video.byTime[frame.AtMS] = len(video.Frames)
		video.Frames = append(video.Frames, frame)
		previous = frame.Elements
	}
	return video, nil
}

func (v *Video) UnmarshalJSON(raw []byte) error {
	decoded, err := DecodeVideo(raw)
	if err != nil {
		return err
	}
	*v = *decoded
	return nil
}

func (v *Video) frameAt(at float64) (*Frame, error) {
	index, found := v.byTime[at]
	if !found {
		return nil, ErrIncomplete
	}
	return &v.Frames[index], nil
}

func (v *Video) sceneOf(frame *Frame) *Scene {
	return frameScene(v, frame)
}

func (v *Video) sceneAt(at float64) (*Scene, error) {
	frame, err := v.frameAt(at)
	if err != nil {
		return nil, err
	}
	return v.sceneOf(frame), nil
}
