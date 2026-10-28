package siteinspect

import (
	"context"
	"errors"
	"strings"
	"testing"

	"vozko/domain/site"
	site_usecase "vozko/usecases/site"
)

type fakePreviews struct {
	result site_usecase.PreviewResult
	err    error
}

func (f fakePreviews) Preview(context.Context, string, string) (site_usecase.PreviewResult, error) {
	return f.result, f.err
}

type fakeDocuments struct {
	pages map[string]string
	err   error
	asked []DocumentRequest
}

func (f *fakeDocuments) Documents(_ context.Context, req DocumentRequest) (map[string]string, error) {
	f.asked = append(f.asked, req)
	return f.pages, f.err
}

func view() *site_usecase.View {
	return &site_usecase.View{
		Project: site_usecase.Project{ID: "p1", WorkspaceID: "w1", Stack: site.StackHTML},
		Files: map[string]site.File{
			site.ManifestPath:     {Content: `{"name":"Clínica","routes":[{"path":"/","page":"pages/index.html"},{"path":"/obrigado","page":"pages/obrigado.html"}]}`},
			"pages/index.html":    {Content: `<form data-vz-form="contato"><input name="lead.phone"></form>`, Rev: 4},
			"pages/obrigado.html": {Content: `<h1>Obrigado</h1>`, Rev: 2},
		},
		Settings: &site_usecase.StoredSettings{Settings: site.Settings{Integrations: site.Integrations{GA4: "G-AB12CD34"}}},
	}
}

const stamp = `<meta name="vz-revs" content="{&quot;pages/index.html&quot;:4,&quot;pages/obrigado.html&quot;:2}">`
const home = `<!doctype html><html><head><title>x</title>` + stamp + `</head><body><main data-vz-node="pages/index.html#0"><img data-vz-node="pages/index.html#9" src="/a.webp"><a data-vz-node="pages/index.html#30" href="/privacidade">Política de Privacidade</a></main></body></html>`
const thanks = `<!doctype html><html><head><title>x</title>` + stamp + `</head><body><h1 data-vz-node="pages/obrigado.html#0">Obrigado</h1></body></html>`

func previewing() fakePreviews {
	return fakePreviews{result: site_usecase.PreviewResult{URL: "https://t.previa.test/", RevisionID: "r1"}}
}

func TestInspectReadsEveryRouteFromTheGivenDocuments(t *testing.T) {
	documents := &fakeDocuments{pages: map[string]string{"/": home, "/obrigado": thanks}}
	r := Service{Previews: previewing()}.Inspect(context.Background(), view(), documents)
	if !r.Known || r.Reason != "" || r.ReasonCode != "" {
		t.Fatalf("report %+v", r)
	}
	if len(documents.asked) != 1 || documents.asked[0].PreviewURL != "https://t.previa.test/" || len(documents.asked[0].Routes) != 2 {
		t.Fatalf("asked %+v", documents.asked)
	}
	codes := map[string]bool{}
	for _, g := range r.Gaps {
		codes[g.Code] = true
	}
	if !codes[site.CodeAltMissing] || !codes[site.CodeConsentBannerMissing] {
		t.Fatalf("gaps %+v", r.Gaps)
	}
	if len(r.Routes) != 2 || r.Routes[0].Route != "/" || !r.Routes[0].Rendered || len(r.Routes[0].Nodes) != 1 || len(r.Routes[0].Nodes[0].Children) != 2 {
		t.Fatalf("routes %+v", r.Routes)
	}
	if len(r.Forms) != 1 || r.Forms[0] != "contato" || r.PreviewVersion != "r1" {
		t.Fatalf("report %+v", r)
	}
}

func TestASubmittedSnapshotIsADocumentSource(t *testing.T) {
	r := Service{Previews: previewing()}.Inspect(context.Background(), view(), Snapshot{"/": home, "/obrigado": thanks})
	if !r.Known || len(r.Routes) != 2 {
		t.Fatalf("report %+v", r)
	}
}

