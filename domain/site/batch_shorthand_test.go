package site

import (
	"encoding/json"
	"strings"
	"testing"
)

func encodedOps(t *testing.T, batch Batch) []string {
	t.Helper()
	out := make([]string, 0, len(batch.Operations))
	for _, o := range batch.Operations {
		raw, err := json.Marshal(o)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(raw))
	}
	return out
}

func TestSetTextTakesTheBridgeHTML(t *testing.T) {
	batch, refused := prepare(t, htmlProject(), `[
	  {"op":"set_text","node":"pages/index.html#24","rev":1,"html":"Olá <b>mundo</b> <a href=\"/sobre\">aqui</a>"},
	  {"op":"set_text","node":"pages/index.html#24","rev":1,"html":""}
	]`)
	if refused != nil {
		t.Fatalf("got %s", failures(refused))
	}
	if *batch.Operations[0].Text != "Olá **mundo** [aqui](/sobre)" || batch.Operations[0].HTML != "" || *batch.Operations[1].Text != "" {
		t.Fatalf("got %+v", encodedOps(t, batch))
	}
}

func TestSetTextRefusesBadHTML(t *testing.T) {
	cases := map[string]string{
		`{"op":"set_text","node":"pages/index.html#24","rev":1,"html":"<p>a</p><p>b</p>"}`:                 "0:text_markup_invalid:html",
		`{"op":"set_text","node":"pages/index.html#24","rev":1,"html":"<a href=\"javascript:x()\">a</a>"}`: "0:text_markup_invalid:html",
		`{"op":"set_text","node":"pages/index.html#24","rev":1,"html":"a","text":"a"}`:                     "0:field_invalid:html",
		`{"op":"set_text","node":"pages/index.html#24","rev":1,"html":5}`:                                  "0:field_invalid:html",
	}
	for op, want := range cases {
		_, refused := prepare(t, htmlProject(), "["+op+"]")
		if got := failures(refused); got != want {
			t.Fatalf("%s\ngot  %s\nwant %s", op, got, want)
		}
	}
}

func TestKnobOperationsBecomeOneKnobsFileWrite(t *testing.T) {
	batch, refused := prepare(t, htmlProject(), `[
	  {"op":"define_knob","name":"Raio","variable":"--radius-card","kind":"range","min":0,"max":2,"step":0.25,"unit":"rem"},
	  {"op":"set_knob","name":"Raio","value":1.5},
	  {"op":"rename_knob","name":"Cor da marca","to":"Cor principal"},
	  {"op":"remove_knob","name":"Espaço do topo"}
	]`)
	if refused != nil {
		t.Fatalf("got %s", failures(refused))
	}
	ops := batch.Operations
	if len(ops) != 4 || ops[0].Op != OpWriteFile || ops[0].Path != KnobsPath || ops[0].Rev != 1 || ops[1].Op != OpSetKnob || ops[3].Op != OpWriteFile {
		t.Fatalf("got %v", encodedOps(t, batch))
	}
	final, err := DecodeKnobs([]byte(*ops[3].Content))
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, k := range final.Knobs {
		names = append(names, k.Name+"="+k.Variable)
	}
	if strings.Join(names, ",") != "Cor principal=--color-brand,Raio=--radius-card" {
		t.Fatalf("got %v", names)
	}
	for i, origin := range batch.Origins {
		if origin.Index != i {
			t.Fatalf("one knob operation is one written operation: %+v", batch.Origins)
		}
	}
	if batch.Origins[0].Op != OpDefineKnob || batch.Origins[1].Op != OpSetKnob {
		t.Fatalf("got %+v", batch.Origins)
	}
}

