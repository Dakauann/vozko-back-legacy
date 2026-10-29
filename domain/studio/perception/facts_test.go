package perception

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func textSurface(id string, measured bool) Surface {
	raw := `{"id":"` + id + `","canvas":{"width":1080,"height":1080,"background":"#ffffff"},"layers":[{"id":"t","type":"text","text":"Oferta","fontSize":0.015,"fill":"#111111","transform":{"x":0.5,"y":0.5,"w":0.8,"h":0.14,"rotation":0,"opacity":1}}],"texts":{}}`
	surface, err := DecodeSurface([]byte(raw))
	if err != nil {
		panic(err)
	}
	if measured {
		surface.Texts["t"] = &TextFacts{Paragraphs: [][]string{{"Oferta"}}, WidestWord: MeasuredWord{Text: "Oferta", Width: 50}, WidestLine: 50, LineHeight: 19.44, Visible: 1, Fitting: 0.015}
	}
	return *surface
}

func TestChecksNameTheArtboardWhenTheDocumentHasSeveral(t *testing.T) {
	lines, err := Checks(&Facts{Artboards: []Surface{textSurface("capa", true)}, Multiple: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "prancheta capa: erro: o texto t") {
		t.Fatalf("lines %v", lines)
	}
	single, _ := Checks(&Facts{Artboards: []Surface{textSurface("capa", true)}})
	if strings.HasPrefix(single[0], "prancheta") {
		t.Fatalf("single artboard got a prefix: %v", single)
	}
}

func TestATextWithoutItsMeasuredLayoutFailsClosed(t *testing.T) {
	if _, err := Checks(&Facts{Artboards: []Surface{textSurface("capa", false)}}); !errors.Is(err, ErrUnmeasured) {
		t.Fatalf("err %v", err)
	}
	if _, err := Look(&Facts{View: ViewRender, Surface: ptr(textSurface("capa", false))}); !errors.Is(err, ErrUnmeasured) {
		t.Fatalf("err %v", err)
	}
}

func TestChecksWithoutAnythingToCheckFailClosed(t *testing.T) {
	if _, err := Checks(&Facts{}); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("err %v", err)
	}
	if _, err := Look(&Facts{}); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("err %v", err)
	}
}

func ptr[T any](v T) *T { return &v }

const shortVideo = `{"document":{"schema":"studio.video","version":1,"canvas":{"aspect":"square","background":"#000000"},"durationMs":3000,"tracks":[{"id":"v1","kind":"visual","clips":[{"id":"a","type":"overlay","startMs":0,"durationMs":3000,"trimInMs":0,"volume":1,"fadeInMs":0,"fadeOutMs":0,"transform":{"x":0.5,"y":0.5,"w":0.6,"h":0.1,"rotation":0,"opacity":1},"layer":{"id":"a-layer","type":"text","text":"Oi","fontSize":0.05,"fill":"#ffffff","transform":{"x":0.5,"y":0.5,"w":1,"h":1,"rotation":0,"opacity":1}}}]}]},"canvas":{"width":1080,"height":1080},"background":"#000000","layers":[],"frames":[],"cuts":[]}`

func TestVideoChecksFailClosedWhenAFrameIsMissing(t *testing.T) {
	video, err := DecodeVideo([]byte(shortVideo))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Checks(&Facts{Video: video}); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("err %v", err)
	}
}

