package site

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func prepare(t *testing.T, p Project, raw string) (Batch, *BatchError) {
	t.Helper()
	batch, err := PrepareBatch(p, []byte(raw))
	if err == nil {
		return batch, nil
	}
	var refused *BatchError
	if !errors.As(err, &refused) || !errors.Is(err, ErrEditRefused) {
		t.Fatalf("got %v", err)
	}
	return batch, refused
}

func failures(refused *BatchError) string {
	if refused == nil {
		return ""
	}
	parts := []string{}
	for _, f := range refused.Failures {
		parts = append(parts, jsonInt(f.Index)+":"+f.Code+":"+f.Field)
	}
	for _, s := range refused.Skipped {
		parts = append(parts, jsonInt(s.Index)+":skipped:"+jsonInt(s.After))
	}
	return strings.Join(parts, ",")
}

func jsonInt(n int) string {
	raw, _ := json.Marshal(n)
	return string(raw)
}

func TestPrepareBatchAcceptsEveryOperationFamily(t *testing.T) {
	raw := `[
	  {"op":"write_file","path":"pages/sobre.html","content":"<h1>Sobre</h1>\n"},
	  {"op":"edit_file","path":"pages/index.html","old":"<h1>Oi</h1>","new":"<h1>Olá</h1>","replace_all":true},
	  {"op":"rename_file","path":"partials/header.html","rev":1,"to":"partials/topo.html"},
	  {"op":"delete_file","path":"scripts/motion.js","rev":1},
	  {"op":"set_text","node":"pages/index.html#24","rev":1,"text":"Olá **mundo**"},
	  {"op":"set_text","node":"pages/index.html#24","rev":1,"text":"Item","item":2},
	  {"op":"set_attr","node":"pages/index.html#0","rev":1,"name":"id","value":"topo"},
	  {"op":"set_attr","node":"pages/index.html#0","rev":1,"name":"aria-hidden"},
	  {"op":"set_classes","node":"pages/index.html#0","rev":1,"add":["py-24","bg-[#123456]"],"remove":["py-20"],"breakpoint":"md"},
	  {"op":"replace_image","node":"pages/index.html#0","rev":1,"media_id":"med_01HX","alt":""},
	  {"op":"duplicate","node":"pages/index.html#0","rev":1,"ref":"copia"},
	  {"op":"move","node":"@copia","before":"pages/index.html#0","rev":1},
	  {"op":"insert_section","at":{"after":"@copia"},"html":"<section>Nova</section>","ref":"nova"},
	  {"op":"insert_section","at":{"page":"pages/obrigado.html"},"rev":1,"library_key":"faq"},
	  {"op":"remove","node":"@nova"},
	  {"op":"set_token","name":"--color-brand","value":"oklch(0.7 0.1 30)"},
	  {"op":"set_knob","name":"Espaço do topo","value":2.5},
	  {"op":"set_knob","name":"Cor da marca","value":"#ff0066"}
	]`
	batch, refused := prepare(t, htmlProject(), raw)
	if refused != nil {
		t.Fatalf("got %s", failures(refused))
	}
	if len(batch.Operations) != 18 || len(batch.Ignored) != 0 {
		t.Fatalf("got %+v", batch)
	}
	if batch.Operations[7].Value != nil || batch.Operations[6].Value == nil || *batch.Operations[6].Value != "topo" {
		t.Fatal("a missing value removes the attribute")
	}
	if *batch.Operations[5].Item != 2 || batch.Operations[16].Number == nil || *batch.Operations[16].Number != 2.5 || *batch.Operations[17].Value != "#ff0066" {
		t.Fatalf("got %+v", batch.Operations[16])
	}
	encoded, err := json.Marshal(batch.Operations[16])
	if err != nil || string(encoded) != `{"op":"set_knob","name":"Espaço do topo","value":2.5}` {
		t.Fatalf("got %s %v", encoded, err)
	}
}

func TestPrepareBatchTreatsBlankPaddingAsAbsentAndReportsUnknownFields(t *testing.T) {
	batch, refused := prepare(t, htmlProject(), `[
	  {"op":"set_classes","node":" pages/index.html#0 ","rev":1,"add":["px-4"],"breakpoint":"  ","colour":"red"},
	  {"op":"delete_file","path":"scripts/motion.js","rev":1,"content":"x"}
	]`)
	if refused != nil {
		t.Fatalf("got %s", failures(refused))
	}
	op := batch.Operations[0]
	if op.Node != "pages/index.html#0" || op.Breakpoint != "" {
		t.Fatalf("got %+v", op)
	}
	if len(batch.Ignored) != 2 || batch.Ignored[0] != (Ignored{Index: 0, Field: "colour"}) || batch.Ignored[1] != (Ignored{Index: 1, Field: "content"}) {
		t.Fatalf("got %+v", batch.Ignored)
	}
}

