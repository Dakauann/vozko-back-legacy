package site

import (
	"testing"

	"vozko/domain/sitehtml"
)

func TestFormOutlineListsEachFormWithTheFieldsTheEditorCanChange(t *testing.T) {
	doc, err := sitehtml.Parse(`<main data-vz-node="pages/index.html#0">
<form data-vz-form="contato" data-vz-node="pages/index.html#1">
  <label data-vz-node="pages/index.html#2"><span data-vz-node="pages/index.html#3">Nome</span><input data-vz-node="pages/index.html#4" name="lead.name" required></label>
  <input data-vz-node="pages/index.html#5" type="email" name="lead.email">
  <input name="field.sem_no">
  <button data-vz-node="pages/index.html#6" type="submit">Enviar</button>
</form>
<vz-form id="vazio" data-vz-node="pages/index.html#7"><input type="submit" data-vz-node="pages/index.html#8"></vz-form>
<form data-vz-node="pages/index.html#9"><input data-vz-node="pages/index.html#10" name="q"></form>
</main>`)
	if err != nil {
		t.Fatal(err)
	}
	got := FormOutline(doc)
	want := `[{"id":"contato","node":"pages/index.html#1","submit":"pages/index.html#6","fields":[` +
		`{"node":"pages/index.html#4","row":"pages/index.html#2","labelNode":"pages/index.html#3","name":"lead.name","kind":"text","required":true,"label":"Nome"},` +
		`{"node":"pages/index.html#5","row":"pages/index.html#5","name":"lead.email","kind":"email"}]},` +
		`{"id":"vazio","node":"pages/index.html#7","submit":"pages/index.html#8","fields":[]}]`
	if jsonOf(t, got) != want {
		t.Fatalf("got\n%s", jsonOf(t, got))
	}
}

func TestFormOutlineIsEmptyWithoutForms(t *testing.T) {
	doc, err := sitehtml.Parse(`<main data-vz-node="pages/index.html#0"><p>x</p></main>`)
	if err != nil {
		t.Fatal(err)
	}
	if got := FormOutline(doc); got == nil || len(got) != 0 {
		t.Fatalf("got %#v", got)
	}
}
