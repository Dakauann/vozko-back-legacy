package siteform

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"vozko/domain/sitehtml"
)

const contactForm = `<main>
<form data-vz-form="contato" data-vz-node="src/components/Form.jsx#10" class="grid gap-4">
  <label>Nome <input name="lead.name" required maxlength="80"></label>
  <label for="tel">WhatsApp</label>
  <input id="tel" type="tel" name="lead.phone" required>
  <input type="email" name="lead.email">
  <select name="field.interesse"><option value="">Escolha</option><option value="implante">Implante</option><option>Clareamento</option></select>
  <label><input type="radio" name="field.periodo" value="manha"> Manhã</label>
  <label><input type="radio" name="field.periodo" value="tarde"> Tarde</label>
  <textarea name="field.obs"></textarea>
  <input type="hidden" name="field.origem" value="landing">
  <label><input type="checkbox" name="consent"> Concordo com a <a href="/privacidade">Política de Privacidade</a>.</label>
  <label><input type="checkbox" name="email_opt_in"> Quero receber novidades por e-mail</label>
  <button type="submit" name="enviar">Enviar</button>
</form>
</main>`

func parse(t *testing.T, source string) *sitehtml.Document {
	t.Helper()
	doc, err := sitehtml.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestExtractReadsTheFormManifestFromRenderedHTML(t *testing.T) {
	manifests, problems := Extract(parse(t, contactForm))
	if len(problems) != 0 {
		t.Fatalf("got %+v", problems)
	}
	if len(manifests) != 1 {
		t.Fatalf("got %+v", manifests)
	}
	m := manifests[0]
	if m.ID != "contato" || m.Node != "src/components/Form.jsx#10" {
		t.Fatalf("got %+v", m)
	}
	if m.ConsentText != "Concordo com a Política de Privacidade." || m.EmailOptInText != "Quero receber novidades por e-mail" {
		t.Fatalf("got consent %q opt in %q", m.ConsentText, m.EmailOptInText)
	}
	got := []string{}
	for _, f := range m.Fields {
		got = append(got, f.Name+":"+string(f.Kind))
	}
	want := "lead.name:text,lead.phone:tel,lead.email:email,field.interesse:select,field.periodo:radio,field.obs:textarea,field.origem:hidden,consent:checkbox,email_opt_in:checkbox"
	if strings.Join(got, ",") != want {
		t.Fatalf("got %s", strings.Join(got, ","))
	}
	name, _ := m.Field("lead.name")
	if !name.Required || name.MaxLength != 80 || name.Label != "Nome" {
		t.Fatalf("got %+v", name)
	}
	phone, _ := m.Field("lead.phone")
	if phone.Label != "WhatsApp" || !phone.Required {
		t.Fatalf("got %+v", phone)
	}
	interest, _ := m.Field("field.interesse")
	if strings.Join(interest.Options, ",") != ",implante,Clareamento" {
		t.Fatalf("got %+v", interest.Options)
	}
	period, _ := m.Field("field.periodo")
	if strings.Join(period.Options, ",") != "manha,tarde" {
		t.Fatalf("got %+v", period.Options)
	}
	origin, _ := m.Field("field.origem")
	if origin.Value != "landing" {
		t.Fatalf("got %+v", origin)
	}
	consent, _ := m.Field("consent")
	if !consent.Required {
		t.Fatal("consent is always required")
	}
	if !m.HasIdentity() {
		t.Fatal("identity")
	}
	if keys := m.fieldKeys(); strings.Join(keys, ",") != "interesse,periodo,obs,origem" {
		t.Fatalf("got %v", keys)
	}
}

func TestExtractAcceptsThePlatformElementMarker(t *testing.T) {
	manifests, problems := Extract(parse(t, `<vz-form id="newsletter"><input type="email" name="lead.email" required></vz-form><form action="/busca"><input name="q"></form>`))
	if len(problems) != 0 || len(manifests) != 1 || manifests[0].ID != "newsletter" || manifests[0].ConsentText != "" {
		t.Fatalf("got %+v %+v", manifests, problems)
	}
}

func TestExtractReportsEveryBrokenFormContract(t *testing.T) {
	cases := map[string]struct {
		html string
		code string
	}{
		"bad id":             {`<form data-vz-form="Contato!"><input name="lead.name"></form>`, CodeFormInvalid},
		"vz form without id": {`<vz-form><input name="lead.name"></vz-form>`, CodeFormInvalid},
		"nested forms":       {`<form data-vz-form="a"><vz-form id="b"></vz-form></form>`, CodeFormInvalid},
		"unknown target":     {`<form data-vz-form="a"><input name="cpf"></form>`, CodeFormFieldUnknown},
		"unknown lead field": {`<form data-vz-form="a"><input name="lead.cpf"></form>`, CodeFormFieldUnknown},
		"bad field key":      {`<form data-vz-form="a"><input name="field.Com Espaço"></form>`, CodeFormFieldUnknown},
		"empty field key":    {`<form data-vz-form="a"><input name="field."></form>`, CodeFormFieldUnknown},
		"file input":         {`<form data-vz-form="a"><input type="file" name="field.doc"></form>`, CodeFormFieldTypeInvalid},
		"email as text":      {`<form data-vz-form="a"><input type="text" name="lead.email"></form>`, CodeFormFieldTypeInvalid},
		"consent as text":    {`<form data-vz-form="a"><input name="consent"></form>`, CodeFormFieldTypeInvalid},
		"repeated field":     {`<form data-vz-form="a"><input name="lead.name"><input name="lead.name"></form>`, CodeFormFieldDuplicate},
		"radio mixed kinds":  {`<form data-vz-form="a"><input type="radio" name="field.x" value="1"><input name="field.x"></form>`, CodeFormFieldDuplicate},
		"repeated form id":   {`<form data-vz-form="a"><input name="lead.name"></form><vz-form id="a"><input name="lead.name"></vz-form>`, CodeFormIDDuplicate},
		"bad maxlength":      {`<form data-vz-form="a"><input name="lead.name" maxlength="dez"></form>`, CodeFormFieldTypeInvalid},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, problems := Extract(parse(t, c.html))
			if len(problems) == 0 || problems[0].Code != c.code {
				t.Fatalf("got %+v", problems)
			}
		})
	}
}

