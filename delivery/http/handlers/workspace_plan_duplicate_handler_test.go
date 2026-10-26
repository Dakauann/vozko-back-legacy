package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gorilla/mux"

	"vozko/delivery/http/response"
	"vozko/delivery/http/response/responsetest"
	"vozko/domain/auth"
	"vozko/domain/shared/deps"
	"vozko/domain/user"
	workspace_plan "vozko/domain/workspace/workspace_plan"
	"vozko/infra/http/middleware"
	"vozko/pkg/codegolden"
)

const planErrorCodesAdvice = "update testdata/plan_error_codes.golden, add the code to vozko-front src/lib/workspace-plan/error-codes.ts and translate adminPlans.errors.<code> in the pt, en, es and de catalogs"

type fakePlanDuplicate struct {
	calls    []workspace_plan.DuplicatePlanInput
	replayed bool
	err      error
}

func (f *fakePlanDuplicate) Execute(input workspace_plan.DuplicatePlanInput) (*workspace_plan.DuplicatePlanResult, error) {
	f.calls = append(f.calls, input)
	if err := input.Validate(); err != nil {
		return nil, err
	}
	if f.err != nil {
		return nil, f.err
	}
	return &workspace_plan.DuplicatePlanResult{Plan: &workspace_plan.PlanDefinition{ID: "copy", Name: "Pro (cópia)"}, Replayed: f.replayed}, nil
}

type fakePlanDuplicatePreview struct {
	err error
}

func (f *fakePlanDuplicatePreview) Execute(sourcePlanID string) (*workspace_plan.PlanDuplicatePreview, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &workspace_plan.PlanDuplicatePreview{Name: "Pro (cópia)"}, nil
}

type planDuplicateRig struct {
	duplicate *fakePlanDuplicate
	preview   *fakePlanDuplicatePreview
	router    *mux.Router
}

func newPlanDuplicateRig(t *testing.T) *planDuplicateRig {
	t.Helper()
	rig := &planDuplicateRig{duplicate: &fakePlanDuplicate{}, preview: &fakePlanDuplicatePreview{}}
	handler, err := NewWorkspacePlanDuplicateHandler(WorkspacePlanDuplicateDeps{Duplicate: rig.duplicate, Preview: rig.preview})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	rig.router = mux.NewRouter()
	admin := rig.router.PathPrefix("").Subrouter()
	admin.Use(middleware.NewRolesMiddleware().RequireRole(user.RoleAdmin))
	RegisterPlanDuplicateAdminRoutes(admin, handler)
	return rig
}

func (rig *planDuplicateRig) send(method, key string, claims *auth.Claims) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/admin/plans/source/duplicate", nil)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	if claims != nil {
		req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsContextKey, claims))
	}
	rec := httptest.NewRecorder()
	rig.router.ServeHTTP(rec, req)
	return rec
}

var planAdmin = &auth.Claims{UserID: "admin-1", Role: string(user.RoleAdmin)}

func TestEveryPlanErrorWritesItsStableCode(t *testing.T) {
	responsetest.RequireEveryErrorWritesItsCode(t, PlanRefusals, writePlanError)
	responsetest.RequireStableCodes(t, PlanRequestRefusals)
}

func TestAnUnexpectedPlanErrorStillWritesACode(t *testing.T) {
	rec := httptest.NewRecorder()
	writePlanError(rec, errors.New("db down"))
	if rec.Code != http.StatusInternalServerError || responsetest.CodeOf(t, rec) != refusalPlanInternal.Code {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestPlanErrorCodesMatchTheGoldenSoTheFrontTranslatesThem(t *testing.T) {
	codes := response.RefusalCodes(response.ErrorRefusalsOf(PlanRefusals), PlanRequestRefusals)
	codegolden.Compare(t, filepath.Join("testdata", "plan_error_codes.golden"), codes, planErrorCodesAdvice)
}

func TestDuplicatePlanAnswersCreatedWithTheNewPlan(t *testing.T) {
	rig := newPlanDuplicateRig(t)
	rec := rig.send(http.MethodPost, "k1", planAdmin)

	var plan workspace_plan.PlanDefinition
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	if rec.Code != http.StatusCreated || plan.ID != "copy" {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	want := workspace_plan.DuplicatePlanInput{SourcePlanID: "source", RequestedBy: "admin-1", IdempotencyKey: "k1"}
	if len(rig.duplicate.calls) != 1 || rig.duplicate.calls[0] != want {
		t.Fatalf("use case got %+v, want %+v", rig.duplicate.calls, want)
	}
}

func TestDuplicatePlanRepeatedAnswersOKWithTheSamePlan(t *testing.T) {
	rig := newPlanDuplicateRig(t)
	rig.duplicate.replayed = true
	if rec := rig.send(http.MethodPost, "k1", planAdmin); rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestDuplicatePlanWithoutAKeyIsRefusedWithItsCode(t *testing.T) {
	rig := newPlanDuplicateRig(t)
	rec := rig.send(http.MethodPost, "", planAdmin)
	if rec.Code != http.StatusBadRequest || responsetest.CodeOf(t, rec) != "plan_duplicate_key_required" {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestDuplicatePlanErrorsAnswerWithTheirCodes(t *testing.T) {
	for err, code := range map[error]string{
		workspace_plan.ErrPlanNotFound:             "plan_not_found",
		workspace_plan.ErrDuplicateNameUnavailable: "plan_duplicate_name_unavailable",
		workspace_plan.ErrDuplicateKeyReused:       "plan_duplicate_key_reused",
	} {
		rig := newPlanDuplicateRig(t)
		rig.duplicate.err = err
		rec := rig.send(http.MethodPost, "k1", planAdmin)
		if got := responsetest.CodeOf(t, rec); got != code {
			t.Fatalf("%v answered %d %q, want %q", err, rec.Code, got, code)
		}
	}
}

func TestDuplicatePlanIsRefusedToAnyoneButAPlatformAdmin(t *testing.T) {
	for _, role := range []string{"user", "workspace_admin", ""} {
		rig := newPlanDuplicateRig(t)
		for _, method := range []string{http.MethodPost, http.MethodGet} {
			rec := rig.send(method, "k1", &auth.Claims{UserID: "u1", Role: role})
			if rec.Code != http.StatusForbidden || responsetest.CodeOf(t, rec) != middleware.RefusalRoleRequired.Code {
				t.Fatalf("role %q %s answered %d %s", role, method, rec.Code, rec.Body.String())
			}
		}
		if len(rig.duplicate.calls) != 0 {
			t.Fatalf("role %q reached the use case", role)
		}
	}
}

func TestDuplicatePlanPreviewNamesTheCopy(t *testing.T) {
	rig := newPlanDuplicateRig(t)
	rec := rig.send(http.MethodGet, "", planAdmin)
	var preview workspace_plan.PlanDuplicatePreview
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	if rec.Code != http.StatusOK || preview.Name != "Pro (cópia)" {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}

	rig.preview.err = workspace_plan.ErrPlanNotFound
	if rec := rig.send(http.MethodGet, "", planAdmin); rec.Code != http.StatusNotFound || responsetest.CodeOf(t, rec) != "plan_not_found" {
		t.Fatalf("missing source answered %d %s", rec.Code, rec.Body.String())
	}
}

func TestDuplicatePlanHandlerNeedsItsUseCases(t *testing.T) {
	if _, err := NewWorkspacePlanDuplicateHandler(WorkspacePlanDuplicateDeps{}); !errors.Is(err, deps.ErrMissing) {
		t.Fatalf("err = %v, want deps.ErrMissing", err)
	}
}
