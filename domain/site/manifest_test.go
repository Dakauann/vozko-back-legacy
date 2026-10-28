package site

import (
	"errors"
	"testing"
)

func TestDecodeManifestReadsNameLanguageAndRoutes(t *testing.T) {
	m, err := DecodeManifest(StackHTML, []byte(htmlSiteJSON))
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "Clínica Sorriso" || m.Lang != "pt-BR" || len(m.Routes) != 2 || m.Routes[1].Path != "/obrigado" || m.Routes[1].Page != "pages/obrigado.html" {
		t.Fatalf("got %+v", m)
	}
	if got := m.RoutePaths(); len(got) != 2 || got[0] != "/" {
		t.Fatalf("got %v", got)
	}
	if route, ok := m.Route("/obrigado"); !ok || route.Page != "pages/obrigado.html" {
		t.Fatalf("got %+v %v", route, ok)
	}
}

func TestDecodeManifestRefusesAnythingItDoesNotUnderstand(t *testing.T) {
	cases := map[string]struct {
		stack Stack
		raw   string
	}{
		"unknown field":      {StackHTML, `{"name":"x","routes":[{"path":"/","page":"pages/index.html"}],"theme":"dark"}`},
		"no name":            {StackHTML, `{"name":"  ","routes":[{"path":"/","page":"pages/index.html"}]}`},
		"long name":          {StackHTML, `{"name":"` + longText(121) + `","routes":[{"path":"/","page":"pages/index.html"}]}`},
		"no routes":          {StackHTML, `{"name":"x","routes":[]}`},
		"no home":            {StackHTML, `{"name":"x","routes":[{"path":"/a","page":"pages/a.html"}]}`},
		"repeated route":     {StackHTML, `{"name":"x","routes":[{"path":"/","page":"pages/index.html"},{"path":"/","page":"pages/a.html"}]}`},
		"bad route path":     {StackHTML, `{"name":"x","routes":[{"path":"/","page":"pages/index.html"},{"path":"/Contato","page":"pages/a.html"}]}`},
		"trailing slash":     {StackHTML, `{"name":"x","routes":[{"path":"/","page":"pages/index.html"},{"path":"/a/","page":"pages/a.html"}]}`},
		"partial as page":    {StackHTML, `{"name":"x","routes":[{"path":"/","page":"partials/header.html"}]}`},
		"react page in html": {StackHTML, `{"name":"x","routes":[{"path":"/","page":"src/pages/Home.jsx"}]}`},
		"css page in react":  {StackReact, `{"name":"x","routes":[{"path":"/","page":"src/index.css"}]}`},
		"bad language":       {StackHTML, `{"name":"x","lang":"portuguese","routes":[{"path":"/","page":"pages/index.html"}]}`},
		"trailing data":      {StackHTML, `{"name":"x","routes":[{"path":"/","page":"pages/index.html"}]} {}`},
		"not json":           {StackHTML, `name: x`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeManifest(c.stack, []byte(c.raw)); !errors.Is(err, ErrManifestInvalid) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestNestedRoutesAreAllowed(t *testing.T) {
	_, err := DecodeManifest(StackReact, []byte(`{"name":"x","routes":[{"path":"/","page":"src/pages/Home.jsx"},{"path":"/servicos/implante-dentario","page":"src/pages/Implante.tsx"}]}`))
	if err != nil {
		t.Fatal(err)
	}
}

func longText(n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = 'a'
	}
	return string(out)
}
