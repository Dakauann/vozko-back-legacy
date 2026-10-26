package attendance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/gorilla/mux"

	attendancedomain "vozko/domain/attendance"
	"vozko/domain/cache"
	workspace_domain "vozko/domain/workspace"
	dept "vozko/domain/workspace/workspace_department"
	"vozko/infra/http/middleware"
)

type stubServiceMessages struct {
	err       error
	calls     int
	last      attendancedomain.ServiceMessageQuery
	available []attendancedomain.Section
}

func (s *stubServiceMessages) AvailableSections(context.Context, string) []attendancedomain.Section {
	return s.available
}

func (s *stubServiceMessages) ServiceMessages(_ context.Context, _ string, q attendancedomain.ServiceMessageQuery) (*attendancedomain.ServiceMessagesSection, error) {
	s.calls++
	s.last = q
	if s.err != nil {
		return nil, s.err
	}
	total := int64(12)
	return &attendancedomain.ServiceMessagesSection{Channel: "whatsapp", Applicable: true, Total: &total, Buckets: []attendancedomain.ServiceMessageBucket{{Bucket: "2026-10-01", Messages: 12}}}, nil
}

var everyDepartment = &dept.DepartmentFilter{IsOwnerOrAdmin: true}

func serveServiceMessages(t *testing.T, sections *stubSections, messages *stubServiceMessages, scope *dept.DepartmentFilter, path string) *httptest.ResponseRecorder {
	t.Helper()
	router := mux.NewRouter()
	allow := func(_ workspace_domain.Resource, _ workspace_domain.Action, next http.HandlerFunc) http.HandlerFunc {
		return next
	}
	RegisterProtectedRoutes(router, &AttendanceHandler{sections: sections, serviceMessages: messages}, allow, func(next http.HandlerFunc) http.HandlerFunc { return next })
	req := httptest.NewRequest(http.MethodGet, path, nil)
	ctx := context.WithValue(req.Context(), middleware.WorkspaceIDContextKey, "ws1")
	if scope != nil {
		ctx = context.WithValue(ctx, middleware.DepartmentFilterContextKey, scope)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req.WithContext(ctx))
	return rec
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %s", rec.Body.String())
	}
	return body.Code
}

const serviceMessagesPath = "/attendance/overview/service_messages?date_from=2026-10-01&date_to=2026-10-07"

