package site

import (
	"strings"
	"testing"
)

func TestSetFormFieldBecomesValidatedAttributes(t *testing.T) {
	batch, refused := prepare(t, htmlProject(), `[
	  {"op":"set_form_field","node":"pages/index.html#24","rev":1,"field":"lead.email","input":"email","required":true},
	  {"op":"set_form_field","node":"pages/index.html#24","rev":1,"field":"field.interesse","required":false},
	  {"op":"set_form_field","node":"pages/index.html#24","rev":1,"field":"lead.phone"}
	]`)
	if refused != nil {
		t.Fatalf("got %s", failures(refused))
	}
	want := []string{
		`{"op":"set_attr","node":"pages/index.html#24","rev":1,"name":"name","value":"lead.email"}`,
		`{"op":"set_attr","node":"pages/index.html#24","rev":1,"name":"type","value":"email"}`,
		`{"op":"set_attr","node":"pages/index.html#24","rev":1,"name":"required","value":""}`,
		`{"op":"set_attr","node":"pages/index.html#24","rev":1,"name":"name","value":"field.interesse"}`,
		`{"op":"set_attr","node":"pages/index.html#24","rev":1,"name":"required"}`,
		`{"op":"set_attr","node":"pages/index.html#24","rev":1,"name":"name","value":"lead.phone"}`,
	}
	if got := encodedOps(t, batch); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s", strings.Join(got, "\n"))
	}
	if batch.Origins[2].Index != 0 || batch.Origins[2].Op != OpSetFormField || batch.Origins[5].Index != 2 {
		t.Fatalf("got %+v", batch.Origins)
	}
}

func TestSetFormFieldRefusesMappingsTheFormCannotTake(t *testing.T) {
	cases := map[string]string{
		`{"op":"set_form_field","node":"pages/index.html#24","rev":1}`:                                      "0:field_required:field",
		`{"op":"set_form_field","node":"pages/index.html#24","rev":1,"field":"lead.cpf"}`:                   "0:form_field_unknown:field",
		`{"op":"set_form_field","node":"pages/index.html#24","rev":1,"field":"field.Nome Completo"}`:        "0:form_field_unknown:field",
		`{"op":"set_form_field","node":"pages/index.html#24","rev":1,"field":"lead.email","input":"tel"}`:   "0:form_field_type_invalid:input",
		`{"op":"set_form_field","node":"pages/index.html#24","rev":1,"field":"field.a","input":"password"}`: "0:form_field_type_invalid:input",
		`{"op":"set_form_field","node":"pages/index.html#24","rev":1,"field":"field.a","input":"select"}`:   "0:form_field_type_invalid:input",
		`{"op":"set_form_field","node":"pages/index.html#24","rev":1,"field":"consent","required":false}`:   "0:form_field_type_invalid:required",
		`{"op":"set_form_field","node":"pages/index.html#24","rev":1,"field":"field.a","required":"sim"}`:   "0:field_invalid:required",
		`{"op":"set_form_field","node":"pages/index.html#24","field":"field.a"}`:                            "0:field_required:rev",
		`{"op":"set_form_field","node":"theme.css#0","rev":1,"field":"field.a"}`:                            "0:node_invalid:node",
	}
	for op, want := range cases {
		_, refused := prepare(t, htmlProject(), "["+op+"]")
		if got := failures(refused); got != want {
			t.Errorf("%s\ngot  %s\nwant %s", op, got, want)
		}
	}
}

func TestInsertFormFieldWritesTheFieldMarkupItself(t *testing.T) {
	batch, refused := prepare(t, htmlProject(), `[
	  {"op":"insert_form_field","at":{"after":"pages/index.html#24"},"rev":1,"field":"lead.phone","input":"tel","text":"Seu <WhatsApp>","required":true,"ref":"tel"},
	  {"op":"insert_form_field","at":{"into":"pages/index.html#0"},"rev":1,"field":"consent","input":"checkbox","text":"Aceito receber contato"},
	  {"op":"insert_form_field","at":{"before":"pages/index.html#24"},"rev":1,"field":"field.mensagem","input":"textarea","text":"Mensagem"}
	]`)
	if refused != nil {
		t.Fatalf("got %s", failures(refused))
	}
	ops := batch.Operations
	if len(ops) != 3 || ops[0].Op != OpInsertSection || ops[0].Ref != "tel" || ops[0].At.After != "pages/index.html#24" || ops[0].Rev != 1 {
		t.Fatalf("got %v", encodedOps(t, batch))
	}
	want := []string{
		`<label class="` + formFieldRowClass + `"><span>Seu &lt;WhatsApp&gt;</span><input class="` + formFieldInputClass + `" type="tel" name="lead.phone" required></label>`,
		`<label class="` + formFieldCheckRowClass + `"><input type="checkbox" name="consent" required><span>Aceito receber contato</span></label>`,
		`<label class="` + formFieldRowClass + `"><span>Mensagem</span><textarea class="` + formFieldInputClass + `" name="field.mensagem" rows="3"></textarea></label>`,
	}
	for i, html := range want {
		if ops[i].HTML != html {
			t.Fatalf("%d: got %s", i, ops[i].HTML)
		}
	}
	if batch.Origins[1].Op != OpInsertFormField {
		t.Fatalf("got %+v", batch.Origins)
	}
}

func TestInsertFormFieldRefusesBadFields(t *testing.T) {
	long := strings.Repeat("a", MaxFormLabelRunes+1)
	cases := map[string]string{
		`{"op":"insert_form_field","at":{"after":"pages/index.html#24"},"rev":1,"input":"text","text":"Nome"}`:                             "0:field_required:field",
		`{"op":"insert_form_field","at":{"after":"pages/index.html#24"},"rev":1,"field":"lead.name","text":"Nome"}`:                        "0:field_required:input",
		`{"op":"insert_form_field","at":{"after":"pages/index.html#24"},"rev":1,"field":"lead.name","input":"email","text":"Nome"}`:        "0:form_field_type_invalid:input",
		`{"op":"insert_form_field","at":{"after":"pages/index.html#24"},"rev":1,"field":"field.a","input":"radio","text":"A"}`:             "0:form_field_type_invalid:input",
		`{"op":"insert_form_field","at":{"after":"pages/index.html#24"},"rev":1,"field":"lead.name","input":"text"}`:                       "0:field_required:text",
		`{"op":"insert_form_field","at":{"after":"pages/index.html#24"},"rev":1,"field":"lead.name","input":"text","text":"` + long + `"}`: "0:form_label_too_long:text",
		`{"op":"insert_form_field","rev":1,"field":"lead.name","input":"text","text":"Nome"}`:                                              "0:field_required:at",
	}
	for op, want := range cases {
		_, refused := prepare(t, htmlProject(), "["+op+"]")
		if got := failures(refused); got != want {
			t.Errorf("%s\ngot  %s\nwant %s", op, got, want)
		}
	}
}

func TestFormFieldOperationsAreShorthand(t *testing.T) {
	for _, kind := range []OperationKind{OpSetFormField, OpInsertFormField} {
		if kind.Family() != FamilyShorthand {
			t.Fatalf("%s is %s", kind, kind.Family())
		}
	}
}
