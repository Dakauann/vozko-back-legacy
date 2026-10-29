package studiotools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"vozko/domain/copilot"
	"vozko/domain/studio"
	"vozko/domain/tools"
	"vozko/usecases/copilot/copilottools"
)

func sampleValue(p tools.Parameter) interface{} {
	switch p.Type {
	case "string":
		return "x"
	case "number":
		return 0.5
	case "integer":
		return 1.0
	case "boolean":
		return true
	case "array":
		return []interface{}{sampleItem(p.Items)}
	case "object":
		return sampleObject(p.Items.Properties)
	}
	return nil
}

func sampleItem(items *tools.ParameterItems) interface{} {
	if items == nil {
		return nil
	}
	switch items.Type {
	case "object":
		return sampleObject(items.Properties)
	case "array":
		return []interface{}{sampleItem(items.Items)}
	case "number":
		return 0.5
	case "integer":
		return 1.0
	case "boolean":
		return true
	}
	return "x"
}

func sampleObject(props map[string]tools.Parameter) map[string]interface{} {
	out := make(map[string]interface{}, len(props))
	for name, p := range props {
		out[name] = sampleValue(p)
	}
	return out
}

func TestEveryFieldTheEditorDocumentsPassesTheFieldGateOnEveryOperation(t *testing.T) {
	for _, kind := range studioKinds {
		edit := studioEditFor(kind)
		everything := sampleObject(edit.Definition().Parameters["operations"].Items.Properties)
		for _, op := range edit.allowed {
			everything["op"] = op
			if err := edit.refuseUnknownFields(map[string]interface{}{"operations": []interface{}{everything}}); err != nil {
				t.Fatalf("%s %s: a documented field was refused: %v", kind, op, err)
			}
		}
	}
}

func TestAnUnknownEditFieldIsRefusedWithTheClosestFamily(t *testing.T) {
	cases := []struct {
		kind    copilot.StudioKind
		op      map[string]interface{}
		field   string
		meant   string
		guideOf string
	}{
		{copilot.StudioImage, map[string]interface{}{"op": "style_text", "layer_id": "l1", "fontsize": 0.1}, `"fontsize"`, `"font_size"`, "font_size"},
		{copilot.StudioImage, map[string]interface{}{"op": "update_layer", "layer_id": "l1", "shadow": map[string]interface{}{"colr": "#000000"}}, `"shadow.colr"`, `"color"`, "shadow"},
		{copilot.StudioVideo, map[string]interface{}{"op": "animate", "clip_id": "c1", "property": "x", "keys": []interface{}{map[string]interface{}{"at_ms": 0.0, "valeu": 1.0}}}, `"keys.valeu"`, `"value"`, "keys"},
		{copilot.StudioVideo, map[string]interface{}{"op": "add_text", "text": "Oi", "glowing_neon": true}, `"glowing_neon"`, "", ""},
	}
	for _, c := range cases {
		edit := studioEditFor(c.kind)
		err := edit.Validate(context.Background(), copilot.Context{View: studioViewOf(c.kind)}, map[string]interface{}{"operations": []interface{}{map[string]interface{}{"op": "select"}, c.op}})
		if !errors.Is(err, copilottools.ErrInvalidArgs) {
			t.Fatalf("%s %v: an unknown field must be refused as invalid, got %v", c.kind, c.op, err)
		}
		message := err.Error()
		if !strings.Contains(message, "operação 2: o campo "+c.field+" não existe") || !strings.Contains(message, "a família mais próxima é ") || !strings.Contains(message, "studio_op_guide") {
			t.Errorf("%s: the refusal does not name the field, the closest family and the guide: %s", c.kind, message)
		}
		if c.meant != "" && !strings.Contains(message, "você quis dizer "+c.meant) {
			t.Errorf("%s: the refusal should point to %s: %s", c.kind, c.meant, message)
		}
		if c.guideOf != "" {
			family, _ := edit.names().resolve(c.guideOf)
			if !strings.Contains(message, "a família mais próxima é "+family) {
				t.Errorf("%s: the refusal should send Elo to the %s family: %s", c.kind, family, message)
			}
		}
	}
}

func TestEveryUnknownFieldOfTheBatchIsListedInOneRefusal(t *testing.T) {
	edit := studioEditFor(copilot.StudioImage)
	err := edit.refuseUnknownFields(map[string]interface{}{"operations": []interface{}{
		map[string]interface{}{"op": "add_text", "text": "a", "colour": "#ffffff"},
		map[string]interface{}{"op": "add_text", "text": "b"},
		map[string]interface{}{"op": "add_shape", "shape": "rect", "radius_px": 4.0, "fil": "#000000"},
	}})
	if err == nil {
		t.Fatal("unknown fields passed the gate")
	}
	for _, want := range []string{`operação 1: o campo "colour"`, `operação 3: o campo "fil"`, `operação 3: o campo "radius_px"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal misses %s: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "operação 2") {
		t.Errorf("a clean operation was blamed: %v", err)
	}
}

func TestAnUnknownEditFieldNeverReachesTheEditor(t *testing.T) {
	editor := &fakeEditor{}
	edit := NewStudioEditVideoTool(studioDeps(studio.KindVideo, nil, nil))
	res := edit.Execute(context.Background(), studioSession(copilot.StudioVideo, editor), map[string]interface{}{"operations": []interface{}{
		map[string]interface{}{"op": "add_text", "text": "Oferta", "unknown": "drop"},
	}})
	if res.Status != copilot.StatusError || !strings.Contains(res.Message, `"unknown"`) {
		t.Fatalf("result = %+v", res)
	}
	if len(editor.commands) > 0 {
		t.Fatal("a batch with an unknown field reached the editor")
	}
}

func TestTheFieldGateReadsTypedArgumentsToo(t *testing.T) {
	edit := studioEditFor(copilot.StudioImage)
	err := edit.refuseUnknownFields(map[string]interface{}{"operations": []map[string]interface{}{{"op": "add_text", "text": "a", "colour": "#ffffff"}}})
	if err == nil || !strings.Contains(err.Error(), `"colour"`) {
		t.Fatalf("typed operations slipped past the gate: %v", err)
	}
}
