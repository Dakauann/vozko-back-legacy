package studiotools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"vozko/domain/copilot"
	"vozko/domain/studio"
)

func bigVideoRead(alerts []string) map[string]interface{} {
	var tracks []interface{}
	for t := 0; t < 4; t++ {
		var clips []interface{}
		for c := 0; c < 12; c++ {
			clips = append(clips, map[string]interface{}{
				"id": fmt.Sprintf("c-%d-%d", t, c), "type": "text", "start_ms": c * 1000, "duration_ms": 1000, "end_ms": (c + 1) * 1000,
				"text":      strings.Repeat("Oferta de verão imperdível ", 6),
				"box":       map[string]interface{}{"x": 0.5, "y": 0.5, "w": 0.4, "h": 0.1},
				"style":     map[string]interface{}{"font_id": "inter", "font_size": 0.08, "fill": "#ffffff", "shadow": map[string]interface{}{"color": "#00000080", "blur": 6, "y": 2}},
				"keyframes": map[string]interface{}{"x": []interface{}{map[string]interface{}{"at_ms": 0, "value": 0.2, "easing": "easeOut"}, map[string]interface{}{"at_ms": 400, "value": 0.5, "easing": "easeOut"}}},
				"animated":  []interface{}{"x"},
			})
		}
		tracks = append(tracks, map[string]interface{}{"id": fmt.Sprintf("t-%d", t), "index": t, "kind": "visual", "clips": clips,
			"transitions": []interface{}{map[string]interface{}{"id": fmt.Sprintf("tr-%d", t), "from": fmt.Sprintf("c-%d-0", t), "to": fmt.Sprintf("c-%d-1", t), "kind": "fade", "duration_ms": 300, "window_ms": []interface{}{850, 1150}, "alignment": "center", "easing": "easeInOut"}}})
	}
	read := map[string]interface{}{"kind": "video", "aspect": "story", "size": "1080x1920", "background": "#000000", "duration_ms": 12000, "playhead_ms": 0, "selection": []interface{}{}, "tracks": tracks, "markers": []interface{}{}, "jobs": []interface{}{},
		"changes_by_user": map[string]interface{}{"added": []interface{}{"c-2-3"}, "removed": []interface{}{"c-9"}, "changed": []interface{}{"c-1-1"}}}
	if alerts != nil {
		read["alerts"] = alerts
	}
	return read
}

func bigImageRead() map[string]interface{} {
	var boards []interface{}
	for b := 0; b < 3; b++ {
		var layers []interface{}
		for l := 0; l < 20; l++ {
			layers = append(layers, map[string]interface{}{"id": fmt.Sprintf("l-%d-%d", b, l), "z": l, "type": "text", "text": strings.Repeat("Promoção ", 20),
				"box":   map[string]interface{}{"x": 0.5, "y": 0.5, "w": 0.4, "h": 0.1},
				"style": map[string]interface{}{"font_id": "inter", "font_size": 0.08, "fill": "#ffffff", "gradient": map[string]interface{}{"kind": "linear", "angle": 90, "from": "#ff0000", "to": "#0000ff"}}})
		}
		boards = append(boards, map[string]interface{}{"id": fmt.Sprintf("a-%d", b), "name": "Feed", "width": 1080, "height": 1080, "x": b * 1200, "y": 0, "background": "#ffffff", "layers": layers, "groups": []interface{}{},
			"alerts": []interface{}{"erro: texto l-0 sai da prancheta", "erro: texto l-0 sai da prancheta", "aviso: sombra fraca"}})
	}
	return map[string]interface{}{"kind": "image", "active_artboard": "a-0", "selection": []interface{}{}, "artboards": boards, "jobs": []interface{}{}}
}

func readWith(t *testing.T, kind copilot.StudioKind, doc map[string]interface{}, args map[string]interface{}) (copilot.Result, map[string]interface{}, *fakeEditor) {
	t.Helper()
	raw, _ := json.Marshal(doc)
	editor := &fakeEditor{replies: map[copilot.ScreenCommandName]copilot.ScreenReply{copilot.ScreenRead: {OK: true, Data: raw}}}
	projectKind := studio.KindVideo
	if kind == copilot.StudioImage {
		projectKind = studio.KindImage
	}
	res := NewStudioReadTool(studioDeps(projectKind, nil, nil)).Execute(context.Background(), studioSession(kind, editor), args)
	var data map[string]interface{}
	if res.Status == copilot.StatusOK {
		if err := json.Unmarshal([]byte(renderJSON(res.Data)), &data); err != nil {
			t.Fatal(err)
		}
	}
	return res, data, editor
}

