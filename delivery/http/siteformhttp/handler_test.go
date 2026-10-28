package siteformhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"vozko/delivery/http/response"
	"vozko/delivery/http/response/responsetest"
	pfdomain "vozko/domain/publicform"
	"vozko/domain/site"
	"vozko/domain/sitesubmission"
	"vozko/infra/http/middleware"
	"vozko/pkg/codegolden"
	"vozko/usecases/siteforms"
)

type fakeForms struct {
	allowed   bool
	allowErr  error
	challenge pfdomain.Challenge
	result    siteforms.SubmitResult
	err       error
	visitor   siteforms.Visitor
	body      []byte
	page      siteforms.SubmissionPage
	query     siteforms.SubmissionQuery
	pageForms []siteforms.PageForm
	workspace string
}

func (f *fakeForms) AllowedOrigin(context.Context, string, string) (bool, error) {
	return f.allowed, f.allowErr
}

func (f *fakeForms) Challenge(_ context.Context, _ string, v siteforms.Visitor) (pfdomain.Challenge, error) {
	f.visitor = v
	return f.challenge, f.err
}

func (f *fakeForms) Submit(_ context.Context, _ string, v siteforms.Visitor, body []byte) (siteforms.SubmitResult, error) {
	f.visitor, f.body = v, body
	return f.result, f.err
}

func (f *fakeForms) Submissions(_ context.Context, q siteforms.SubmissionQuery) (siteforms.SubmissionPage, error) {
	f.query = q
	return f.page, f.err
}

func publicServer(forms *fakeForms) *mux.Router {
	r := mux.NewRouter()
	RegisterPublicRoutes(r, NewPublicHandler(forms))
	return r
}

func send(r http.Handler, method, path, origin, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("CF-Connecting-IP", "203.0.113.7")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

const pageOrigin = "https://loja.vozko.site"

func TestThePreflightAnswersOnlyThePublishedOrigin(t *testing.T) {
	forms := &fakeForms{allowed: true}
	rec := send(publicServer(forms), http.MethodOptions, "/public/sites/forms/key-1/submissions", pageOrigin, "")
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != pageOrigin || rec.Header().Get("Access-Control-Allow-Credentials") != "" ||
		!strings.Contains(rec.Header().Get("Access-Control-Allow-Methods"), "POST") || rec.Header().Get("Access-Control-Allow-Headers") != "Content-Type" || rec.Header().Get("Vary") != "Origin" {
		t.Fatalf("preflight %d %v", rec.Code, rec.Header())
	}
	forms.allowed = false
	rec = send(publicServer(forms), http.MethodOptions, "/public/sites/forms/key-1/challenge", "https://evil.example", "")
	if rec.Code != http.StatusForbidden || rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("foreign preflight %d %v", rec.Code, rec.Header())
	}
	forms.allowed, forms.allowErr = true, errors.New("db down")
	rec = send(publicServer(forms), http.MethodOptions, "/public/sites/forms/key-1/challenge", pageOrigin, "")
	if rec.Code == http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("an unreadable store must refuse the preflight: %d", rec.Code)
	}
}

