package sitepublishhttp

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
	"vozko/domain/auth"
	"vozko/domain/dnsrecord"
	"vozko/domain/site"
	"vozko/domain/sitepublish"
	"vozko/domain/sitesubmission"
	"vozko/domain/workspace"
	"vozko/infra/http/middleware"
	"vozko/pkg/codegolden"
	"vozko/usecases/siteanalytics"
	"vozko/usecases/siteforms"
	"vozko/usecases/sitepublish"
)

type fakePublisher struct {
	err     error
	actor   sitepublish_usecase.Actor
	added   sitepublish_usecase.AddHostnameInput
	project string
	offline string
}

func (f *fakePublisher) Unpublish(_ context.Context, a sitepublish_usecase.Actor, projectID string) error {
	f.actor, f.offline = a, projectID
	return f.err
}

func (f *fakePublisher) Publish(_ context.Context, a sitepublish_usecase.Actor, projectID string) (sitepublish_usecase.PublishResult, error) {
	f.actor, f.project = a, projectID
	if f.err != nil {
		return sitepublish_usecase.PublishResult{}, f.err
	}
	return sitepublish_usecase.PublishResult{Deployment: sitepublish.DeploymentRecord{ID: "dep-1", Status: sitepublish.DeploymentActive, CreatedAt: time.Now()}, Hostnames: []string{"loja.paginas.test"}, ProjectVersion: 14, RevisionID: "rev-mark", HasUnpublishedChanges: true}, nil
}

func (f *fakePublisher) Rollback(_ context.Context, a sitepublish_usecase.Actor, projectID, deploymentID string) (sitepublish.DeploymentRecord, error) {
	return sitepublish.DeploymentRecord{ID: deploymentID, Status: sitepublish.DeploymentActive}, f.err
}

func (f *fakePublisher) ListDeployments(context.Context, sitepublish_usecase.Actor, string, int) ([]sitepublish.DeploymentRecord, error) {
	return []sitepublish.DeploymentRecord{{ID: "dep-1", Status: sitepublish.DeploymentSuperseded}}, f.err
}

func (f *fakePublisher) ListHostnames(context.Context, sitepublish_usecase.Actor, string) ([]sitepublish.HostnameRecord, error) {
	return nil, f.err
}

func (f *fakePublisher) AddHostname(_ context.Context, _ sitepublish_usecase.Actor, _ string, in sitepublish_usecase.AddHostnameInput) (sitepublish.HostnameRecord, error) {
	f.added = in
	return sitepublish.HostnameRecord{ID: "h1", Hostname: in.Hostname, Kind: in.Kind, Status: sitepublish.HostnamePendingDNS}, f.err
}

func (f *fakePublisher) RemoveHostname(context.Context, sitepublish_usecase.Actor, string, string) (sitepublish.HostnameRecord, error) {
	return sitepublish.HostnameRecord{}, f.err
}

func (f *fakePublisher) VerifyHostname(_ context.Context, a sitepublish_usecase.Actor, projectID, hostnameID string) (sitepublish.HostnameRecord, error) {
	f.actor, f.project = a, projectID
	pending := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	return sitepublish.HostnameRecord{
		ID: hostnameID, Hostname: "www.cliente.com.br", Kind: sitepublish.HostnameCustom, Status: sitepublish.HostnamePendingDNS,
		Verification: sitepublish.Verification{CNAMETarget: "cname.paginas.test", OwnershipToken: "tok", OwnershipStatus: dnsrecord.Pending, PendingSince: &pending},
	}, f.err
}

func (f *fakePublisher) DomainSlots(_ context.Context, a sitepublish_usecase.Actor) (sitepublish_usecase.DomainSlots, error) {
	f.actor = a
	return sitepublish_usecase.DomainSlots{Used: 2, Allowed: 3}, f.err
}

type fakeAnalytics struct{ err error }

func (f fakeAnalytics) Read(context.Context, siteanalytics_usecase.Actor, string, string, string) (siteanalytics_usecase.Report, error) {
	return siteanalytics_usecase.Report{From: "2026-10-01", To: "2026-10-28", Days: []sitepublish.DailyStat{}}, f.err
}

type grant struct {
	resource workspace.Resource
	action   workspace.Action
}

func router(h *Handler) (*mux.Router, *[]grant) {
	r := mux.NewRouter()
	var seen []grant
	RegisterProtectedRoutes(r, h, func(res workspace.Resource, act workspace.Action, next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, req *http.Request) {
			seen = append(seen, grant{res, act})
			next(w, req)
		}
	})
	return r, &seen
}

func request(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	ctx := context.WithValue(req.Context(), middleware.ClaimsContextKey, &auth.Claims{UserID: "u-1"})
	ctx = context.WithValue(ctx, middleware.WorkspaceIDContextKey, "ws-1")
	return req.WithContext(ctx)
}