func TestExtractListsAllProblemsAtOnce(t *testing.T) {
	_, problems := Extract(parse(t, `<form data-vz-form="a"><input name="cpf"><input type="file" name="field.doc"></form>`))
	if len(problems) != 2 || problems[0].Form != "a" || problems[0].Field != "cpf" || problems[1].Field != "field.doc" {
		t.Fatalf("got %+v", problems)
	}
}

func TestManifestsRoundTripThroughStrictJSON(t *testing.T) {
	manifests, _ := Extract(parse(t, contactForm))
	raw, err := json.Marshal(manifests[0])
	if err != nil {
		t.Fatal(err)
	}
	back, err := DecodeManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := json.Marshal(back)
	if string(again) != string(raw) {
		t.Fatalf("got %s\nwant %s", again, raw)
	}
	for name, bad := range map[string]string{
		"unknown field":  `{"id":"a","fields":[],"extra":1}`,
		"bad id":         `{"id":"A B","fields":[]}`,
		"unknown target": `{"id":"a","fields":[{"name":"cpf","kind":"text"}]}`,
		"bad kind":       `{"id":"a","fields":[{"name":"lead.name","kind":"file"}]}`,
		"empty":          ``,
	} {
		if _, err := DecodeManifest([]byte(bad)); !errors.Is(err, ErrManifestInvalid) {
			t.Fatalf("%s: got %v", name, err)
		}
	}
}

func TestExtractIgnoresTheRuntimeTrapFieldButNoLookalike(t *testing.T) {
	source := `<form data-vz-form="contato"><input name="lead.phone" type="tel"><div aria-hidden="true"><input type="text" name="` + TrapField + `" tabindex="-1"></div></form>`
	manifests, problems := Extract(parse(t, source))
	if len(problems) != 0 || len(manifests) != 1 || len(manifests[0].Fields) != 1 || manifests[0].Fields[0].Name != TargetPhone {
		t.Fatalf("got %+v %+v", manifests, problems)
	}
	_, problems = Extract(parse(t, `<form data-vz-form="contato"><input name="vz_website2"></form>`))
	if len(problems) != 1 || problems[0].Code != CodeFormFieldUnknown {
		t.Fatalf("got %+v", problems)
	}
}