func TestTheChallengeIsTheBareObjectTheRuntimeSolves(t *testing.T) {
	expires := time.Date(2026, 10, 28, 12, 2, 0, 0, time.UTC)
	forms := &fakeForms{allowed: true, challenge: pfdomain.Challenge{ID: "c1", Token: "c1.k.16.1.sig", Bits: 16, ExpiresAt: expires}}
	rec := send(publicServer(forms), http.MethodPost, "/public/sites/forms/key-1/challenge", pageOrigin, "{}")
	if rec.Code != http.StatusOK || rec.Header().Get("Access-Control-Allow-Origin") != pageOrigin {
		t.Fatalf("challenge %d %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got["id"] != "c1" || got["token"] != "c1.k.16.1.sig" || got["bits"] != float64(16) || got["expiresAt"] != "2026-10-28T12:02:00Z" {
		t.Fatalf("challenge body %v", got)
	}
	if forms.visitor.Origin != pageOrigin || forms.visitor.IP != "203.0.113.7" || forms.visitor.UserAgent != "Mozilla/5.0" {
		t.Fatalf("visitor %+v", forms.visitor)
	}
	if rec := send(publicServer(forms), http.MethodPost, "/public/sites/forms/key-1/challenge", pageOrigin, `{"x":1}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("a challenge body with fields = %d", rec.Code)
	}
}

func TestASubmissionAnswersTheSuccessActionAndTheFieldsToFix(t *testing.T) {
	forms := &fakeForms{allowed: true, result: siteforms.SubmitResult{SubmissionID: "sub-1", Success: sitesubmission.SuccessFor(&site.Success{Kind: site.SuccessRedirect, Route: "/obrigado"}, "")}}
	rec := send(publicServer(forms), http.MethodPost, "/public/sites/forms/key-1/submissions", pageOrigin, `{"fields":{}}`)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"id":"sub-1","success":{"kind":"redirect","route":"/obrigado"}}` {
		t.Fatalf("submission %d %s", rec.Code, rec.Body.String())
	}
	if string(forms.body) != `{"fields":{}}` {
		t.Fatalf("body passed = %s", forms.body)
	}
	forms.err = &sitesubmission.FieldsError{Details: map[string]string{"lead.phone": "form_phone_invalid"}}
	rec = send(publicServer(forms), http.MethodPost, "/public/sites/forms/key-1/submissions", pageOrigin, `{}`)
	var body struct {
		Code    string            `json:"code"`
		Details map[string]string `json:"details"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusUnprocessableEntity || body.Code != sitesubmission.CodeFieldsInvalid || body.Details["lead.phone"] != "form_phone_invalid" || rec.Header().Get("Access-Control-Allow-Origin") != pageOrigin {
		t.Fatalf("fields %d %s", rec.Code, rec.Body.String())
	}
}

func TestPublicRefusalsCarryTheCodesTheRuntimeReads(t *testing.T) {
	cases := map[error]struct {
		status int
		code   string
	}{
		pfdomain.ErrChallengeReused:                               {http.StatusBadRequest, "challenge_reused"},
		fmt.Errorf("spend: %w", pfdomain.ErrChallengeUnavailable): {http.StatusServiceUnavailable, "form_unavailable"},
		pfdomain.ErrChallengeExpired:                              {http.StatusBadRequest, "challenge_expired"},
		pfdomain.ErrRateLimited:                                   {http.StatusTooManyRequests, "rate_limited"},
		sitesubmission.ErrOriginRefused:                           {http.StatusForbidden, sitesubmission.CodeOriginRefused},
		sitesubmission.ErrFormUnknown:                             {http.StatusNotFound, sitesubmission.CodeFormUnknown},
		sitesubmission.ErrBotSuspected:                            {http.StatusBadRequest, sitesubmission.CodeBotSuspected},
		errors.New("db down"):                                     {http.StatusInternalServerError, CodeInternal},
	}
	for err, want := range cases {
		rec := send(publicServer(&fakeForms{allowed: true, err: err}), http.MethodPost, "/public/sites/forms/key-1/submissions", pageOrigin, `{}`)
		if rec.Code != want.status || responsetest.CodeOf(t, rec) != want.code {
			t.Errorf("%v: %d %s", err, rec.Code, rec.Body.String())
		}
	}
	rec := send(publicServer(&fakeForms{allowed: false, err: sitesubmission.ErrOriginRefused}), http.MethodPost, "/public/sites/forms/key-1/submissions", "https://evil.example", `{}`)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("a foreign origin must not be allowed to read the answer")
	}
	huge := `{"fields":{"a":"` + strings.Repeat("x", sitesubmission.MaxBodyBytes) + `"}}`
	if rec := send(publicServer(&fakeForms{allowed: true}), http.MethodPost, "/public/sites/forms/key-1/submissions", pageOrigin, huge); rec.Code != http.StatusBadRequest {
		t.Fatalf("a huge body = %d", rec.Code)
	}
}

func TestEverySiteFormErrorWritesItsStableCode(t *testing.T) {
	responsetest.RequireEveryErrorWritesItsCode(t, Refusals, writeError)
}

func TestSiteFormErrorCodesMatchTheGolden(t *testing.T) {
	codes := append(response.RefusalCodes(response.ErrorRefusalsOf(Refusals)), sitesubmission.CodeFieldsInvalid, CodeInternal, CodePageInvalid)
	codegolden.Compare(t, filepath.Join("testdata", "site_form_error_codes.golden"), codes,
		"update testdata/site_form_error_codes.golden, and translate sites.errors.<code> in the pt, en, es and de catalogs of vozko-front")
}

func TestSubmissionsAreListedForTheWorkspaceProject(t *testing.T) {
	forms := &fakeForms{page: siteforms.SubmissionPage{Total: 1, Items: []sitesubmission.Submission{{ID: "s1", FormID: "contato", Route: "/", LeadID: "l1",
		Fields: map[string]string{"lead.name": "Ana", "lead.email": "ana@example.com"}, UTM: sitesubmission.UTM{Source: "instagram"}, Consent: sitesubmission.Consent{Granted: true, Text: "Aceito"}, CreatedAt: time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)}}}}
	r := mux.NewRouter()
	h := NewHandler(forms)
	r.HandleFunc("/sites/projects/{id}/submissions", h.List)
	req := httptest.NewRequest(http.MethodGet, "/sites/projects/p1/submissions?form=contato&from=2026-10-01T00:00:00Z&page=2&pageSize=10", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.WorkspaceIDContextKey, "w1"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"formId":"contato"`) || !strings.Contains(rec.Body.String(), `"total":1`) ||
		!strings.Contains(rec.Body.String(), `"name":"Ana","contact":"ana@example.com","source":"instagram"`) || !strings.Contains(rec.Body.String(), `"lead.email":"ana@example.com"`) {
		t.Fatalf("list %d %s", rec.Code, rec.Body.String())
	}
	if forms.query.WorkspaceID != "w1" || forms.query.ProjectID != "p1" || forms.query.FormID != "contato" || forms.query.Page != 2 || forms.query.PageSize != 10 || forms.query.From == nil {
		t.Fatalf("query %+v", forms.query)
	}
	bad := httptest.NewRequest(http.MethodGet, "/sites/projects/p1/submissions?from=yesterday", nil)
	bad = bad.WithContext(context.WithValue(bad.Context(), middleware.WorkspaceIDContextKey, "w1"))
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, bad)
	if rec.Code != http.StatusBadRequest || responsetest.CodeOf(t, rec) != CodePageInvalid {
		t.Fatalf("bad date %d %s", rec.Code, rec.Body.String())
	}
}
