package sitehttp

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"vozko/domain/site"
	workspace_domain "vozko/domain/workspace"
	site_usecase "vozko/usecases/site"
	"vozko/usecases/siteinspect"
)

type fakeInspector struct {
	report siteinspect.Report
	seen   *site_usecase.View
}

func (f *fakeInspector) Outline(_ context.Context, v *site_usecase.View) siteinspect.Report {
	f.seen = v
	return f.report
}

func TestOutlineAnswersTheInspectionOfTheWorkingVersion(t *testing.T) {
	svc := newFake()
	inspector := &fakeInspector{report: siteinspect.Report{
		Known:  true,
		Routes: []site.RouteOutline{{Route: "/", Rendered: true, Nodes: []site.OutlineNode{{Node: "pages/index.html#0", Tag: "main", Path: "html[0]/body[0]/main[0]"}}}},
		Forms:  []string{"contato"},
		Gaps:   []site.Issue{{Code: site.CodeAltMissing, Route: "/", Node: "pages/index.html#9"}},
	}}
	r, grants := routerWith(svc, inspector)
	rec := call(r, http.MethodGet, "/sites/projects/p-1/outline", "", nil)
	if rec.Code != http.StatusOK || grants["GET /sites/projects/{id}/outline"] != (grant{workspace_domain.ResourceSites, workspace_domain.ActionRead}) {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	var body OutlineResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.GapsChecked || len(body.OpenGaps) != 1 || body.Routes[0].Nodes[0].Node != "pages/index.html#0" || body.Forms[0] != "contato" || inspector.seen == nil {
		t.Fatalf("outline %+v", body)
	}
	inspector.report = siteinspect.Report{Reason: siteinspect.ReasonBuildFailed, ReasonCode: siteinspect.CodeBuildFailed, BuildIssues: []site_usecase.BuildIssue{{Path: "pages/index.html", Line: 3, Message: "x"}}}
	rec = call(r, http.MethodGet, "/sites/projects/p-1/outline", "", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusOK || body.GapsChecked || body.ReasonCode != siteinspect.CodeBuildFailed || len(body.BuildErrors) != 1 || body.OpenGaps == nil {
		t.Fatalf("unread outline %d %s", rec.Code, rec.Body.String())
	}
}