func TestKnobOperationsRefuseClashesAndUnknownKnobs(t *testing.T) {
	cases := map[string]string{
		`{"op":"define_knob","name":"Cor da marca","variable":"--x","kind":"color"}`:                   "0:knob_exists:name",
		`{"op":"define_knob","name":"Outra","variable":"--hero-gap","kind":"color"}`:                   "0:knob_exists:variable",
		`{"op":"define_knob","name":"Outra","variable":"--x","kind":"range","min":2,"max":1,"step":1}`: "0:knobs_invalid:max",
		`{"op":"define_knob","name":"Outra","variable":"x","kind":"color"}`:                            "0:knobs_invalid:variable",
		`{"op":"define_knob","name":"Outra","variable":"--x"}`:                                         "0:field_required:kind",
		`{"op":"define_knob","variable":"--x","kind":"color"}`:                                         "0:field_required:name",
		`{"op":"define_knob","name":"Outra","variable":"--x","kind":"color","min":"a"}`:                "0:field_invalid:min",
		`{"op":"rename_knob","name":"Brilho","to":"Luz"}`:                                              "0:knob_unknown:name",
		`{"op":"rename_knob","name":"Cor da marca","to":"Espaço do topo"}`:                             "0:knob_exists:to",
		`{"op":"rename_knob","name":"Cor da marca"}`:                                                   "0:field_required:to",
		`{"op":"remove_knob","name":"Brilho"}`:                                                         "0:knob_unknown:name",
	}
	for op, want := range cases {
		_, refused := prepare(t, htmlProject(), "["+op+"]")
		if got := failures(refused); got != want {
			t.Fatalf("%s\ngot  %s\nwant %s", op, got, want)
		}
	}
}

func TestKnobOperationsNeedAReadableKnobsFile(t *testing.T) {
	p := htmlProject()
	p.Files[KnobsPath] = File{Content: "{", Rev: 2}
	_, refused := prepare(t, p, `[{"op":"remove_knob","name":"Cor da marca"}]`)
	if got := failures(refused); got != "0:knobs_invalid:name" {
		t.Fatalf("got %s", got)
	}
	delete(p.Files, KnobsPath)
	batch, refused := prepare(t, p, `[{"op":"define_knob","name":"Cor","variable":"--color-brand","kind":"color"}]`)
	if refused != nil || batch.Operations[0].Rev != 0 {
		t.Fatalf("a missing knobs file starts empty: %s %+v", failures(refused), batch.Operations)
	}
}

func TestWhatsAppLinkOperationsExpandIntoAttributes(t *testing.T) {
	batch, refused := prepare(t, htmlProject(), `[
	  {"op":"set_text","node":"pages/index.html#0","rev":1,"text":"Oi"},
	  {"op":"set_whatsapp_link","node":"pages/index.html#24","rev":1,"number_ref":"phone_9","text":"  Quero saber mais  "},
	  {"op":"clear_whatsapp_link","node":"pages/index.html#24","rev":1}
	]`)
	if refused != nil {
		t.Fatalf("got %s", failures(refused))
	}
	want := []string{
		`{"op":"set_text","node":"pages/index.html#0","rev":1,"text":"Oi"}`,
		`{"op":"set_attr","node":"pages/index.html#24","rev":1,"name":"data-vz-whatsapp","value":"phone_9"}`,
		`{"op":"set_attr","node":"pages/index.html#24","rev":1,"name":"data-vz-text","value":"Quero saber mais"}`,
		`{"op":"set_attr","node":"pages/index.html#24","rev":1,"name":"href","value":"#"}`,
		`{"op":"set_attr","node":"pages/index.html#24","rev":1,"name":"data-vz-whatsapp"}`,
		`{"op":"set_attr","node":"pages/index.html#24","rev":1,"name":"data-vz-text"}`,
	}
	if got := encodedOps(t, batch); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s", strings.Join(got, "\n"))
	}
	sources := []int{}
	for _, o := range batch.Origins {
		sources = append(sources, o.Index)
	}
	if jsonOf(t, sources) != "[0,1,1,1,2,2]" || batch.Origins[3].Op != OpSetWhatsAppLink || batch.Origins[5].Op != OpClearWhatsAppLink {
		t.Fatalf("got %+v", batch.Origins)
	}
	if refs := batch.WhatsAppNumbers(); len(refs) != 1 || refs[0] != (NumberUse{Index: 1, Ref: "phone_9", Field: "number_ref"}) {
		t.Fatalf("got %+v", refs)
	}
}