func TestRoutesAreGuardedByTheRightPermissions(t *testing.T) {
	cases := []struct {
		method, path, body string
		want               grant
		status             int
	}{
		{http.MethodGet, "/sites/projects/p1/deployments", "", grant{"sites", "read"}, http.StatusOK},
		{http.MethodPost, "/sites/projects/p1/deployments", "", grant{"sites", "publish"}, http.StatusCreated},
		{http.MethodPost, "/sites/projects/p1/deployments/d1/activate", "", grant{"sites", "publish"}, http.StatusOK},
		{http.MethodGet, "/sites/projects/p1/hostnames", "", grant{"site_domains", "read"}, http.StatusOK},
		{http.MethodPost, "/sites/projects/p1/hostnames", `{"kind":"custom","hostname":"www.cliente.com.br"}`, grant{"site_domains", "update"}, http.StatusCreated},
		{http.MethodDelete, "/sites/projects/p1/hostnames/h1", "", grant{"site_domains", "update"}, http.StatusOK},
		{http.MethodPost, "/sites/projects/p1/hostnames/h1/verify", "", grant{"site_domains", "update"}, http.StatusOK},
		{http.MethodGet, "/sites/domain-slots", "", grant{"site_domains", "read"}, http.StatusOK},
		{http.MethodGet, "/sites/projects/p1/analytics", "", grant{"sites", "read"}, http.StatusOK},
		{http.MethodPost, "/sites/projects/p1/unpublish", "", grant{"sites", "publish"}, http.StatusNoContent},
	}
	for _, c := range cases {
		r, seen := router(NewHandler(&fakePublisher{}, fakeAnalytics{}, sitepublish.Addresses{Domain: "paginas.test"}))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, request(c.method, c.path, c.body))
		if w.Code != c.status || len(*seen) != 1 || (*seen)[0] != c.want {
			t.Fatalf("%s %s: status %d grants %v body %s", c.method, c.path, w.Code, *seen, w.Body.String())
		}
	}
}

func TestPublishAnswersTheDeploymentAndItsHostnames(t *testing.T) {
	pub := &fakePublisher{}
	r, _ := router(NewHandler(pub, fakeAnalytics{}, sitepublish.Addresses{Domain: "paginas.test"}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, request(http.MethodPost, "/sites/projects/p1/deployments", ""))
	var out PublishResponse
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("%v %s", err, w.Body.String())
	}
	if out.Deployment.Address != "https://deploy--dep-1.paginas.test/" || out.Hostnames[0] != "loja.paginas.test" || pub.actor.WorkspaceID != "ws-1" || pub.project != "p1" || out.ProjectVersion != 14 {
		t.Fatalf("got %+v %+v", out, pub)
	}
}

func TestAddHostnameRefusesUnknownFields(t *testing.T) {
	r, _ := router(NewHandler(&fakePublisher{}, fakeAnalytics{}, sitepublish.Addresses{Domain: "paginas.test"}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, request(http.MethodPost, "/sites/projects/p1/hostnames", `{"kind":"custom","owner":"x"}`))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid_body") {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}

func TestErrorsCarryTheirDetails(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
		detail string
	}{
		{&sitepublish_usecase.IssuesFailure{Kind: sitepublish_usecase.ErrGapsOpen, Issues: []site.Issue{{Code: site.CodeAltMissing, Route: "/"}}}, http.StatusUnprocessableEntity, "gaps_open", "alt_missing"},
		{&sitepublish_usecase.BuildFailure{Errors: []sitepublish_usecase.BuildError{{Path: "index.html", Line: 3, Message: "unexpected"}}}, http.StatusUnprocessableEntity, "build_failed", "unexpected"},
		{sitepublish_usecase.ErrSubscriptionInactive, http.StatusPaymentRequired, "subscription_inactive", ""},
		{fmt.Errorf("%w: %w", sitepublish_usecase.ErrFormsUnavailable, fmt.Errorf("%w: form contato", site.ErrFormChannelRequired)), http.StatusUnprocessableEntity, "form_channel_required", ""},
		{fmt.Errorf("%w: %w", sitepublish_usecase.ErrFormsUnavailable, sitesubmission.ErrSuccessChannelRequired), http.StatusUnprocessableEntity, "form_success_channel_required", ""},
		{fmt.Errorf("%w: %w", sitepublish_usecase.ErrFormsUnavailable, siteforms.ErrWhatsAppNumber), http.StatusUnprocessableEntity, "whatsapp_number_unavailable", ""},
		{fmt.Errorf("route /: %w", siteforms.ErrWhatsAppNumber), http.StatusUnprocessableEntity, "whatsapp_number_unavailable", ""},
		{fmt.Errorf("%w: %w", sitepublish_usecase.ErrFormsUnavailable, siteforms.ErrProjectNotFound), http.StatusNotFound, "site_not_found", ""},
		{fmt.Errorf("%w: pk_x", sitepublish_usecase.ErrWidgetUnknown), http.StatusUnprocessableEntity, "webchat_widget_unknown", ""},
		{fmt.Errorf("%w: boom", sitepublish_usecase.ErrFormsUnavailable), http.StatusServiceUnavailable, "forms_unavailable", ""},
		{errors.New("surprise"), http.StatusServiceUnavailable, "publish_unavailable", ""},
	}
	for _, c := range cases {
		r, _ := router(NewHandler(&fakePublisher{err: c.err}, fakeAnalytics{}, sitepublish.Addresses{Domain: "paginas.test"}))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, request(http.MethodPost, "/sites/projects/p1/deployments", ""))
		if w.Code != c.status || !strings.Contains(w.Body.String(), `"code":"`+c.code+`"`) || !strings.Contains(w.Body.String(), c.detail) {
			t.Fatalf("%v: got %d %s", c.err, w.Code, w.Body.String())
		}
	}
}

