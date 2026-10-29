package siteform

import (
	"slices"
	"strings"
	"testing"

	"vozko/domain/customfield"
)

func kindNames(kinds []Kind) string {
	out := make([]string, 0, len(kinds))
	for _, k := range kinds {
		out = append(out, string(k))
	}
	return strings.Join(out, ",")
}

func TestTargetKindsListWhatEachLeadTargetAccepts(t *testing.T) {
	cases := map[string]string{
		TargetName:       "text",
		TargetPhone:      "tel,text",
		TargetEmail:      "email",
		TargetConsent:    "checkbox",
		TargetEmailOptIn: "checkbox",
		"field.idade":    "text,textarea,number,date,email,tel,url,select,radio,checkbox,hidden",
		"field.Idade":    "",
		"lead.cpf":       "",
		"":               "",
	}
	for target, want := range cases {
		if got := kindNames(TargetKinds(target)); got != want {
			t.Errorf("%q: got %s want %s", target, got, want)
		}
	}
}

func TestTargetKindsAgreeWithTheManifestRule(t *testing.T) {
	for _, target := range []string{TargetName, TargetPhone, TargetEmail, TargetConsent, TargetEmailOptIn, "field.x"} {
		for kind := range knownKinds {
			if kindFits(target, kind) != slices.Contains(TargetKinds(target), kind) {
				t.Errorf("%s %s disagrees", target, kind)
			}
		}
	}
}

func TestLeadTargetsAreListedInFormOrder(t *testing.T) {
	if got := strings.Join(LeadTargets(), ","); got != "lead.name,lead.phone,lead.email,consent,email_opt_in" {
		t.Fatalf("got %s", got)
	}
}

const builtForm = `<form data-vz-form="contato" data-vz-node="pages/index.html#1">
  <label data-vz-node="pages/index.html#2" class="grid"><span data-vz-node="pages/index.html#3">Seu nome</span><input data-vz-node="pages/index.html#4" name="lead.name" required></label>
  <div data-vz-node="pages/index.html#5"><label data-vz-node="pages/index.html#6" for="w">WhatsApp</label><input data-vz-node="pages/index.html#7" id="w" type="tel" name="lead.phone"></div>
  <label data-vz-node="pages/index.html#8">E-mail <input data-vz-node="pages/index.html#9" type="email" name="lead.email"></label>
  <textarea data-vz-node="pages/index.html#10" name="field.Obs"></textarea>
  <label data-vz-node="pages/index.html#11"><input data-vz-node="pages/index.html#12" type="checkbox" name="consent"><span data-vz-node="pages/index.html#13">Aceito</span></label>
  <input type="hidden" name="vz_website">
  <button data-vz-node="pages/index.html#14" type="submit">Enviar</button>
</form>`

func TestReadControlDescribesEachFieldForTheEditor(t *testing.T) {
	doc := parse(t, builtForm)
	node := func(i int) string {
		if i < 0 {
			return ""
		}
		return doc.Elements[i].Node
	}
	var got []string
	for i := range doc.Elements {
		c, ok := ReadControl(doc, i)
		if !ok {
			continue
		}
		got = append(got, strings.Join([]string{node(i), c.Name, string(c.Kind), map[bool]string{true: "required", false: "optional"}[c.Required], c.Label, node(c.LabelTextAt), node(c.RowAt)}, "|"))
	}
	want := []string{
		"pages/index.html#4|lead.name|text|required|Seu nome|pages/index.html#3|pages/index.html#2",
		"pages/index.html#7|lead.phone|tel|optional|WhatsApp|pages/index.html#6|pages/index.html#5",
		"pages/index.html#9|lead.email|email|optional|E-mail||pages/index.html#8",
		"pages/index.html#10|field.Obs|textarea|optional|||pages/index.html#10",
		"pages/index.html#12|consent|checkbox|required|Aceito|pages/index.html#13|pages/index.html#11",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s", strings.Join(got, "\n"))
	}
}

func TestReadControlSkipsButtonsAndOtherElements(t *testing.T) {
	doc := parse(t, `<form data-vz-form="a"><input type="submit"><input type="image"><button>Ok</button><p>x</p></form>`)
	for i := range doc.Elements {
		if c, ok := ReadControl(doc, i); ok {
			t.Fatalf("got %+v", c)
		}
	}
}

func TestCustomFieldKindsFollowTheLeadFieldType(t *testing.T) {
	cases := map[customfield.FieldType]string{
		customfield.TypeDate:    "date,text",
		customfield.TypeBoolean: "checkbox",
		customfield.TypeSelect:  "select,radio",
		"unknown":               "",
	}
	for kind, want := range cases {
		if got := kindNames(CustomFieldKinds(kind)); got != want {
			t.Errorf("%s: got %s", kind, got)
		}
	}
	for _, kind := range customFieldKinds {
		for _, k := range kind {
			if !slices.Contains(TargetKinds("field.x"), k) {
				t.Errorf("%s is not a field kind", k)
			}
		}
	}
}

func TestCustomTargetsKeepOnlyTheLeadFieldsAFormMayFill(t *testing.T) {
	got := CustomTargets([]*customfield.Definition{
		{Key: "cidade", Label: "Cidade", Type: customfield.TypeText, ObjectType: customfield.ObjectLead},
		{Key: "cpf", Label: "CPF", Type: customfield.TypeText, ObjectType: customfield.ObjectLead, Sensitive: true},
		{Key: "valor", Label: "Valor", Type: customfield.TypeNumber, ObjectType: customfield.ObjectOpportunity},
		{Key: "Mal Formado", Label: "x", Type: customfield.TypeText, ObjectType: customfield.ObjectLead},
		{Key: "tipo", Label: "Tipo", Type: "desconhecido", ObjectType: customfield.ObjectLead},
		nil,
	})
	if len(got) != 1 || got[0].Target != "field.cidade" || got[0].Label != "Cidade" || got[0].Kinds[0] != KindText {
		t.Fatalf("got %+v", got)
	}
}
