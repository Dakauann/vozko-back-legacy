package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"vozko/delivery/http/response"
	"vozko/domain/auth"
	"vozko/domain/campaign"
	"vozko/domain/campaign/launchcheck"
	businessphone "vozko/domain/whatsapp/business_phone"
	wc "vozko/domain/whatsapp_campaign"
	wd "vozko/domain/workspace/workspace_department"
	"vozko/infra/http/middleware"
	whatsapp_campaign_usecase "vozko/usecases/whatsapp_campaign"
)

func scheduleCreateHandler(phone businessphone.Status) (*WhatsAppCampaignHandler, *draftCreate) {
	create := &draftCreate{}
	check := whatsapp_campaign_usecase.NewLaunchCheck(whatsapp_campaign_usecase.LaunchCheckDeps{
		Templates: draftTemplates{}, Quality: draftQuality{}, Phones: draftPhones{status: phone}, TemplateGrants: draftGrants{},
		Exits: draftExits{}, Costs: draftCosts{}, Balances: draftBalances{}, Caps: draftCaps{}, Now: time.Now,
	})
	launch := whatsapp_campaign_usecase.NewDraftLaunch(whatsapp_campaign_usecase.DraftLaunchDeps{Create: create, Start: &draftStart{}, Check: check, Audience: draftCount{}, Rates: draftRate{}})
	h := &WhatsAppCampaignHandler{createUseCase: create}
	h.scheduled = launch
	h.SetStartGuard((&startGate{allowed: true}).guard)
	return h, create
}

func campaignCreateRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/whatsapp/campaigns", strings.NewReader(body))
	ctx := context.WithValue(req.Context(), middleware.ClaimsContextKey, &auth.Claims{UserID: "u1", Role: "user"})
	ctx = context.WithValue(ctx, middleware.WorkspaceIDContextKey, "ws")
	return req.WithContext(ctx)
}

func scheduledBody(at time.Time, fingerprint string) string {
	return fmt.Sprintf(`{"name":"Natal","templateId":"tpl-1","businessPhoneId":"phone-1","phoneNumbers":[{"number":"5511999990000"}],"scheduledStart":%q,"confirmedFingerprint":%q}`, at.Format(time.RFC3339), fingerprint)
}

func TestCreatingAScheduledCampaignRunsTheLaunchCheck(t *testing.T) {
	h, create := scheduleCreateHandler(businessphone.StatusConnected)
	at := time.Now().Add(3 * time.Hour).UTC().Truncate(time.Second)
	rec := httptest.NewRecorder()

	h.Create(rec, campaignCreateRequest(scheduledBody(at, "")))

	if rec.Code != http.StatusCreated || len(create.got) != 1 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if got := create.got[0]; !got.ScheduledStart.Equal(at) || got.ScheduledBy != "u1" {
		t.Fatalf("created %+v", got)
	}
	var body wc.Campaign
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.ID != "camp-new" || body.ScheduledBy != "u1" {
		t.Fatalf("%s: %v", rec.Body.String(), err)
	}
}

func TestCreatingAScheduledBlockedCampaignIsRefusedWithTheIssues(t *testing.T) {
	h, create := scheduleCreateHandler(businessphone.StatusRestricted)
	rec := httptest.NewRecorder()

	h.Create(rec, campaignCreateRequest(scheduledBody(time.Now().Add(3*time.Hour), "")))

	if rec.Code != http.StatusConflict || !bytes.Contains(rec.Body.Bytes(), []byte("campaign_launch_blocked")) || !bytes.Contains(rec.Body.Bytes(), []byte("phone_unavailable")) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if len(create.got) != 0 {
		t.Fatal("a blocked campaign was scheduled")
	}
}

func TestCreatingAnUnscheduledCampaignSkipsTheScheduleCheck(t *testing.T) {
	create := &draftCreate{}
	h := &WhatsAppCampaignHandler{createUseCase: create}
	rec := httptest.NewRecorder()

	h.Create(rec, campaignCreateRequest(`{"name":"Natal","templateId":"tpl-1","businessPhoneId":"phone-1","phoneNumbers":[{"number":"5511999990000"}]}`))

	if rec.Code != http.StatusCreated || len(create.got) != 1 || create.got[0].ScheduledBy != "" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestScheduleRefusalsAreCoded(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{&launchcheck.RefusedError{Report: launchcheck.Blocked(launchcheck.CodePhoneUnavailable, 0)}, http.StatusConflict, "campaign_launch_blocked"},
		{&launchcheck.RefusedError{Report: launchcheck.Report{Findings: []launchcheck.Finding{{Code: launchcheck.CodeNoExit, Level: launchcheck.Confirm}}}}, http.StatusConflict, "campaign_launch_unconfirmed"},
		{campaign.ErrSelectionScheduleRefused, http.StatusConflict, "send_schedule_from_leads"},
		{wc.ErrLaunchCheckUnavailable, http.StatusServiceUnavailable, "campaign_launch_check_unavailable"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		(&WhatsAppCampaignHandler{}).handleDomainError(rec, tc.err)
		var body struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != tc.status || body.Code != tc.code {
			t.Fatalf("%v wrote %d %s, want %d %q", tc.err, rec.Code, rec.Body.String(), tc.status, tc.code)
		}
	}
}

type startGate struct {
	allowed bool
	asked   int
}

func (g *startGate) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		g.asked++
		if !g.allowed {
			response.WriteErrorWithCode(w, http.StatusForbidden, "start_permission_required", "denied", nil)
			return
		}
		next(w, r)
	}
}