func TestInspectSaysWhyTheGapsCouldNotBeRead(t *testing.T) {
	ok := previewing()
	broken := view()
	broken.Files[site.ManifestPath] = site.File{Content: "{"}
	both := map[string]string{"/": home, "/obrigado": thanks}
	older := strings.Replace(thanks, "&quot;pages/obrigado.html&quot;:2", "&quot;pages/obrigado.html&quot;:1", 1)
	huge := strings.Replace(home, "<title>x</title>", "<title>"+strings.Repeat("x", MaxDocumentBytes)+"</title>", 1)
	cases := map[string]struct {
		svc       Service
		view      *site_usecase.View
		documents Documents
		reason    string
		code      string
		route     string
		build     int
		rules     int
	}{
		"manifest":   {Service{Previews: ok}, broken, &fakeDocuments{pages: both}, ReasonNoManifest, site.CodeManifestInvalid, "", 0, 0},
		"build":      {Service{Previews: fakePreviews{err: &site_usecase.BuildFailedError{Errors: []site_usecase.BuildIssue{{Path: "pages/index.html", Line: 2, Message: "x"}}}}}, view(), &fakeDocuments{pages: both}, ReasonBuildFailed, CodeBuildFailed, "", 1, 0},
		"rules":      {Service{Previews: fakePreviews{err: &site_usecase.IssuesError{Issues: []site.Issue{{Code: site.CodeCSSDirectiveForbidden}}}}}, view(), &fakeDocuments{pages: both}, ReasonRulesBroken, CodeRulesBroken, "", 0, 1},
		"preview":    {Service{Previews: fakePreviews{err: errors.New("down")}}, view(), &fakeDocuments{pages: both}, ReasonNoPreview, CodePreviewUnavailable, "", 0, 0},
		"no source":  {Service{Previews: ok}, view(), nil, ReasonNoCapture, CodeCaptureUnavailable, "", 0, 0},
		"no editor":  {Service{Previews: ok}, view(), &fakeDocuments{err: errors.New("no tab")}, ReasonNoCapture, CodeCaptureUnavailable, "", 0, 0},
		"bad route":  {Service{Previews: ok}, view(), &fakeDocuments{pages: map[string]string{"/": home, "/obrigado": `<html><body><a href="/">x</body></html>`}}, "a rota /obrigado não gerou um HTML legível", CodeRouteUnreadable, "/obrigado", 0, 0},
		"stale":      {Service{Previews: ok}, view(), &fakeDocuments{pages: map[string]string{"/": home, "/obrigado": older}}, ReasonStale, CodeSnapshotStale, "/obrigado", 0, 0},
		"missing":    {Service{Previews: ok}, view(), &fakeDocuments{pages: map[string]string{"/": home}}, ReasonInvalid, CodeSnapshotInvalid, "", 0, 0},
		"extra":      {Service{Previews: ok}, view(), &fakeDocuments{pages: map[string]string{"/": home, "/velha": thanks}}, ReasonInvalid, CodeSnapshotInvalid, "", 0, 0},
		"oversized":  {Service{Previews: ok}, view(), &fakeDocuments{pages: map[string]string{"/": huge, "/obrigado": thanks}}, ReasonInvalid, CodeSnapshotInvalid, "", 0, 0},
		"empty page": {Service{Previews: ok}, view(), &fakeDocuments{pages: map[string]string{"/": "", "/obrigado": thanks}}, ReasonInvalid, CodeSnapshotInvalid, "", 0, 0},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := c.svc.Inspect(context.Background(), c.view, c.documents)
			if r.Known || r.Reason != c.reason || r.ReasonCode != c.code || r.ReasonRoute != c.route || len(r.BuildIssues) != c.build || len(r.RuleIssues) != c.rules {
				t.Fatalf("report %+v", r)
			}
		})
	}
}

func TestASnapshotLargerThanTheTotalBudgetIsRefused(t *testing.T) {
	routes := []string{"/a", "/b", "/c", "/d", "/e"}
	documents := map[string]string{}
	for _, route := range routes {
		documents[route] = strings.Repeat("x", MaxDocumentBytes)
	}
	if err := CheckSnapshot(documents, routes); !errors.Is(err, ErrSnapshotInvalid) {
		t.Fatalf("CheckSnapshot() = %v", err)
	}
	documents["/e"] = "x"
	documents["/d"] = strings.Repeat("x", MaxSnapshotBytes-3*MaxDocumentBytes-1)
	if err := CheckSnapshot(documents, routes); err != nil {
		t.Fatalf("a snapshot inside the budget passes: %v", err)
	}
}