func TestABigProjectIsReadAsAnOutlineWithEveryIDAndFarFewerCharacters(t *testing.T) {
	doc := bigVideoRead(nil)
	full := renderJSON(doc)
	res, data, _ := readWith(t, copilot.StudioVideo, doc, nil)
	if res.Status != copilot.StatusOK {
		t.Fatalf("result = %+v", res)
	}
	outline := renderJSON(res.Data)
	if len(outline)*2 > len(full) {
		t.Fatalf("the outline has %d characters for a %d character document; it should be under half", len(outline), len(full))
	}
	for _, want := range []string{`"c-3-11"`, `"t-2"`, `"tr-1"`, `"start_ms"`, `"box"`, `"changes_by_user"`} {
		if !strings.Contains(outline, want) {
			t.Errorf("the outline lost %s", want)
		}
	}
	for _, dropped := range []string{`"keyframes"`, `"shadow"`, `"font_id"`} {
		if strings.Contains(outline, dropped) {
			t.Errorf("the outline still carries %s", dropped)
		}
	}
	if note, _ := data["read"].(string); !strings.Contains(note, "esboço") || !strings.Contains(note, "ids") || !strings.Contains(note, "track_id") {
		t.Errorf("the outline does not say how to read the rest: %q", note)
	}
}

func TestASmallProjectIsReadWhole(t *testing.T) {
	doc := map[string]interface{}{"kind": "video", "tracks": []interface{}{map[string]interface{}{"id": "t-0", "kind": "visual", "clips": []interface{}{map[string]interface{}{"id": "c-1", "type": "text", "keyframes": map[string]interface{}{"x": []interface{}{}}}}}}}
	res, data, _ := readWith(t, copilot.StudioVideo, doc, nil)
	if res.Status != copilot.StatusOK || !strings.Contains(renderJSON(res.Data), `"keyframes"`) || data["read"] != nil {
		t.Fatalf("a small project must come whole, got %s", renderJSON(res.Data))
	}
}

func TestRangedReadsBringTheWholeItemsAsked(t *testing.T) {
	res, data, editor := readWith(t, copilot.StudioVideo, bigVideoRead(nil), map[string]interface{}{"ids": []interface{}{"c-1-4", "tr-2", "nada"}, "detail": "full"})
	if res.Status != copilot.StatusOK {
		t.Fatalf("result = %+v", res)
	}
	cmd, _ := editor.sent(copilot.ScreenRead)
	if renderJSON(cmd.Args) != `{"detail":"full"}` {
		t.Fatalf("the editor gets only the detail, got %s", renderJSON(cmd.Args))
	}
	items, _ := data["items"].([]interface{})
	if len(items) != 2 || !strings.Contains(renderJSON(items), `"keyframes"`) || !strings.Contains(renderJSON(items), `"in":"t-1"`) {
		t.Fatalf("items = %s", renderJSON(items))
	}
	if renderJSON(data["missing"]) != `["nada"]` || data["tracks"] != nil || data["duration_ms"] == nil {
		t.Fatalf("a ranged read keeps the project facts, drops the rest and names what is missing: %s", renderJSON(res.Data))
	}
}

func TestATrackOrArtboardIsReadWhole(t *testing.T) {
	_, video, _ := readWith(t, copilot.StudioVideo, bigVideoRead(nil), map[string]interface{}{"track_id": "t-3"})
	if track := renderJSON(video["track"]); !strings.Contains(track, `"c-3-7"`) || !strings.Contains(track, `"keyframes"`) || strings.Contains(track, `"c-2-7"`) {
		t.Fatalf("track = %s", track)
	}
	_, image, _ := readWith(t, copilot.StudioImage, bigImageRead(), map[string]interface{}{"artboard_id": "a-2"})
	if board := renderJSON(image["artboard"]); !strings.Contains(board, `"l-2-19"`) || !strings.Contains(board, `"gradient"`) || strings.Contains(board, `"l-1-0"`) {
		t.Fatalf("artboard = %s", board)
	}
	if res, _, _ := readWith(t, copilot.StudioVideo, bigVideoRead(nil), map[string]interface{}{"track_id": "t-9"}); res.Status != copilot.StatusError {
		t.Fatalf("an unknown track must be refused, got %+v", res)
	}
}