func TestAnalyticsRefusesBadPeriods(t *testing.T) {
	r, _ := router(NewHandler(&fakePublisher{}, fakeAnalytics{err: siteanalytics_usecase.ErrInvalidPeriod}, sitepublish.Addresses{Domain: "paginas.test"}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, request(http.MethodGet, "/sites/projects/p1/analytics?from=x", ""))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "analytics_period_invalid") {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}

type fakeModerator struct{ reason string }

func (f *fakeModerator) AdminCases(context.Context, []sitepublish.AbuseStatus) ([]sitepublish_usecase.AdminCase, error) {
	return []sitepublish_usecase.AdminCase{{AbuseCase: sitepublish.AbuseCase{ID: "c1", Status: sitepublish.AbuseHeld}, ProjectName: "Bolos", WorkspaceName: "Loja da Ana"}}, nil
}

func (f *fakeModerator) AdminSuspend(_ context.Context, _, projectID, reason string) (sitepublish.AbuseCase, error) {
	f.reason = reason
	if strings.TrimSpace(reason) == "" {
		return sitepublish.AbuseCase{}, sitepublish_usecase.ErrReasonRequired
	}
	return sitepublish.AbuseCase{ID: "c2", ProjectID: projectID, Status: sitepublish.AbuseSuspended}, nil
}

func (f *fakeModerator) AdminRestore(context.Context, string, string, string) ([]sitepublish.AbuseCase, error) {
	return nil, nil
}

func TestAdminRoutes(t *testing.T) {
	mod := &fakeModerator{}
	r := mux.NewRouter()
	RegisterAdminRoutes(r, NewAdminHandler(mod))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, request(http.MethodGet, "/admin/sites?status=held", ""))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"held"`) || !strings.Contains(w.Body.String(), `"projectName":"Bolos"`) || !strings.Contains(w.Body.String(), `"workspaceName":"Loja da Ana"`) {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, request(http.MethodPost, "/admin/sites/p1/suspend", `{"reason":""}`))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "reason_required") {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, request(http.MethodPost, "/admin/sites/p1/restore", `{"reason":"ok"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}

func TestSiteErrorCodesMatchTheGoldenSoTheFrontTranslatesThem(t *testing.T) {
	codes := response.RefusalCodes(response.ErrorRefusalsOf(refusals), fallbackRefusals)
	codegolden.Compare(t, filepath.Join("testdata", "site_publish_error_codes.golden"), codes,
		"update testdata/site_publish_error_codes.golden, and translate sites.errors.<code> in the pt, en, es and de catalogs of vozko-front")
}

func TestVerifyShowsTheOwnershipRecordAndItsDeadline(t *testing.T) {
	r, _ := router(NewHandler(&fakePublisher{}, fakeAnalytics{}, sitepublish.Addresses{Domain: "paginas.test"}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, request(http.MethodPost, "/sites/projects/p1/hostnames/h1/verify", ""))
	var got HostnameDTO
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	own := got.Verification.Ownership
	if own == nil || own.Type != "TXT" || own.Name != "_vz-domain.www.cliente.com.br" || own.Host != "_vz-domain.www" || own.Value != "vz-domain-verification=tok" || own.Status != "pending" {
		t.Fatalf("ownership = %+v", own)
	}
	if cname := got.Verification.CNAME; cname == nil || cname.Host != "www" || cname.Value != "cname.paginas.test" || got.Verification.Zone != "cliente.com.br" {
		t.Fatalf("cname = %+v zone %s", cname, got.Verification.Zone)
	}
	if got.Verification.PendingUntil == nil || !got.Verification.PendingUntil.Equal(time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("pending until = %v, want 72 hours after the domain was added", got.Verification.PendingUntil)
	}
}

func TestDomainSlotsAnswersUsedAndAllowed(t *testing.T) {
	r, _ := router(NewHandler(&fakePublisher{}, fakeAnalytics{}, sitepublish.Addresses{Domain: "paginas.test"}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, request(http.MethodGet, "/sites/domain-slots", ""))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"used":2`) || !strings.Contains(w.Body.String(), `"allowed":3`) || !strings.Contains(w.Body.String(), `"platformDomain":"paginas.test"`) {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}
