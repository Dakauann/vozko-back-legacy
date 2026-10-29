package site

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestInsertElementBecomesAnInsertSectionWithTheMarkupGoWrites(t *testing.T) {
	batch, refused := prepare(t, htmlProject(), `[
	  {"op":"insert_element","at":{"after":"pages/index.html#0"},"rev":1,"element":"webchat","key":"wk_Ab12-x","ref":"chat"},
	  {"op":"insert_element","at":{"page":"pages/obrigado.html"},"rev":1,"element":"booking","key":"agenda-1"}
	]`)
	if refused != nil {
		t.Fatalf("got %s", failures(refused))
	}
	ops := batch.Operations
	if len(ops) != 2 || ops[0].Op != OpInsertSection || ops[1].Op != OpInsertSection {
		t.Fatalf("got %v", encodedOps(t, batch))
	}
	if ops[0].HTML != `<vz-webchat widget="wk_Ab12-x"></vz-webchat>` || ops[0].Ref != "chat" || ops[0].At.After != "pages/index.html#0" || ops[0].Rev != 1 {
		t.Fatalf("got %v", encodedOps(t, batch))
	}
	if ops[1].HTML != `<vz-booking booking="agenda-1"></vz-booking>` || ops[1].At.Page != "pages/obrigado.html" {
		t.Fatalf("got %v", encodedOps(t, batch))
	}
	if batch.Origins[0].Op != OpInsertElement || batch.Origins[1].Index != 1 {
		t.Fatalf("got %+v", batch.Origins)
	}
	want := []ElementUse{{Index: 0, Element: ElementWebchat, Key: "wk_Ab12-x"}, {Index: 1, Element: ElementBooking, Key: "agenda-1"}}
	if got := batch.PlatformElements(); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %+v", got)
	}
}

func TestInsertElementWorksInReactPages(t *testing.T) {
	batch, refused := prepare(t, reactProject(), `[{"op":"insert_element","at":{"page":"src/pages/Home.jsx"},"rev":1,"element":"booking","key":"agenda"}]`)
	if refused != nil || batch.Operations[0].HTML != `<vz-booking booking="agenda"></vz-booking>` {
		t.Fatalf("got %s %v", failures(refused), encodedOps(t, batch))
	}
}

func TestInsertElementRefusesAnythingButAKnownPlatformElement(t *testing.T) {
	cases := map[string]string{
		`{"op":"insert_element","at":{"page":"pages/index.html"},"rev":1,"key":"agenda"}`:                                                "0:field_required:element",
		`{"op":"insert_element","at":{"page":"pages/index.html"},"rev":1,"element":"script","key":"agenda"}`:                             "0:element_unknown:element",
		`{"op":"insert_element","at":{"page":"pages/index.html"},"rev":1,"element":"vz-webchat","key":"agenda"}`:                         "0:element_unknown:element",
		`{"op":"insert_element","at":{"page":"pages/index.html"},"rev":1,"element":"webchat"}`:                                           "0:field_required:key",
		`{"op":"insert_element","at":{"page":"pages/index.html"},"rev":1,"element":"webchat","key":"a\" onload=\"x()"}`:                  "0:element_key_invalid:key",
		`{"op":"insert_element","at":{"page":"pages/index.html"},"rev":1,"element":"booking","key":"Agenda Nova"}`:                       "0:element_key_invalid:key",
		`{"op":"insert_element","at":{"page":"pages/index.html"},"rev":1,"element":"booking","key":"agenda","html":"<script></script>"}`: "",
		`{"op":"insert_element","rev":1,"element":"booking","key":"agenda"}`:                                                             "0:field_required:at",
		`{"op":"insert_element","at":{"page":"pages/nope.html"},"rev":1,"element":"booking","key":"agenda"}`:                             "0:path_not_found:at",
		`{"op":"insert_element","at":{"after":"pages/index.html#0"},"element":"booking","key":"agenda"}`:                                 "0:field_required:rev",
		`{"op":"insert_element","at":{"page":"pages/index.html"},"rev":1,"element":"booking","key":"agenda","ref":"Bad Ref"}`:            "0:ref_invalid:ref",
	}
	for op, want := range cases {
		batch, refused := prepare(t, htmlProject(), "["+op+"]")
		if got := failures(refused); got != want {
			t.Fatalf("%s\ngot  %s\nwant %s", op, got, want)
		}
		if want == "" && (len(batch.Ignored) != 1 || batch.Ignored[0].Field != "html" || strings.Contains(batch.Operations[0].HTML, "script")) {
			t.Fatalf("raw markup must never ride on insert_element: %+v %v", batch.Ignored, encodedOps(t, batch))
		}
	}
}

func TestInsertElementIsAShorthandAndRoundTrips(t *testing.T) {
	if OpInsertElement.Family() != FamilyShorthand {
		t.Fatal("insert_element expands to insert_section")
	}
	found := false
	for _, kind := range ShorthandKinds() {
		found = found || kind == OpInsertElement
	}
	if !found {
		t.Fatal("insert_element is listed with the shorthand kinds")
	}
	o := Operation{Op: OpInsertElement, At: &Placement{Into: "pages/index.html#0"}, Rev: 2, Element: ElementWebchat, Key: "wk"}
	raw, err := json.Marshal(o)
	if err != nil || string(raw) != `{"op":"insert_element","rev":2,"at":{"into":"pages/index.html#0"},"element":"webchat","key":"wk"}` {
		t.Fatalf("got %s %v", raw, err)
	}
}

func TestPlatformElementsMatchTheBridgeFixture(t *testing.T) {
	raw, err := os.ReadFile("../../infra/siteruntime/testdata/live_rules.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Elements []struct {
			Element string `json:"element"`
			Tag     string `json:"tag"`
			Attr    string `json:"attr"`
		} `json:"elements"`
		ElementKeys []struct {
			Element string `json:"element"`
			Key     string `json:"key"`
			Edit    bool   `json:"edit"`
		} `json:"elementKeys"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Elements) != len(PlatformElements()) {
		t.Fatalf("fixture lists %d platform elements, Go %d", len(fixture.Elements), len(PlatformElements()))
	}
	for i, e := range PlatformElements() {
		got := fixture.Elements[i]
		if got.Element != string(e) || got.Tag != e.Tag() || got.Attr != e.Attr() {
			t.Fatalf("fixture %+v, Go %s %s %s", got, e, e.Tag(), e.Attr())
		}
	}
	if len(fixture.ElementKeys) == 0 {
		t.Fatal("the fixture carries key cases")
	}
	for _, c := range fixture.ElementKeys {
		op, _ := json.Marshal([]any{map[string]any{"op": "insert_element", "at": map[string]string{"page": "pages/index.html"}, "rev": 1, "element": c.Element, "key": c.Key}})
		_, refused := prepare(t, htmlProject(), string(op))
		if (refused == nil) != c.Edit {
			t.Errorf("insert_element %s %q: accepted %v, fixture says %v (%s)", c.Element, c.Key, refused == nil, c.Edit, failures(refused))
		}
	}
}