func TestFramesRepeatAnElementOfThePreviousFrameByIndex(t *testing.T) {
	raw := `{"canvas":{"width":100,"height":100},"background":"#000000","layers":[{"id":"l","type":"shape","shape":"rect","fill":"#ffffff","transform":{"x":0.5,"y":0.5,"w":1,"h":1,"rotation":0,"opacity":1}}],"frames":[{"atMs":0,"elements":[{"id":"a","layer":0,"media":false,"transform":{"x":0.5,"y":0.5,"w":0.2,"h":0.2,"rotation":0,"opacity":1},"opacity":1}]},{"atMs":100,"elements":[0]}]}`
	video, err := DecodeVideo([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if got := video.Frames[1].Elements[0]; got.ID != "a" || got.Transform.W != 0.2 {
		t.Fatalf("repeated element %+v", got)
	}
	for _, broken := range []string{
		strings.Replace(raw, `"elements":[0]`, `"elements":[3]`, 1),
		strings.Replace(raw, `"layer":0`, `"layer":7`, 1),
		strings.Replace(raw, `"atMs":100`, `"atMs":0`, 1),
		strings.Replace(raw, `"opacity":1}]}`, `"opacity":1,"extra":true}]}`, 1),
	} {
		if _, err := DecodeVideo([]byte(broken)); !errors.Is(err, ErrMalformed) {
			t.Fatalf("expected a malformed error for %s, got %v", broken, err)
		}
	}
}

func TestShapesRequestNeedsExactlyOneSceneSourceAndALayer(t *testing.T) {
	surface := `{"canvas":{"width":100,"height":100,"background":"#ffffff"},"layers":[],"texts":{}}`
	for _, body := range []string{
		`{"approximate":false,"layers":{"depth":true,"layout":false}}`,
		`{"approximate":false,"layers":{"depth":false,"layout":false},"surfaces":[` + surface + `]}`,
		`{"approximate":false,"layers":{"depth":true,"layout":false},"surfaces":[` + surface + `],"video":` + shortVideo + `}`,
		`{"approximate":false,"layers":{"depth":true,"layout":false},"surfaces":[` + surface + `],"unknown":1}`,
	} {
		if _, err := DecodeShapesRequest([]byte(body)); !errors.Is(err, ErrMalformed) {
			t.Fatalf("expected malformed for %s, got %v", body, err)
		}
	}
	request, err := DecodeShapesRequest([]byte(`{"approximate":false,"layers":{"depth":true,"layout":true},"surfaces":[` + surface + `]}`))
	if err != nil {
		t.Fatal(err)
	}
	shapes, err := Shapes(request)
	if err != nil || len(shapes) != 1 || shapes[0] == nil {
		t.Fatalf("shapes %v err %v", shapes, err)
	}
}

func heavySurfaces(tb testing.TB) []Surface {
	tb.Helper()
	var surfaces []Surface
	seen := map[string]bool{}
	readHeavy(tb, func(g golden) {
		if g.Fn == "imageLook" && g.Args.View == ViewRender && !g.Approximate && strings.HasSuffix(g.Source, "-perceived") && !seen[g.Source] {
			seen[g.Source] = true
			surfaces = append(surfaces, *g.Surface)
		}
	})
	if len(surfaces) == 0 {
		tb.Fatal("no heavy surfaces in the goldens")
	}
	return surfaces
}

func readHeavy(tb testing.TB, each func(golden)) {
	readGoldens(tb, filepath.Join("testdata", "goldens", "heavy.jsonl.gz"), func(_ int, raw []byte) {
		var g golden
		if err := json.Unmarshal(raw, &g); err != nil {
			tb.Fatal(err)
		}
		each(g)
	})
}

func overlayOf(surfaces []Surface) error {
	_, err := Shapes(&ShapesRequest{Layers: Layers{Depth: true, Layout: true}, Surfaces: surfaces})
	return err
}

func TestHeavyDocumentOverlayStaysUnderFiftyMillisecondsAtP95(t *testing.T) {
	if os.Getenv("PERCEPTION_TIMING") == "" {
		t.Skip("set PERCEPTION_TIMING=1 to measure the overlay latency on this machine")
	}
	surfaces := heavySurfaces(t)
	var took []time.Duration
	for i := 0; i < 60; i++ {
		start := time.Now()
		if err := overlayOf(surfaces); err != nil {
			t.Fatal(err)
		}
		took = append(took, time.Since(start))
	}
	sort.Slice(took, func(i, j int) bool { return took[i] < took[j] })
	p95 := took[len(took)*95/100]
	t.Logf("heavy overlay: p50 %v, p95 %v over %d artboards", took[len(took)/2], p95, len(surfaces))
	if p95 > 50*time.Millisecond {
		t.Fatalf("p95 %v is over 50 ms", p95)
	}
}

func BenchmarkHeavyDocumentOverlay(b *testing.B) {
	surfaces := heavySurfaces(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := overlayOf(surfaces); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHeavyDocumentChecks(b *testing.B) {
	surfaces := heavySurfaces(b)
	facts := &Facts{Artboards: surfaces, Multiple: true}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Checks(facts); err != nil {
			b.Fatal(err)
		}
	}
}

func TestALookWhoseMarksCouldNotBeDrawnSaysSo(t *testing.T) {
	surface := textSurface("capa", true)
	data, err := Look(&Facts{View: ViewDepth, Surface: &surface, Unmarked: true})
	if err != nil || data["marks_unavailable"] != MarksUnavailable {
		t.Fatalf("data %v err %v", data, err)
	}
	clean, _ := Look(&Facts{View: ViewDepth, Surface: &surface})
	if _, said := clean["marks_unavailable"]; said {
		t.Fatalf("a drawn look must not claim missing marks: %v", clean)
	}
}

func TestAnEditThatTouchedNoArtboardChecksNothingAndSaysSoCleanly(t *testing.T) {
	facts, err := DecodeFacts([]byte(`{"approximate":false,"artboards":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	lines, err := Checks(facts)
	if err != nil || lines == nil || len(lines) != 0 {
		t.Fatalf("lines %v err %v", lines, err)
	}
}

func TestFactsTooLargeForTheReplyFailClosedEverywhere(t *testing.T) {
	facts := &Facts{Oversized: true, Artboards: []Surface{}, Surface: ptr(textSurface("capa", true))}
	if _, err := Checks(facts); !errors.Is(err, ErrOversized) {
		t.Fatalf("checks err %v", err)
	}
	if _, err := Look(facts); !errors.Is(err, ErrOversized) {
		t.Fatalf("look err %v", err)
	}
	if _, err := ArtboardAlerts(facts); !errors.Is(err, ErrOversized) {
		t.Fatalf("alerts err %v", err)
	}
	if _, err := VideoAlerts(facts); !errors.Is(err, ErrOversized) {
		t.Fatalf("video alerts err %v", err)
	}
}

func TestGuideCrossingsCountNothingOnASingleText(t *testing.T) {
	surface := textSurface("capa", true)
	if count, err := GuideCrossings(&surface); err != nil || count != 0 {
		t.Fatalf("count %d err %v", count, err)
	}
	unmeasured := textSurface("capa", false)
	if _, err := GuideCrossings(&unmeasured); !errors.Is(err, ErrUnmeasured) {
		t.Fatalf("err %v", err)
	}
}