func TestSchedulingACampaignAtCreationNeedsTheStartPermission(t *testing.T) {
	h, create := scheduleCreateHandler(businessphone.StatusConnected)
	gate := &startGate{}
	h.SetStartGuard(gate.guard)
	rec := httptest.NewRecorder()

	h.Create(rec, campaignCreateRequest(scheduledBody(time.Now().Add(3*time.Hour), "")))

	if rec.Code != http.StatusForbidden || !bytes.Contains(rec.Body.Bytes(), []byte("start_permission_required")) || len(create.got) != 0 {
		t.Fatalf("%d %s created %d", rec.Code, rec.Body.String(), len(create.got))
	}

	gate.allowed = true
	rec = httptest.NewRecorder()
	h.Create(rec, campaignCreateRequest(scheduledBody(time.Now().Add(3*time.Hour), "")))
	if rec.Code != http.StatusCreated || len(create.got) != 1 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestAScheduleWithoutTheStartGuardIsRefused(t *testing.T) {
	h, create := scheduleCreateHandler(businessphone.StatusConnected)
	h.SetStartGuard(nil)
	rec := httptest.NewRecorder()

	h.Create(rec, campaignCreateRequest(scheduledBody(time.Now().Add(3*time.Hour), "")))

	if rec.Code != http.StatusForbidden || !bytes.Contains(rec.Body.Bytes(), []byte("start_permission_required")) || len(create.got) != 0 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestCreatingWithoutAScheduleNeedsOnlyCreate(t *testing.T) {
	create := &draftCreate{}
	gate := &startGate{}
	h := &WhatsAppCampaignHandler{createUseCase: create}
	h.SetStartGuard(gate.guard)
	rec := httptest.NewRecorder()

	h.Create(rec, campaignCreateRequest(`{"name":"Natal","templateId":"tpl-1","businessPhoneId":"phone-1","phoneNumbers":[{"number":"5511999990000"}]}`))

	if rec.Code != http.StatusCreated || gate.asked != 0 {
		t.Fatalf("%d asked %d", rec.Code, gate.asked)
	}
}

type ownedOne struct{}

func (ownedOne) Owned(workspaceID string, _ *wd.DepartmentFilter, id string) (*wc.Campaign, error) {
	return &wc.Campaign{ID: id, WorkspaceID: workspaceID}, nil
}

type updateRecorder struct{ got []*wc.Campaign }

func (u *updateRecorder) Execute(_ context.Context, id string, c *wc.Campaign) (*wc.Campaign, error) {
	u.got = append(u.got, c)
	return &wc.Campaign{ID: id}, nil
}

func (u *updateRecorder) SetArchived(context.Context, string, bool) error { return nil }

func campaignUpdateRequest(body string) *http.Request {
	req := campaignCreateRequest(body)
	req.Method = http.MethodPut
	return mux.SetURLVars(req, map[string]string{"id": "camp-1"})
}

func TestSettingOrKeepingAScheduleOnEditNeedsTheStartPermission(t *testing.T) {
	updates := &updateRecorder{}
	gate := &startGate{}
	h := &WhatsAppCampaignHandler{updateUseCase: updates, access: ownedOne{}}
	h.SetStartGuard(gate.guard)
	at := time.Now().Add(3 * time.Hour).UTC().Format(time.RFC3339)
	rec := httptest.NewRecorder()

	h.Update(rec, campaignUpdateRequest(`{"name":"Natal","scheduledStart":"`+at+`"}`))

	if rec.Code != http.StatusForbidden || !bytes.Contains(rec.Body.Bytes(), []byte("start_permission_required")) || len(updates.got) != 0 {
		t.Fatalf("%d %s updates %d", rec.Code, rec.Body.String(), len(updates.got))
	}

	gate.allowed = true
	rec = httptest.NewRecorder()
	h.Update(rec, campaignUpdateRequest(`{"name":"Natal","scheduledStart":"`+at+`"}`))
	if rec.Code != http.StatusOK || len(updates.got) != 1 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestAnEditWithoutAScheduleNeedsNoStartPermission(t *testing.T) {
	updates := &updateRecorder{}
	gate := &startGate{}
	h := &WhatsAppCampaignHandler{updateUseCase: updates, access: ownedOne{}}
	h.SetStartGuard(gate.guard)
	rec := httptest.NewRecorder()

	h.Update(rec, campaignUpdateRequest(`{"name":"Natal"}`))

	if rec.Code != http.StatusOK || gate.asked != 0 || len(updates.got) != 1 {
		t.Fatalf("%d asked %d updates %d", rec.Code, gate.asked, len(updates.got))
	}
}
