package sitepublish

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"vozko/domain/site"
	"vozko/domain/sitehtml"
)

func page(t *testing.T, route, source string) site.RenderedPage {
	t.Helper()
	doc, err := sitehtml.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	return site.RenderedPage{Route: route, Document: doc}
}

func TestFormIDsComeFromEveryVzFormInTheSource(t *testing.T) {
	project := site.NewProject(site.StackReact, map[string]string{
		"src/pages/Home.tsx":   `<vz-form id="contato"><form></form></vz-form><VzForm/><vz-form class="x" id='orcamento-2'>`,
		"src/pages/About.tsx":  `<vz-form id="contato">`,
		"src/pages/Lead.tsx":   `<form data-vz-form="lead-1">`,
		"src/pages/Agenda.tsx": `<vz-booking class="mt-8" booking="agenda"></vz-booking><vz-booking booking="Bad Id">`,
		"index.html":           `<vz-form id="Bad Id">`,
	})
	if got := FormIDs(project); !reflect.DeepEqual(got, []string{"agenda", "contato", "lead-1", "orcamento-2"}) {
		t.Fatalf("got %v", got)
	}
}

func TestFillPrerenderNeedsExactlyOnePlaceholder(t *testing.T) {
	got, err := FillPrerender(`<div id="root"><!--vz-prerender--></div>`, `<h1>Oi</h1>`)
	if err != nil || got != `<div id="root"><h1>Oi</h1></div>` {
		t.Fatalf("got %q %v", got, err)
	}
	for _, source := range []string{`<div id="root"></div>`, `<!--vz-prerender--><!--vz-prerender-->`} {
		if _, err := FillPrerender(source, "x"); !errors.Is(err, ErrPrerenderPlaceholder) {
			t.Fatalf("%q: got %v", source, err)
		}
	}
}

func TestNeedsRuntimeForPlatformElements(t *testing.T) {
	cases := map[string]bool{
		`<html><body><h1>Oi</h1></body></html>`:                                 false,
		`<html><body><vz-form id="a"><form></form></vz-form></body></html>`:     true,
		`<html><body><vz-consent></vz-consent></body></html>`:                   true,
		`<html><body><vz-popup><dialog></dialog></vz-popup></body></html>`:      true,
		`<html><body><span data-vz-countdown="10"></span></body></html>`:        true,
		`<html><body><vz-whatsapp channel="x">Fale</vz-whatsapp></body></html>`: true,
		`<html><body><a href="/x" data-vz-cta>Comprar</a></body></html>`:        true,
		`<html><body><form data-vz-form="lead"></form></body></html>`:           true,
		`<html><body><vz-booking booking="agenda"></vz-booking></body></html>`:  true,
	}
	for source, want := range cases {
		if got := NeedsRuntime(page(t, "/", source).Document); got != want {
			t.Fatalf("%s: got %v", source, got)
		}
	}
}

