package perception

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"vozko/domain/studio"
)

var unitCanvas = Canvas{Width: 1000, Height: 1000}

func card(id string, rotation float64, effects []studio.Effect) *studio.Layer {
	return &studio.Layer{ID: id, Type: studio.LayerShape, Shape: studio.ShapeRect, Fill: "#ffffff", Transform: studio.Transform{X: 0.5, Y: 0.5, W: 0.2, H: 0.2, Rotation: rotation, Opacity: 1}, Effects: effects}
}

func drop(x, y float64) studio.Effect {
	return studio.Effect{ID: "d", Type: "drop-shadow", Color: "#000000", Opacity: 0.3, X: x, Y: y, Blur: 20}
}

func close(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}

func TestShadowCastsTurnWithALayerStyle(t *testing.T) {
	casts := shadowCasts(card("c", 90, []studio.Effect{drop(0, 10)}), unitCanvas)
	if len(casts) != 1 || !close(casts[0].dx, -10) || !close(casts[0].dy, 0) || !close(casts[0].angle, 180) {
		t.Fatalf("cast %+v", casts[0])
	}
}

func TestSimpleShadowStaysInScreenSpace(t *testing.T) {
	layer := card("c", 90, nil)
	layer.Shadow = &studio.Shadow{Color: "#00000066", Blur: 12, Y: 10}
	casts := shadowCasts(layer, unitCanvas)
	if !close(casts[0].dx, 0) || !close(casts[0].dy, 10) || math.Abs(casts[0].alpha-0.4) > 0.01 {
		t.Fatalf("cast %+v", casts[0])
	}
}

func TestFlippedShapeMirrorsTheOffsetBeforeTurning(t *testing.T) {
	layer := card("c", 0, []studio.Effect{drop(8, 6)})
	layer.FlipX = true
	casts := shadowCasts(layer, unitCanvas)
	if !close(casts[0].dx, -8) || !close(casts[0].dy, 6) {
		t.Fatalf("cast %+v", casts[0])
	}
}

func TestCastAlphaMultipliesColorEffectAndLayer(t *testing.T) {
	effect := drop(0, 10)
	effect.Opacity, effect.Color = 0.5, "#00000080"
	layer := card("c", 0, []studio.Effect{effect})
	layer.Transform.Opacity = 0.5
	if alpha := shadowCasts(layer, unitCanvas)[0].alpha; math.Abs(alpha-0.125) > 0.01 {
		t.Fatalf("alpha %v", alpha)
	}
}

func TestHiddenInnerAndGlowEffectsAreNotLight(t *testing.T) {
	hidden := drop(0, 10)
	hidden.Hidden = true
	glow := studio.Effect{ID: "g", Type: "outer-glow", Color: "#ffffff", Opacity: 1, Blur: 30}
	inner := studio.Effect{ID: "i", Type: "inner-shadow", Color: "#000000", Opacity: 1, Y: 4, Blur: 4}
	casts := shadowCasts(card("c", 0, []studio.Effect{glow, inner, hidden}), unitCanvas)
	if len(casts) != 1 || casts[0].kind != castGlow {
		t.Fatalf("casts %+v", casts)
	}
}

func castsOf(layers ...*studio.Layer) []*shadowCast {
	var casts []*shadowCast
	for _, layer := range layers {
		casts = append(casts, shadowCasts(layer, unitCanvas)...)
	}
	return casts
}

func outlierIDs(casts []*shadowCast) []string {
	ids := []string{}
	for _, o := range lightOutliers(casts) {
		ids = append(ids, o.cast.layerID)
	}
	return ids
}

func TestTheShadowAgainstTheLightIsBlamed(t *testing.T) {
	casts := castsOf(card("a", 0, []studio.Effect{drop(0, 12)}), card("b", 0, []studio.Effect{drop(2, 14)}), card("c", 0, []studio.Effect{drop(0, -12)}))
	outliers := lightOutliers(casts)
	if len(outliers) != 1 || outliers[0].cast.layerID != "c" || angleApart(outliers[0].cast.angle, outliers[0].light) <= 150 {
		t.Fatalf("outliers %+v", outliers)
	}
}