func TestPrepareBatchRefusesMalformedBatches(t *testing.T) {
	for _, raw := range []string{``, `{}`, `[]`, `null`, `[1] x`} {
		if _, err := PrepareBatch(htmlProject(), []byte(raw)); !errors.Is(err, ErrBatchMalformed) {
			t.Fatalf("%q: got %v", raw, err)
		}
	}
}

func TestPrepareBatchRefusesInvalidValues(t *testing.T) {
	cases := map[string]struct {
		op   string
		want string
	}{
		"not an object":        {`"write_file"`, "0:operation_invalid:"},
		"unknown op":           {`{"op":"paint"}`, "0:operation_unknown:op"},
		"missing op":           {`{"path":"a"}`, "0:field_required:op"},
		"wrong type":           {`{"op":"write_file","path":"pages/a.html","content":5}`, "0:field_invalid:content"},
		"missing content":      {`{"op":"write_file","path":"pages/a.html"}`, "0:field_required:content"},
		"bad path":             {`{"op":"write_file","path":"../etc/passwd","content":""}`, "0:path_invalid:path"},
		"platform shell":       {`{"op":"write_file","path":"index.html","content":""}`, "0:path_invalid:path"},
		"edit missing file":    {`{"op":"edit_file","path":"pages/x.html","old":"a","new":"b"}`, "0:path_not_found:path"},
		"edit empty old":       {`{"op":"edit_file","path":"pages/index.html","old":"","new":"b"}`, "0:field_required:old"},
		"edit no change":       {`{"op":"edit_file","path":"pages/index.html","old":"a","new":"a"}`, "0:edit_noop:new"},
		"edit placeholder":     {`{"op":"edit_file","path":"pages/index.html","old":"<h1>Oi</h1>","new":"<h1>Olá</h1>\n<!-- ... existing code ... -->"}`, "0:edit_placeholder:new"},
		"write placeholder":    {`{"op":"write_file","path":"scripts/motion.js","content":"import x from \"motion\"\n// rest unchanged\n"}`, "0:edit_placeholder:content"},
		"write pt placeholder": {`{"op":"write_file","path":"scripts/motion.js","content":"/* resto do código */"}`, "0:edit_placeholder:content"},
		"delete required":      {`{"op":"delete_file","path":"site.json"}`, "0:file_required:path"},
		"rename onto existing": {`{"op":"rename_file","path":"pages/obrigado.html","to":"pages/index.html"}`, "0:path_exists:to"},
		"rename required":      {`{"op":"rename_file","path":"theme.css","to":"pages/x.html"}`, "0:file_required:path"},
		"node malformed":       {`{"op":"remove","node":"pages/index.html","rev":1}`, "0:node_invalid:node"},
		"node in stylesheet":   {`{"op":"remove","node":"theme.css#0","rev":1}`, "0:node_invalid:node"},
		"node file missing":    {`{"op":"remove","node":"pages/x.html#0","rev":1}`, "0:path_not_found:node"},
		"missing rev":          {`{"op":"remove","node":"pages/index.html#0"}`, "0:field_required:rev"},
		"future rev":           {`{"op":"remove","node":"pages/index.html#0","rev":2}`, "0:rev_invalid:rev"},
		"zero rev":             {`{"op":"remove","node":"pages/index.html#0","rev":0}`, "0:rev_invalid:rev"},
		"negative item":        {`{"op":"set_text","node":"pages/index.html#0","rev":1,"text":"a","item":-1}`, "0:field_invalid:item"},
		"bad markup":           {`{"op":"set_text","node":"pages/index.html#0","rev":1,"text":"**oi"}`, "0:text_markup_invalid:text"},
		"two paragraphs":       {`{"op":"set_text","node":"pages/index.html#0","rev":1,"text":"a\n\nb"}`, "0:text_markup_invalid:text"},
		"event handler attr":   {`{"op":"set_attr","node":"pages/index.html#0","rev":1,"name":"onclick","value":"x()"}`, "0:attribute_forbidden:name"},
		"annotation attr":      {`{"op":"set_attr","node":"pages/index.html#0","rev":1,"name":"data-vz-node","value":"x#1"}`, "0:attribute_forbidden:name"},
		"class attr":           {`{"op":"set_attr","node":"pages/index.html#0","rev":1,"name":"class","value":"a"}`, "0:attribute_forbidden:name"},
		"bad attr name":        {`{"op":"set_attr","node":"pages/index.html#0","rev":1,"name":"Data X","value":"a"}`, "0:attribute_forbidden:name"},
		"script url":           {`{"op":"set_attr","node":"pages/index.html#0","rev":1,"name":"href","value":"javascript:alert(1)"}`, "0:url_forbidden:value"},
		"password type":        {`{"op":"set_attr","node":"pages/index.html#0","rev":1,"name":"type","value":"Password"}`, "0:credential_field_forbidden:value"},
		"card autocomplete":    {`{"op":"set_attr","node":"pages/index.html#0","rev":1,"name":"autocomplete","value":"cc-number"}`, "0:credential_field_forbidden:value"},
		"external action":      {`{"op":"set_attr","node":"pages/index.html#0","rev":1,"name":"action","value":"https://evil.example"}`, "0:form_action_forbidden:value"},
		"no classes":           {`{"op":"set_classes","node":"pages/index.html#0","rev":1}`, "0:field_required:add"},
		"bad class":            {`{"op":"set_classes","node":"pages/index.html#0","rev":1,"add":["a b"]}`, "0:class_invalid:add"},
		"bad breakpoint":       {`{"op":"set_classes","node":"pages/index.html#0","rev":1,"add":["a"],"breakpoint":"2xl"}`, "0:breakpoint_invalid:breakpoint"},
		"prefixed class":       {`{"op":"set_classes","node":"pages/index.html#0","rev":1,"add":["md:a"],"breakpoint":"md"}`, "0:class_invalid:add"},
		"bad media":            {`{"op":"replace_image","node":"pages/index.html#0","rev":1,"media_id":"../x"}`, "0:media_invalid:media_id"},
		"move two anchors":     {`{"op":"move","node":"pages/index.html#0","rev":1,"before":"pages/index.html#24","after":"pages/index.html#24"}`, "0:position_invalid:before"},
		"move no anchor":       {`{"op":"move","node":"pages/index.html#0","rev":1}`, "0:position_invalid:before"},
		"move onto itself":     {`{"op":"move","node":"pages/index.html#0","rev":1,"after":"pages/index.html#0"}`, "0:position_invalid:after"},
		"move across files":    {`{"op":"move","node":"pages/index.html#0","rev":1,"after":"pages/obrigado.html#0"}`, "0:position_invalid:after"},
		"insert no position":   {`{"op":"insert_section","html":"<p>a</p>"}`, "0:field_required:at"},
		"insert two positions": {`{"op":"insert_section","at":{"before":"pages/index.html#0","page":"pages/index.html"},"rev":1,"html":"<p>a</p>"}`, "0:position_invalid:at"},
		"insert html and key":  {`{"op":"insert_section","at":{"page":"pages/index.html"},"rev":1,"html":"<p>a</p>","library_key":"faq"}`, "0:field_invalid:library_key"},
		"insert nothing":       {`{"op":"insert_section","at":{"page":"pages/index.html"},"rev":1}`, "0:field_required:html"},
		"insert bad key":       {`{"op":"insert_section","at":{"page":"pages/index.html"},"rev":1,"library_key":"FAQ!"}`, "0:library_key_invalid:library_key"},
		"bad ref":              {`{"op":"duplicate","node":"pages/index.html#0","rev":1,"ref":"Cópia"}`, "0:ref_invalid:ref"},
		"unknown ref":          {`{"op":"remove","node":"@fantasma"}`, "0:ref_unknown:node"},
		"ref with rev":         {`{"op":"remove","node":"@fantasma","rev":1}`, "0:ref_unknown:node"},
		"bad token name":       {`{"op":"set_token","name":"color","value":"red"}`, "0:token_invalid:name"},
		"token injection":      {`{"op":"set_token","name":"--a","value":"red; } body { display:none"}`, "0:token_invalid:value"},
		"token url":            {`{"op":"set_token","name":"--a","value":"url(https://x)"}`, "0:token_invalid:value"},
		"unknown knob":         {`{"op":"set_knob","name":"Brilho","value":1}`, "0:knob_unknown:name"},
		"knob out of range":    {`{"op":"set_knob","name":"Espaço do topo","value":9}`, "0:knob_value_invalid:value"},
		"knob wrong type":      {`{"op":"set_knob","name":"Cor da marca","value":3}`, "0:knob_value_invalid:value"},
		"knob value object":    {`{"op":"set_knob","name":"Cor da marca","value":{"a":1}}`, "0:field_invalid:value"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, refused := prepare(t, htmlProject(), "["+c.op+"]")
			if got := failures(refused); got != c.want {
				t.Fatalf("got  %s\nwant %s", got, c.want)
			}
		})
	}
}

