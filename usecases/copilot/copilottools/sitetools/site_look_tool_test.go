package sitetools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	"vozko/domain/copilot"
	"vozko/domain/site"
	site_usecase "vozko/usecases/site"
	"vozko/usecases/siterender"
)

func sheetURL(t *testing.T) string {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 320, 200)), nil); err != nil {
		t.Fatal(err)
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func pngURL(t *testing.T) string {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 320, 200))); err != nil {
		t.Fatal(err)
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func capturedOf(width int) viewportFacts {
	facts := capturedFacts{ScrollWidth: width, Overflow: []string{}, Texts: []capturedText{}, Images: []capturedImage{}, ConsoleErrors: []string{}, FailedRequests: []capturedRequest{}}
	if width <= 480 {
		facts.ScrollWidth = width + 120
		facts.Overflow = []string{"src/components/Hero.jsx#88"}
	}
	for i := 0; i < 40; i++ {
		facts.Texts = append(facts.Texts, capturedText{Node: "src/components/LeadForm.jsx#1234", FG: [3]int{170, 170, 170}, BG: [3]int{255, 255, 255}, Px: 14, Weight: 400})
	}
	facts.ConsoleErrors = append(facts.ConsoleErrors, "TypeError: x is undefined")
	facts.FailedRequests = append(facts.FailedRequests, capturedRequest{URL: "https://cdn.example.com/hero.webp", Status: 404})
	facts.Metrics = capturedMetrics{LCP: 4100, CLS: 0.02, TBT: 40}
	return viewportFacts{W: width, Facts: facts}
}

func richCapture(viewports []lookViewport, marks bool) lookReply {
	reply := lookReply{}
	for _, v := range viewports {
		reply.Viewports = append(reply.Viewports, lookViewportReport{W: v.W, PageH: 4200, Slices: 5})
		reply.Facts = append(reply.Facts, capturedOf(v.W))
	}
	if marks {
		for i := 0; i < site.MaxMarks; i++ {
			reply.Marks = append(reply.Marks, lookMark{N: i + 1, Node: "src/routes/index.jsx#" + strings.Repeat("9", 4), Tag: "section"})
		}
	}
	return reply
}

func lookAnswer(t *testing.T, reply lookReply, image string) copilot.ScreenReply {
	t.Helper()
	data, err := json.Marshal(reply)
	if err != nil {
		t.Fatal(err)
	}
	return copilot.ScreenReply{OK: true, Data: data, Images: []string{image}}
}

func answering(t *testing.T, r *rig, reply copilot.ScreenReply) {
	t.Helper()
	if r.editor.replies == nil {
		r.editor.replies = map[copilot.ScreenCommandName]copilot.ScreenReply{}
	}
	r.editor.replies[copilot.ScreenLook] = reply
}

func TestLookAsksTheOpenEditorForThePreviewRouteAndChecksTheFactsServerSide(t *testing.T) {
	r := newRig(t)
	mobile := []lookViewport{{W: 390, H: 844}}
	image := sheetURL(t)
	answering(t, r, lookAnswer(t, richCapture(mobile, true), image))
	res := NewSiteLookTool(r.deps).Execute(context.Background(), r.cc, args(t, `{"route":"/obrigado","device":"mobile","marks":true}`))
	if res.Status != copilot.StatusOK || len(res.Images) != 1 || res.Images[0] != image {
		t.Fatalf("got %+v", res)
	}
	sent := r.editor.sent(copilot.ScreenLook)
	if len(sent) != 1 {
		t.Fatalf("sent %+v", r.editor.commands)
	}
	order := sent[0].Args.(lookOrder)
	if order.PreviewURL != previewURL || order.Route != "/obrigado" || len(order.Viewports) != 1 || order.Viewports[0] != (lookViewport{W: 390, H: 844}) || order.Marks == nil || order.Marks.Limit != site.MaxMarks || len(order.Marks.Ranks) == 0 {
		t.Fatalf("order %+v", order)
	}
	raw, _ := json.Marshal(res.Data)
	for _, want := range []string{`"open_gaps":[]`, `"marks":["1 src/routes/index.jsx#9999 section"`, site.CheckContrastLow, site.CheckOverflowHorizontal, site.CheckConsoleError, site.CheckRequestFailed, site.CheckLCPSlow, `"390 px: página de 4200 px em 5 faixas"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the look lacks %s: %s", want, raw)
		}
	}
}

func TestLookDefaultsToTheRouteOnTheScreenAndBothDevices(t *testing.T) {
	r := newRig(t)
	both := viewportsFor("")
	answering(t, r, lookAnswer(t, richCapture(both, false), sheetURL(t)))
	r.editor.replies[copilot.ScreenRead] = copilot.ScreenReply{OK: true, Data: []byte(`{"stack":"react","files":{},"route":"/privacidade","selection":null}`)}
	res := NewSiteLookTool(r.deps).Execute(context.Background(), r.cc, map[string]interface{}{})
	order := r.editor.sent(copilot.ScreenLook)[0].Args.(lookOrder)
	if res.Status != copilot.StatusOK || order.Route != "/privacidade" || len(order.Viewports) != 2 || order.Viewports[0].W != siterender.Desktop.Width || order.Marks != nil {
		t.Fatalf("got %+v %+v", res, order)
	}
}

func TestLookRefusesBlindModelsUnknownRoutesBadFocusAndBrokenBuildsWithoutAsking(t *testing.T) {
	cases := map[string]struct {
		prepare func(r *rig)
		args    string
		want    string
	}{
		"blind": {func(r *rig) { r.cc.SeesImages = false }, `{}`, "não enxerga"},
		"route": {func(*rig) {}, `{"route":"/precos"}`, "/precos"},
		"focus": {func(*rig) {}, `{"focus_node":"../x"}`, "focus_node"},
		"build": {func(r *rig) {
			r.storage.previewErr = &site_usecase.BuildFailedError{Errors: []site_usecase.BuildIssue{{Path: "src/App.jsx", Line: 3, Message: "boom"}}}
		}, `{"route":"/"}`, "src/App.jsx:3 boom"},
		"no preview": {func(r *rig) { r.storage.previewErr = site_usecase.ErrPreviewUnavailable }, `{"route":"/"}`, "prévia"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			c.prepare(r)
			res := NewSiteLookTool(r.deps).Execute(context.Background(), r.cc, args(t, c.args))
			if res.Status != copilot.StatusError || !strings.Contains(res.Message, c.want) || len(r.editor.sent(copilot.ScreenLook)) != 0 {
				t.Fatalf("got %+v", res)
			}
		})
	}
}

func TestLookFailsClosedOnAnyReplyTheEditorCannotBackUp(t *testing.T) {
	desktop := []lookViewport{{W: 1440, H: 900}}
	good := richCapture(desktop, false)
	badNode := richCapture(desktop, false)
	badNode.Facts[0].Facts.Overflow = []string{"<script>"}
	badColor := richCapture(desktop, false)
	badColor.Facts[0].Facts.Texts[0].FG = [3]int{300, 0, 0}
	unasked := richCapture(desktop, true)
	tooMany := richCapture(desktop, false)
	for i := 0; i <= maxCapturedTexts; i++ {
		tooMany.Facts[0].Facts.Texts = append(tooMany.Facts[0].Facts.Texts, tooMany.Facts[0].Facts.Texts[0])
	}
	otherWidth := richCapture([]lookViewport{{W: 390, H: 844}}, false)
	cases := map[string]func(t *testing.T) copilot.ScreenReply{
		"no image": func(t *testing.T) copilot.ScreenReply {
			r := lookAnswer(t, good, sheetURL(t))
			r.Images = nil
			return r
		},
		"png inside": func(t *testing.T) copilot.ScreenReply { return lookAnswer(t, good, pngURL(t)) },
		"html inside": func(t *testing.T) copilot.ScreenReply {
			return lookAnswer(t, good, "data:image/jpeg;base64,"+base64.StdEncoding.EncodeToString([]byte("<html></html>")))
		},
		"bad node":    func(t *testing.T) copilot.ScreenReply { return lookAnswer(t, badNode, sheetURL(t)) },
		"bad color":   func(t *testing.T) copilot.ScreenReply { return lookAnswer(t, badColor, sheetURL(t)) },
		"unasked":     func(t *testing.T) copilot.ScreenReply { return lookAnswer(t, unasked, sheetURL(t)) },
		"too many":    func(t *testing.T) copilot.ScreenReply { return lookAnswer(t, tooMany, sheetURL(t)) },
		"other width": func(t *testing.T) copilot.ScreenReply { return lookAnswer(t, otherWidth, sheetURL(t)) },
		"extra field": func(t *testing.T) copilot.ScreenReply {
			r := lookAnswer(t, good, sheetURL(t))
			r.Data = append(bytes.TrimSuffix(r.Data, []byte("}")), []byte(`,"html":"<b>x</b>"}`)...)
			return r
		},
	}
	for name, reply := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			answering(t, r, reply(t))
			res := NewSiteLookTool(r.deps).Execute(context.Background(), r.cc, args(t, `{"device":"desktop"}`))
			if res.Status != copilot.StatusError || len(res.Images) != 0 {
				t.Fatalf("got %+v", res)
			}
		})
	}
}

func TestLookExplainsAMissingFocusAndAnAbsentEditor(t *testing.T) {
	r := newRig(t)
	answering(t, r, copilot.ScreenReply{OK: false, Error: &copilot.ScreenError{Code: focusMissing, Message: "o node sumiu"}})
	if res := NewSiteLookTool(r.deps).Execute(context.Background(), r.cc, args(t, `{"focus_node":"src/App.jsx#4"}`)); res.Status != copilot.StatusError || res.Message != lookStale {
		t.Fatalf("got %+v", res)
	}
	r = newRig(t)
	r.editor.errs = map[copilot.ScreenCommandName]error{copilot.ScreenLook: copilot.ErrScreenUnavailable}
	res := NewSiteLookTool(r.deps).Execute(context.Background(), r.cc, map[string]interface{}{})
	if res.Status != copilot.StatusError || res.AwaitsScreen == nil || len(res.Images) != 0 {
		t.Fatalf("a silent editor is awaited, never replaced by a server render: %+v", res)
	}
}

func TestCaptureReferenceReturnsTokensAndAnImageOnlyForSeeingModels(t *testing.T) {
	r := newRig(t)
	r.renderer.reference = siterender.Reference{URL: "https://exemplo.com.br/", Title: "Exemplo", Image: []byte{1, 2, 3}, Tokens: siterender.Tokens{Fonts: []siterender.FontShare{{Family: "Inter", Share: 0.8}}}}
	res := NewSiteCaptureReferenceTool(r.deps).Execute(context.Background(), r.cc, args(t, `{"url":"https://exemplo.com.br"}`))
	if res.Status != copilot.StatusOK || len(res.Images) != 1 || !strings.Contains(decoded(t, res.Data)["note"].(string), "nunca copie") {
		t.Fatalf("got %+v", res)
	}
	r.cc.SeesImages = false
	if res := NewSiteCaptureReferenceTool(r.deps).Execute(context.Background(), r.cc, args(t, `{"url":"https://exemplo.com.br"}`)); res.Status != copilot.StatusOK || len(res.Images) != 0 {
		t.Fatalf("got %+v", res)
	}
	r.renderer.refErr = siterender.ErrReferenceRefused
	if res := NewSiteCaptureReferenceTool(r.deps).Execute(context.Background(), r.cc, args(t, `{"url":"http://10.0.0.1"}`)); res.Status != copilot.StatusError {
		t.Fatalf("got %+v", res)
	}
}