func TestAPayingWorkspaceReadsItsServiceMessagesWithEveryFilter(t *testing.T) {
	messages := &stubServiceMessages{}
	rec := serveServiceMessages(t, &stubSections{}, messages, everyDepartment,
		serviceMessagesPath+"&department_id=d1&member_id=m1&campaign_id=c1&channel=whatsapp&origin=receptive")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	f := messages.last.Filter
	if f.DateFrom == nil || f.DateTo == nil || f.DepartmentID != "d1" || f.MemberID != "m1" || f.CampaignID != "c1" || f.Channel != "whatsapp" || f.Origin != "receptive" {
		t.Fatalf("filter = %+v", f)
	}
	if !slices.Equal(messages.last.Departments, []string{"d1"}) {
		t.Fatalf("departments = %v, want the chosen one", messages.last.Departments)
	}
	var body map[string]json.RawMessage
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if string(body["total"]) != "12" {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestTheGenericSectionRouteNeverServesServiceMessages(t *testing.T) {
	sections := &stubSections{}
	messages := &stubServiceMessages{err: attendancedomain.ErrServiceMessagesNotOnPlan}
	rec := serveServiceMessages(t, sections, messages, everyDepartment, serviceMessagesPath)
	if rec.Code != http.StatusForbidden || messages.calls != 1 || sections.summaries != 0 {
		t.Fatalf("status = %d, gated calls = %d; the service message path must go through the plan gate", rec.Code, messages.calls)
	}
}

func TestAWorkspaceThatDoesNotPayIsRefusedWithItsCodeAndNoData(t *testing.T) {
	rec := serveServiceMessages(t, &stubSections{}, &stubServiceMessages{err: attendancedomain.ErrServiceMessagesNotOnPlan}, everyDepartment, serviceMessagesPath)
	if rec.Code != http.StatusForbidden || errorCode(t, rec) != CodeServiceMessagesNotOnPlan {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]json.RawMessage
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	for _, leaked := range []string{"total", "buckets", "spend", "data"} {
		if _, ok := body[leaked]; ok {
			t.Fatalf("the refusal carries %q: %s", leaked, rec.Body.String())
		}
	}
}

func TestAnUnresolvedPriceIsRefusedWithItsCode(t *testing.T) {
	err := fmt.Errorf("%w: plan lookup failed", attendancedomain.ErrServiceMessagePriceUnavailable)
	rec := serveServiceMessages(t, &stubSections{}, &stubServiceMessages{err: err}, everyDepartment, serviceMessagesPath)
	if rec.Code != http.StatusServiceUnavailable || errorCode(t, rec) != CodeServiceMessagesPriceUnavailable {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestAMemberOutsideTheChosenDepartmentReadsNothing(t *testing.T) {
	member := &dept.DepartmentFilter{DepartmentIDs: []string{"d1"}, WorkspaceHasDepartments: true}
	stranded := &dept.DepartmentFilter{WorkspaceHasDepartments: true}
	for name, tc := range map[string]struct {
		scope *dept.DepartmentFilter
		query string
	}{
		"another department": {member, "&department_id=d9"},
		"no department":      {stranded, ""},
		"no resolved scope":  {nil, ""},
	} {
		t.Run(name, func(t *testing.T) {
			messages := &stubServiceMessages{}
			rec := serveServiceMessages(t, &stubSections{}, messages, tc.scope, serviceMessagesPath+tc.query)
			if rec.Code != http.StatusForbidden || errorCode(t, rec) != CodeDepartmentForbidden || messages.calls != 0 {
				t.Fatalf("status = %d calls = %d body = %s", rec.Code, messages.calls, rec.Body.String())
			}
		})
	}
}

func TestAMemberWithoutAChoiceReadsTheirOwnDepartments(t *testing.T) {
	messages := &stubServiceMessages{}
	member := &dept.DepartmentFilter{DepartmentIDs: []string{"d1", "d2"}, WorkspaceHasDepartments: true}
	rec := serveServiceMessages(t, &stubSections{}, messages, member, serviceMessagesPath)
	if rec.Code != http.StatusOK || !slices.Equal(messages.last.Departments, []string{"d1", "d2"}) {
		t.Fatalf("status = %d departments = %v", rec.Code, messages.last.Departments)
	}
}

func TestServiceMessagesRefuseAMissingPeriodWithItsCode(t *testing.T) {
	err := fmt.Errorf("%w: no start", attendancedomain.ErrInvalidWindow)
	rec := serveServiceMessages(t, &stubSections{}, &stubServiceMessages{err: err}, everyDepartment, "/attendance/overview/service_messages")
	if rec.Code != http.StatusBadRequest || errorCode(t, rec) != CodeServiceMessagesPeriodInvalid {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestServiceMessagesRefuseAnUnknownOriginBeforeReading(t *testing.T) {
	messages := &stubServiceMessages{}
	rec := serveServiceMessages(t, &stubSections{}, messages, everyDepartment, serviceMessagesPath+"&origin=Receptive")
	if rec.Code != http.StatusBadRequest || errorCode(t, rec) != CodeOriginInvalid || messages.calls != 0 {
		t.Fatalf("status = %d calls = %d", rec.Code, messages.calls)
	}
}

func TestServiceMessagesTellTheClientWhenToRetryABusyGate(t *testing.T) {
	rec := serveServiceMessages(t, &stubSections{}, &stubServiceMessages{err: cache.ErrGateBusy}, everyDepartment, serviceMessagesPath)
	if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("status = %d retry-after = %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	if errorCode(t, rec) != "" {
		t.Fatalf("a busy gate carries a code, so the client would not retry it")
	}
}

func TestServiceMessagesReportOtherFailures(t *testing.T) {
	rec := serveServiceMessages(t, &stubSections{}, &stubServiceMessages{err: errors.New("boom")}, everyDepartment, serviceMessagesPath)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestTheSectionListNamesWhatTheWorkspaceMayRead(t *testing.T) {
	messages := &stubServiceMessages{available: attendancedomain.AvailableSections(true)}
	rec := serveServiceMessages(t, &stubSections{}, messages, everyDepartment, "/attendance/sections")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var body attendancedomain.AvailableSectionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || !slices.Contains(body.Sections, attendancedomain.SectionServiceMessages) {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if messages.calls != 0 {
		t.Fatalf("listing the sections read service messages")
	}
}
