package studiotools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"vozko/domain/copilot"
	"vozko/domain/studio"
	studio_usecase "vozko/usecases/studio"
)

const tinyText = `{"id":"t","type":"text","text":"Oferta","fontSize":0.015,"fill":"#111111","transform":{"x":0.5,"y":0.5,"w":0.8,"h":0.14,"rotation":0,"opacity":1}}`

const tinyTextFacts = `{"t":{"paragraphs":[["Oferta"]],"widestWord":{"text":"Oferta","width":50},"widestLine":50,"lineHeight":19.44,"visible":1,"fitting":0.015}}`

func tinySurface(id string) string {
	return `{"id":"` + id + `","canvas":{"width":1080,"height":1080,"background":"#ffffff"},"layers":[` + tinyText + `],"texts":` + tinyTextFacts + `}`
}

func perceivedDeps(kind studio.Kind) StudioDeps {
	deps := studioDeps(kind, nil, nil)
	deps.Perception = studio_usecase.NewPerceptionService()
	return deps
}

func imageEdit(deps StudioDeps, data string) map[string]any {
	editor := &fakeEditor{replies: map[copilot.ScreenCommandName]copilot.ScreenReply{copilot.ScreenEdit: {OK: true, Data: []byte(data)}}}
	args := map[string]interface{}{"operations": []interface{}{map[string]interface{}{"op": "add_text", "text": "Oferta"}}}
	res := NewStudioEditImageTool(deps).Execute(context.Background(), studioSession(copilot.StudioImage, editor), args)
	var out map[string]any
	_ = json.Unmarshal([]byte(renderJSON(res.Data)), &out)
	return out
}

func TestTheEditChecksComeFromTheBackendAnalysisOfTheReturnedDocument(t *testing.T) {
	out := imageEdit(perceivedDeps(studio.KindImage), `{"applied":1,"perception":{"approximate":false,"artboards":[`+tinySurface("capa")+`]}}`)
	checks, _ := out["checks"].([]any)
	if len(checks) == 0 || !strings.HasPrefix(checks[0].(string), "erro: o texto t (\"Oferta\") está pequeno demais") {
		t.Fatalf("checks = %v", out)
	}
	if _, leaked := out["perception"]; leaked {
		t.Fatal("the measured facts must not reach the model")
	}
}

func TestTheEditSaysTheChecksCouldNotRunInsteadOfSayingNothingIsWrong(t *testing.T) {
	for name, deps := range map[string]StudioDeps{"no facts": perceivedDeps(studio.KindImage), "no analysis": studioDeps(studio.KindImage, nil, nil)} {
		data := `{"applied":1,"checks":[]}`
		if name == "no analysis" {
			data = `{"applied":1,"perception":{"approximate":false,"artboards":[` + tinySurface("capa") + `]}}`
		}
		out := imageEdit(deps, data)
		if _, said := out["checks"]; said || out["checks_unavailable"] != studioChecksUnavailable {
			t.Fatalf("%s: result = %v", name, out)
		}
	}
	broken := imageEdit(perceivedDeps(studio.KindImage), `{"applied":1,"perception":{"approximate":false,"artboards":[{"canvas":{"width":1080,"height":1080,"background":"#fff"},"layers":[`+tinyText+`],"texts":{}}]}}`)
	if broken["checks_unavailable"] != studioChecksUnavailable {
		t.Fatalf("a text without its measured layout must not pass as clean, got %v", broken)
	}
}