func TestPrepareBatchReportsEveryFailureAndSkipsDependents(t *testing.T) {
	raw := `[
	  {"op":"write_file","path":"pages/Novo.html","content":"x"},
	  {"op":"edit_file","path":"pages/Novo.html","old":"x","new":"y"},
	  {"op":"set_attr","node":"pages/index.html#0","rev":1,"name":"onclick","value":"x"},
	  {"op":"set_token","name":"--color-brand","value":"#000"},
	  {"op":"duplicate","node":"pages/obrigado.html#0","rev":9,"ref":"copia"},
	  {"op":"remove","node":"@copia"},
	  {"op":"delete_file","path":"pages/obrigado.html"},
	  {"op":"rename_file","path":"partials/header.html","rev":1,"to":"partials/topo.html"},
	  {"op":"edit_file","path":"partials/topo.html","old":"Logo","new":"Marca"}
	]`
	_, refused := prepare(t, htmlProject(), raw)
	want := "0:path_invalid:path,2:attribute_forbidden:name,4:rev_invalid:rev,1:skipped:0,5:skipped:4,6:skipped:4"
	if got := failures(refused); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestPrepareBatchFollowsTheFileSetThroughTheBatch(t *testing.T) {
	_, refused := prepare(t, htmlProject(), `[
	  {"op":"write_file","path":"pages/novo.html","content":"<p>a</p>"},
	  {"op":"set_text","node":"pages/novo.html#0","rev":1,"text":"b"},
	  {"op":"rename_file","path":"pages/novo.html","to":"pages/outro.html"},
	  {"op":"edit_file","path":"pages/outro.html","old":"b","new":"c"},
	  {"op":"delete_file","path":"pages/outro.html"},
	  {"op":"edit_file","path":"pages/outro.html","old":"c","new":"d"}
	]`)
	if got := failures(refused); got != "5:path_not_found:path" {
		t.Fatalf("got %s", got)
	}
}

func TestPrepareBatchFollowsAKnobsFileWrittenInTheBatch(t *testing.T) {
	_, refused := prepare(t, htmlProject(), `[
	  {"op":"write_file","path":"knobs.json","rev":1,"content":"{\"knobs\":[{\"name\":\"Raio\",\"variable\":\"--radius\",\"kind\":\"range\",\"min\":0,\"max\":2,\"step\":1,\"unit\":\"rem\"}]}"},
	  {"op":"set_knob","name":"Raio","value":1},
	  {"op":"set_knob","name":"Espaço do topo","value":1}
	]`)
	if got := failures(refused); got != "2:knob_unknown:name" {
		t.Fatalf("got %s", got)
	}
}

func TestPrepareBatchChecksReactNodesAndPaths(t *testing.T) {
	_, refused := prepare(t, reactProject(), `[
	  {"op":"set_text","node":"src/pages/Home.jsx#45","rev":1,"text":"Oi","item":0},
	  {"op":"set_classes","node":"src/index.css#0","rev":1,"add":["a"]},
	  {"op":"write_file","path":"src/components/Hero.tsx","content":"export const Hero = () => null\n"},
	  {"op":"write_file","path":"pages/index.html","content":""}
	]`)
	if got := failures(refused); got != "1:node_invalid:node,3:path_invalid:path" {
		t.Fatalf("got %s", got)
	}
}

func TestEveryOperationKindHasAFamily(t *testing.T) {
	families := map[Family]int{}
	for _, kind := range OperationKinds() {
		families[kind.Family()]++
	}
	if len(OperationKinds()) != 14 || families[FamilyCode] != 4 || families[FamilyVisual] != 8 || families[FamilyTheme] != 2 {
		t.Fatalf("got %v", families)
	}
}