func TestAShadowThatTurnedWithItsLayerIsBlamed(t *testing.T) {
	casts := castsOf(card("a", 0, []studio.Effect{drop(0, 12)}), card("b", 0, []studio.Effect{drop(0, 12)}), card("c", 60, []studio.Effect{drop(0, 12)}))
	if ids := outlierIDs(casts); !reflect.DeepEqual(ids, []string{"c"}) {
		t.Fatalf("outliers %v", ids)
	}
}

func TestSmallDifferencesAndPairsBlameNobody(t *testing.T) {
	tilted := castsOf(card("a", -8, []studio.Effect{drop(10, 16)}), card("b", 12, []studio.Effect{drop(10, 16)}), card("c", 0, []studio.Effect{drop(10, 16)}))
	pair := castsOf(card("a", 0, []studio.Effect{drop(0, 12)}), card("b", 0, []studio.Effect{drop(0, -12)}))
	if len(outlierIDs(tilted)) != 0 || len(outlierIDs(pair)) != 0 {
		t.Fatal("expected no outliers")
	}
}

func TestShadowVisibilityOnDarkAndLightGrounds(t *testing.T) {
	if shadowShows(Rgba{A: 1}, 0.5, Rgba{R: 11, G: 16, B: 38, A: 1}) {
		t.Fatal("a dark shadow on a dark ground should not show")
	}
	if !shadowShows(Rgba{R: 90, G: 58, B: 26, A: 1}, 0.22, Rgba{R: 244, G: 239, B: 230, A: 1}) {
		t.Fatal("a soft shadow on a light ground should show")
	}
}

func target(id string, rotation float64) GuideTarget {
	return GuideTarget{ID: id, Transform: studio.Transform{X: 0.5, Y: 0.5, W: 0.2, H: 0.2, Rotation: rotation, Opacity: 1}}
}

func TestTiltedLayersGroupByAngleAndNearParallelsAreNamed(t *testing.T) {
	angles := targetAngles([]GuideTarget{target("band", -12), target("title", -12.4), target("seal", -14.5), target("logo", 0)})
	want := []AngleGroup{{Degrees: -14.5, IDs: []string{"seal"}}, {Degrees: -12.2, IDs: []string{"band", "title"}}}
	if !reflect.DeepEqual(angles, want) {
		t.Fatalf("angles %+v", angles)
	}
	notes := angleNotes(angles)
	if len(notes) != 1 || !strings.Contains(notes[0], "-14.5") {
		t.Fatalf("notes %v", notes)
	}
}

func TestAnglesThatDifferOnPurposeStayQuiet(t *testing.T) {
	if notes := angleNotes(targetAngles([]GuideTarget{target("a", -12), target("b", 30)})); len(notes) != 0 {
		t.Fatalf("notes %v", notes)
	}
}

func TestNumbersPrintLikeTheBrowser(t *testing.T) {
	cases := map[float64]string{0: "0", 3: "3", 4.5: "4.5", 0.035: "0.035", -12.2: "-12.2", 1e-7: "1e-7", 1e21: "1e+21", 123456.75: "123456.75"}
	for value, want := range cases {
		if got := num(value); got != want {
			t.Errorf("num(%v) = %s, want %s", value, got, want)
		}
	}
	if jsRound(-2.5) != -2 || jsRound(2.5) != 3 || jsRound(0.49999999999999994) != 0 {
		t.Fatal("rounding differs from Math.round")
	}
}

func TestSnippetCutsLongTextLikeTheBrowser(t *testing.T) {
	layer := &studio.Layer{Text: "  Uma   plataforma\nunificada com CRM e muito mais  "}
	if got := snippet(layer); got != "Uma plataforma unificada…" {
		t.Fatalf("snippet %q", got)
	}
}