func TestTheChangesReadBringsWhatTheUserChanged(t *testing.T) {
	res, data, _ := readWith(t, copilot.StudioVideo, bigVideoRead(nil), map[string]interface{}{"changes": true})
	if res.Status != copilot.StatusOK {
		t.Fatalf("result = %+v", res)
	}
	items := renderJSON(data["items"])
	if !strings.Contains(items, `"c-2-3"`) || !strings.Contains(items, `"c-1-1"`) || strings.Contains(items, `"c-0-0"`) || !strings.Contains(renderJSON(data["changes_by_user"]), `"c-9"`) {
		t.Fatalf("changes read = %s", renderJSON(res.Data))
	}
	quiet := bigVideoRead(nil)
	delete(quiet, "changes_by_user")
	_, calm, _ := readWith(t, copilot.StudioVideo, quiet, map[string]interface{}{"changes": true})
	if note, _ := calm["read"].(string); !strings.Contains(note, "nada mudou") {
		t.Fatalf("an unchanged project says so, got %s", renderJSON(calm))
	}
}

func TestReadModesAreOneAtATimeAndFollowTheEditor(t *testing.T) {
	for name, c := range map[string]struct {
		kind copilot.StudioKind
		args map[string]interface{}
	}{
		"two modes":           {copilot.StudioVideo, map[string]interface{}{"ids": []interface{}{"c-1"}, "changes": true}},
		"artboard on a video": {copilot.StudioVideo, map[string]interface{}{"artboard_id": "a-1"}},
		"track on an image":   {copilot.StudioImage, map[string]interface{}{"track_id": "t-1"}},
	} {
		res, _, editor := readWith(t, c.kind, bigVideoRead(nil), c.args)
		if res.Status != copilot.StatusError || len(editor.commands) > 0 {
			t.Errorf("%s: must be refused before the editor, got %+v", name, res)
		}
	}
}

func TestAlertsAreMergedAcrossFramesAndCappedLikeTheDesignChecks(t *testing.T) {
	var alerts []string
	for i := 0; i < 5; i++ {
		alerts = append(alerts, fmt.Sprintf("erro: em %d s: texto c-1 sai da tela", i))
	}
	for i := 0; i < 20; i++ {
		alerts = append(alerts, fmt.Sprintf("aviso: em 2 s: clipe c-%d pisca", i), fmt.Sprintf("aviso: em 3 s: clipe c-%d pisca", i))
	}
	_, data, _ := readWith(t, copilot.StudioVideo, bigVideoRead(alerts), nil)
	got, _ := data["alerts"].([]interface{})
	if len(got) != studioMaxCheckLines+1 {
		t.Fatalf("alerts = %s", renderJSON(got))
	}
	if got[0] != "erro: em 0, 1, 2, 3 e 4 s: texto c-1 sai da tela" || got[1] != "aviso: em 2 e 3 s: clipe c-0 pisca" {
		t.Fatalf("frames were not merged: %s", renderJSON(got))
	}
	if got[studioMaxCheckLines] != fmt.Sprintf("e mais %d pontos parecidos", 21-studioMaxCheckLines) {
		t.Fatalf("the cap line is %q", got[studioMaxCheckLines])
	}
	_, image, _ := readWith(t, copilot.StudioImage, bigImageRead(), map[string]interface{}{"artboard_id": "a-0"})
	if board := image["artboard"].(map[string]interface{}); renderJSON(board["alerts"]) != `["erro: texto l-0 sai da prancheta","aviso: sombra fraca"]` {
		t.Fatalf("artboard alerts = %s", renderJSON(board["alerts"]))
	}
}