func TestWhatsAppLinkOperationsRefuseBadInput(t *testing.T) {
	long := strings.Repeat("a", MaxWhatsAppTextRunes+1)
	cases := map[string]string{
		`{"op":"set_whatsapp_link","node":"pages/index.html#24","rev":1,"text":"oi"}`:                             "0:field_required:number_ref",
		`{"op":"set_whatsapp_link","node":"pages/index.html#24","rev":1,"number_ref":"a b"}`:                      "0:field_invalid:number_ref",
		`{"op":"set_whatsapp_link","node":"pages/index.html#24","rev":1,"number_ref":"p1","text":"` + long + `"}`: "0:whatsapp_text_too_long:text",
		`{"op":"set_whatsapp_link","node":"pages/index.html#24","number_ref":"p1"}`:                               "0:field_required:rev",
		`{"op":"clear_whatsapp_link","node":"theme.css#0","rev":1}`:                                               "0:node_invalid:node",
	}
	for op, want := range cases {
		_, refused := prepare(t, htmlProject(), "["+op+"]")
		if got := failures(refused); got != want {
			t.Fatalf("%s\ngot  %s\nwant %s", op, got, want)
		}
	}
}

func TestDirectWhatsAppAttributesAreCheckedToo(t *testing.T) {
	batch, refused := prepare(t, htmlProject(), `[{"op":"set_attr","node":"pages/index.html#24","rev":1,"name":"data-vz-whatsapp","value":"inst_2"}]`)
	if refused != nil {
		t.Fatal(failures(refused))
	}
	if refs := batch.WhatsAppNumbers(); len(refs) != 1 || refs[0] != (NumberUse{Index: 0, Ref: "inst_2", Field: "value"}) {
		t.Fatalf("got %+v", refs)
	}
}

func TestStaleRevsListTheOlderRevisionsTheBuilderNeeds(t *testing.T) {
	p := revisedProject()
	batch, refused := prepare(t, p, `[
	  {"op":"set_text","node":"pages/index.html#24","rev":2,"text":"Oi"},
	  {"op":"move","node":"pages/index.html#0","rev":3,"after":"pages/index.html#24"},
	  {"op":"insert_section","at":{"before":"pages/index.html#0"},"rev":2,"html":"<p>x</p>"},
	  {"op":"set_text","node":"pages/obrigado.html#0","rev":1,"text":"Valeu"},
	  {"op":"set_text","node":"pages/index.html#24","rev":4,"text":"Oi"}
	]`)
	if refused != nil {
		t.Fatal(failures(refused))
	}
	if got := jsonOf(t, batch.StaleRevs(p)); got != `{"pages/index.html":[2,3]}` {
		t.Fatalf("got %s", got)
	}
}

func jsonOf(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestANodeReadAtAnOlderRevNeedsThatRevision(t *testing.T) {
	p := revisedProject()
	cases := map[string]struct {
		node string
		rev  uint64
		want string
	}{
		"older":   {"pages/index.html#24", 2, `{"pages/index.html":[2]}`},
		"current": {"pages/index.html#24", p.Files["pages/index.html"].Rev, `{}`},
		"no rev":  {"pages/index.html#24", 0, `{}`},
		"ref":     {"@novo", 1, `{}`},
		"garbage": {"index", 1, `{}`},
		"missing": {"pages/nada.html#0", 1, `{}`},
	}
	for name, c := range cases {
		if got := jsonOf(t, NodeStaleRevs(p, c.node, c.rev)); got != c.want {
			t.Errorf("%s: got %s", name, got)
		}
	}
}