func TestTheImageReadCarriesTheAlertsOfEachArtboard(t *testing.T) {
	data := `{"artboards":[{"id":"capa"},{"id":"verso"}],"perception":{"approximate":false,"artboards":[` + tinySurface("capa") + `,{"canvas":{"width":1080,"height":1080,"background":"#ffffff"},"layers":[],"texts":{}}]}}`
	editor := &fakeEditor{replies: map[copilot.ScreenCommandName]copilot.ScreenReply{copilot.ScreenRead: {OK: true, Data: []byte(data)}}}
	res := NewStudioReadTool(perceivedDeps(studio.KindImage)).Execute(context.Background(), studioSession(copilot.StudioImage, editor), nil)
	out := renderJSON(res.Data)
	if !strings.Contains(out, `"alerts":["aviso: faixa vazia`) || strings.Contains(out, "perception") || strings.Count(out, `"alerts"`) != 1 {
		t.Fatalf("read = %s", out)
	}
	silent := &fakeEditor{replies: map[copilot.ScreenCommandName]copilot.ScreenReply{copilot.ScreenRead: {OK: true, Data: []byte(`{"artboards":[{"id":"capa"}]}`)}}}
	res = NewStudioReadTool(perceivedDeps(studio.KindImage)).Execute(context.Background(), studioSession(copilot.StudioImage, silent), nil)
	if !strings.Contains(renderJSON(res.Data), studioReportsUnavailable) {
		t.Fatalf("a read without facts must say the alerts could not be measured, got %s", renderJSON(res.Data))
	}
}

func lookAt(deps StudioDeps, data string, target *copilot.ScreenTarget) string {
	editor := &fakeEditor{replies: map[copilot.ScreenCommandName]copilot.ScreenReply{copilot.ScreenLook: {OK: true, Data: []byte(data), Images: []string{"data:image/jpeg;base64,AAAA"}, Target: target}}}
	res := NewStudioLookTool(deps).Execute(context.Background(), studioSession(copilot.StudioImage, editor), map[string]interface{}{"view": "guides"})
	return renderJSON(res.Data)
}

func TestTheGuidesLookKeepsTheEditorLinesAndAddsTheMeasuredReport(t *testing.T) {
	tilted := `{"id":"a","type":"shape","shape":"rect","fill":"#111111","transform":{"x":0.3,"y":0.3,"w":0.2,"h":0.2,"rotation":-12,"opacity":1}}`
	facts := `{"approximate":false,"view":"guides","surface":{"canvas":{"width":1080,"height":1080,"background":"#ffffff"},"layers":[` + tilted + `],"texts":{}},"targets":[{"id":"a","transform":{"x":0.3,"y":0.3,"w":0.2,"h":0.2,"rotation":-12,"opacity":1}},{"id":"b","transform":{"x":0.7,"y":0.7,"w":0.2,"h":0.2,"rotation":-14.5,"opacity":1}}]}`
	out := lookAt(perceivedDeps(studio.KindImage), `{"artboard_id":"capa","guides":{"lines":[]},"perception":`+facts+`}`, &copilot.ScreenTarget{Kind: copilot.LookArtboard, ArtboardID: "capa"})
	for _, want := range []string{`"lines":[]`, `"angles":[{"degrees":-14.5,"ids":["b"]},{"degrees":-12,"ids":["a"]}]`, `ficam quase paralelas`, `"layout":{"margins"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("look lost %s: %s", want, out)
		}
	}
	if strings.Contains(out, "perception") {
		t.Fatalf("the measured facts leaked: %s", out)
	}
}

func TestALookAtTheWholeSheetOrAtMediaNeedsNoReports(t *testing.T) {
	for _, target := range []*copilot.ScreenTarget{{Kind: copilot.LookArtboards}, {Kind: copilot.LookMedia}} {
		out := lookAt(perceivedDeps(studio.KindImage), `{"artboards":[]}`, target)
		if strings.Contains(out, "unavailable") {
			t.Fatalf("%s look got %s", target.Kind, out)
		}
	}
	if out := lookAt(perceivedDeps(studio.KindImage), `{"artboard_id":"capa"}`, &copilot.ScreenTarget{Kind: copilot.LookArtboard}); !strings.Contains(out, studioReportsUnavailable) {
		t.Fatalf("an artboard look without facts must say so, got %s", out)
	}
}

func TestASheetLookWhoseMarksFailedSaysSo(t *testing.T) {
	out := lookAt(perceivedDeps(studio.KindImage), `{"artboards":[],"perception":{"approximate":false,"unmarked":true}}`, &copilot.ScreenTarget{Kind: copilot.LookArtboards})
	if !strings.Contains(out, "as marcas dos problemas não puderam ser desenhadas") || strings.Contains(out, `"perception"`) {
		t.Fatalf("sheet look = %s", out)
	}
}
