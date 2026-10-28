package sitepublish

import (
	"strings"
	"testing"
)

func query(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

func TestPersonalizeFillsDynamicTextFromTheQuery(t *testing.T) {
	page := []byte(`<!doctype html><html lang="pt-BR"><body><h1>Clareamento em <vz-text param="utm_term" fallback="Curitiba">Curitiba</vz-text></h1></body></html>`)
	out, err := PersonalizePage(page, "", query(map[string]string{"utm_term": "São Paulo"}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `<vz-text param="utm_term" fallback="Curitiba">São Paulo</vz-text>`) {
		t.Fatalf("got %s", out)
	}
	out, _ = PersonalizePage(page, "", query(nil))
	if !strings.Contains(string(out), `fallback="Curitiba">Curitiba</vz-text>`) {
		t.Fatalf("no parameter shows the fallback, got %s", out)
	}
}

func TestPersonalizeEscapesAndTrimsTheVisitorText(t *testing.T) {
	page := []byte(`<p><vz-text param="q" fallback="x"><b>old</b></vz-text></p>`)
	out, _ := PersonalizePage(page, "", query(map[string]string{"q": "<script>alert(1)</script>\x00" + strings.Repeat("a", 100)}))
	text := string(out)
	if strings.Contains(text, "<script>") || strings.Contains(text, "<b>old</b>") || strings.Contains(text, "\x00") {
		t.Fatalf("got %s", text)
	}
	if !strings.Contains(text, "&lt;script&gt;") {
		t.Fatalf("got %s", text)
	}
	inner := text[strings.Index(text, `fallback="x">`)+len(`fallback="x">`) : strings.Index(text, "</vz-text>")]
	if len([]rune(strings.NewReplacer("&lt;", "<", "&gt;", ">").Replace(inner))) != MaxDynamicTextLength {
		t.Fatalf("got %d runes: %s", len([]rune(inner)), inner)
	}
}

func TestPersonalizeMarksTheVariantOnTheRootElement(t *testing.T) {
	page := []byte(`<!doctype html><html lang="pt-BR"><head></head><body>oi</body></html>`)
	out, _ := PersonalizePage(page, "b", query(nil))
	if !strings.Contains(string(out), `<html lang="pt-BR" data-vz-variant="b">`) {
		t.Fatalf("got %s", out)
	}
}

func TestPersonalizeLeavesOtherPagesUntouched(t *testing.T) {
	page := []byte(`<!doctype html><html><body><p class="a">Olá &amp; tchau</p><!-- c --></body></html>`)
	out, err := PersonalizePage(page, "", query(map[string]string{"q": "x"}))
	if err != nil || string(out) != string(page) {
		t.Fatalf("got %s %v", out, err)
	}
}