func TestStripAppBundleKeepsOnlyThePlatformRuntimeWhenAsked(t *testing.T) {
	source := `<html><head><link rel="modulepreload" href="/assets/vendor-AAAA1111.js"><script type="module" src="/assets/index-5XKZ3QJ2.js"></script><script type="module" src="/assets/vz-runtime-ABCD1234.js"></script><script type="application/ld+json">{}</script></head><body><h1>Oi</h1></body></html>`
	kept, err := StripAppBundle(source, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(kept, "index-5XKZ3QJ2") || strings.Contains(kept, "modulepreload") || !strings.Contains(kept, "vz-runtime-ABCD1234") || !strings.Contains(kept, "ld+json") {
		t.Fatalf("got %s", kept)
	}
	bare, err := StripAppBundle(source, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(bare, "<script type=\"module\"") || !strings.Contains(bare, "<h1>Oi</h1>") {
		t.Fatalf("got %s", bare)
	}
}

func TestOutboundURLsAreTheAbsoluteHTTPDestinations(t *testing.T) {
	p := page(t, "/", `<html><body><a href="https://wa.me/55">a</a><a href="/sobre">b</a><a href="https://wa.me/55">c</a><iframe src="https://www.youtube.com/embed/x"></iframe><a href="mailto:a@b.c">d</a><a href="http://exemplo.com.br/x">e</a></body></html>`)
	got := OutboundURLs([]site.RenderedPage{p})
	want := []string{"http://exemplo.com.br/x", "https://wa.me/55", "https://www.youtube.com/embed/x"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestAbsoluteURLsFindsMediaInHTMLAndCSS(t *testing.T) {
	text := `<img src="https://cdn.example.com/a.png" srcset="https://cdn.example.com/b.webp 2x, https://cdn.example.com/c.webp 3x"> .hero{background:url("https://cdn.example.com/d.jpg")} url(https://cdn.example.com/e.jpg)`
	got := AbsoluteURLs(text)
	want := []string{"https://cdn.example.com/a.png", "https://cdn.example.com/b.webp", "https://cdn.example.com/c.webp", "https://cdn.example.com/d.jpg", "https://cdn.example.com/e.jpg"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestHoldForReviewNeedsATrustedNameAndInputs(t *testing.T) {
	cases := []struct {
		source string
		hold   bool
	}{
		{`<html><head><title>Atualize seu cadastro Itaú</title></head><body><input name="lead.phone"></body></html>`, true},
		{`<html><body><p>Regularize seu CPF na Receita Federal</p><vz-form id="a"><form><input name="lead.email"></form></vz-form></body></html>`, true},
		{`<html><body><p>Pague com PIX</p><select name="field.x"></select></body></html>`, true},
		{`<html><body><p>Rastreie sua encomenda dos Correios</p><textarea name="message"></textarea></body></html>`, true},
		{`<html><body><p>Acesse gov.br</p><input name="name"></body></html>`, true},
		{`<html><body><p>Aceitamos Pix e cartão</p></body></html>`, false},
		{`<html><body><p>Consultoria de marketing</p><input name="email"></body></html>`, false},
		{`<html><body><p>Pixel perfeito</p><input name="email"></body></html>`, false},
	}
	for _, c := range cases {
		reason, hold := HoldForReview([]site.RenderedPage{page(t, "/", c.source)})
		if hold != c.hold || (hold && reason == "") {
			t.Fatalf("%s: got %v %q", c.source, hold, reason)
		}
	}
}

func TestWebchatWidgetsAreTheEmbeddedWidgetIDs(t *testing.T) {
	a := page(t, "/", `<html><body><vz-webchat widget="w2"></vz-webchat><vz-webchat widget="w1"></vz-webchat></body></html>`)
	b := page(t, "/b", `<html><body><vz-webchat widget="w1"></vz-webchat><vz-webchat></vz-webchat></body></html>`)
	if got := WebchatWidgets([]site.RenderedPage{a, b}); !reflect.DeepEqual(got, []string{"w1", "w2"}) {
		t.Fatalf("got %v", got)
	}
}

func TestSettingsDestinationsAreTheExternalRedirectTargets(t *testing.T) {
	settings := site.Settings{Redirects: []site.Redirect{
		{From: "/promo", To: "https://loja.example.com/oferta", Status: 302},
		{From: "/velho", To: "/", Status: 301},
		{From: "/outra", To: "https://loja.example.com/oferta", Status: 301},
		{From: "/banco", To: "https://itau-seguro.example/login", Status: 302},
	}}
	if got := SettingsDestinations(settings); !reflect.DeepEqual(got, []string{"https://itau-seguro.example/login", "https://loja.example.com/oferta"}) {
		t.Fatalf("got %v", got)
	}
}

func TestHoldDestinationsForReviewLooksForTrustedNamesInTheHost(t *testing.T) {
	cases := map[string]bool{
		"https://itau-seguro.example/login":             true,
		"https://atendimento.caixa-tem.example/":        true,
		"https://www.receita-federal.example.net/cpf":   true,
		"https://loja.example.com/pix":                  false,
		"https://vzk.to/abc":                            false,
		"https://wa.me/5511999999999":                   false,
		"https://pixelperfeito.example/":                false,
		"https://nubank.example/promo":                  true,
		"https://loja.example.com/itau-seguro-cadastro": false,
	}
	for target, want := range cases {
		reason, hold := HoldDestinationsForReview([]string{target})
		if hold != want || (hold && !strings.Contains(reason, target)) {
			t.Errorf("%s: got %v %q", target, hold, reason)
		}
	}
	if _, hold := HoldDestinationsForReview(nil); hold {
		t.Fatal("no destinations, no hold")
	}
}
