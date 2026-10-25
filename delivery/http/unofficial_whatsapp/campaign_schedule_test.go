package unofficial_whatsapp

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
	"vozko/domain/campaign/scheduledstart"
	uw "vozko/domain/unofficial_whatsapp"
	uwc "vozko/domain/unofficial_whatsapp_campaign"
	workspace_domain "vozko/domain/workspace"
	"vozko/infra/http/middleware"
	"vozko/usecases/campaignlaunch"
	uwcuc "vozko/usecases/unofficial_whatsapp_campaign"
)

type scheduledLaunches struct {
	inputs []uwcuc.DraftLaunchInput
	err    error
}

func (s *scheduledLaunches) Launch(_ context.Context, in uwcuc.DraftLaunchInput) (campaignlaunch.Outcome[*uwc.Campaign], error) {
	s.inputs = append(s.inputs, in)
	if s.err != nil {
		return campaignlaunch.Outcome[*uwc.Campaign]{}, s.err
	}
	created := *in.Campaign
	created.ID = "camp-new"
	created.ScheduledStart = in.Launch.At
	return campaignlaunch.Outcome[*uwc.Campaign]{Campaign: &created}, nil
}

type plainCreate struct{ got []*uwc.Campaign }

func (p *plainCreate) Execute(_ context.Context, c *uwc.Campaign, _ uw.DepartmentScope) (*uwc.Campaign, error) {
	stored := *c
	stored.ID = "camp-plain"
	p.got = append(p.got, &stored)
	return &stored, nil
}

func unofficialCreateRequest(body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/unofficial-whatsapp/campaigns", strings.NewReader(body))
	ctx := context.WithValue(r.Context(), middleware.WorkspaceIDContextKey, "ws-1")
	ctx = context.WithValue(ctx, middleware.ClaimsContextKey, &auth.Claims{UserID: "u-1", Role: "member"})
	return r.WithContext(ctx)
}

func unofficialScheduledBody(at time.Time) string {
	return fmt.Sprintf(`{"name":"Natal","instanceId":"inst-1","message":{"kind":"text","text":"Oi /sair"},"targets":[{"number":"5511999990000"}],"scheduledStart":%q}`, at.Format(time.RFC3339))
}

func TestCreatingAScheduledUnofficialCampaignGoesThroughTheLaunchCheck(t *testing.T) {
	launches, create := &scheduledLaunches{}, &plainCreate{}
	h := campaignHandler(CampaignHandlerDeps{Create: create, Departments: openScope{}, Scheduled: launches})
	h.SetStartGuard(allowStart)
	at := time.Now().Add(3 * time.Hour).UTC().Truncate(time.Second)
	rec := httptest.NewRecorder()

	h.Create(rec, unofficialCreateRequest(unofficialScheduledBody(at)))

	if rec.Code != http.StatusCreated || len(launches.inputs) != 1 || len(create.got) != 0 {
		t.Fatalf("%d %s launches %d creates %d", rec.Code, rec.Body.String(), len(launches.inputs), len(create.got))
	}
	in := launches.inputs[0]
	if in.Launch.Mode != campaign.LaunchSchedule || !in.Launch.At.Equal(at) || !in.Campaign.ScheduledStart.IsZero() || in.Campaign.CreatedByID != "u-1" {
		t.Fatalf("launched %+v with %+v", in.Launch, in.Campaign)
	}
	var body campaignDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.ID != "camp-new" || body.ScheduledStart == nil {
		t.Fatalf("%s: %v", rec.Body.String(), err)
	}
}

func TestABlockedUnofficialScheduleAnswersTheIssues(t *testing.T) {
	launches := &scheduledLaunches{err: &launchcheck.RefusedError{Report: launchcheck.Blocked(launchcheck.CodeNumberUnusable, 0)}}
	h := campaignHandler(CampaignHandlerDeps{Create: &plainCreate{}, Departments: openScope{}, Scheduled: launches})
	h.SetStartGuard(allowStart)
	rec := httptest.NewRecorder()

	h.Create(rec, unofficialCreateRequest(unofficialScheduledBody(time.Now().Add(3*time.Hour))))

	if rec.Code != http.StatusConflict || !bytes.Contains(rec.Body.Bytes(), []byte("campaign_launch_blocked")) || !bytes.Contains(rec.Body.Bytes(), []byte("number_unusable")) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestAnUnscheduledUnofficialCampaignRecordsItsCreator(t *testing.T) {
	create := &plainCreate{}
	h := campaignHandler(CampaignHandlerDeps{Create: create, Departments: openScope{}})
	rec := httptest.NewRecorder()

	h.Create(rec, unofficialCreateRequest(`{"name":"Natal","instanceId":"inst-1","message":{"kind":"text","text":"Oi /sair"},"targets":[{"number":"5511999990000"}]}`))

	if rec.Code != http.StatusCreated || len(create.got) != 1 || create.got[0].CreatedByID != "u-1" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestTheFailedScheduleReasonIsShown(t *testing.T) {
	dto := campaignToDTO(&uwc.Campaign{ID: "c1", ScheduleFailure: scheduledstart.FailureBlocked})
	raw, _ := json.Marshal(dto)
	if !bytes.Contains(raw, []byte(`"scheduleFailure":"schedule_blocked"`)) {
		t.Fatalf("%s", raw)
	}
	clean, _ := json.Marshal(campaignToDTO(&uwc.Campaign{ID: "c2"}))
	if bytes.Contains(clean, []byte("scheduleFailure")) {
		t.Fatalf("%s", clean)
	}
}

func allowStart(next http.HandlerFunc) http.HandlerFunc { return next }

func denyStart(http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		response.WriteErrorWithCode(w, http.StatusForbidden, "start_permission_required", "denied", nil)
	}
}

func TestAnUnofficialScheduleNeedsTheStartPermission(t *testing.T) {
	launches := &scheduledLaunches{}
	h := campaignHandler(CampaignHandlerDeps{Create: &plainCreate{}, Departments: openScope{}, Scheduled: launches})
	h.SetStartGuard(denyStart)
	rec := httptest.NewRecorder()

	h.Create(rec, unofficialCreateRequest(unofficialScheduledBody(time.Now().Add(3*time.Hour))))

	if rec.Code != http.StatusForbidden || !bytes.Contains(rec.Body.Bytes(), []byte("start_permission_required")) || len(launches.inputs) != 0 {
		t.Fatalf("%d %s launches %d", rec.Code, rec.Body.String(), len(launches.inputs))
	}
}

func TestAnUnofficialScheduleWithoutAStartGuardIsRefused(t *testing.T) {
	launches := &scheduledLaunches{}
	h := campaignHandler(CampaignHandlerDeps{Create: &plainCreate{}, Departments: openScope{}, Scheduled: launches})
	rec := httptest.NewRecorder()

	h.Create(rec, unofficialCreateRequest(unofficialScheduledBody(time.Now().Add(3*time.Hour))))

	if rec.Code != http.StatusForbidden || !bytes.Contains(rec.Body.Bytes(), []byte("start_permission_required")) || len(launches.inputs) != 0 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestTheRoutesCheckTheStartPermissionOnlyForASchedule(t *testing.T) {
	launches, create := &scheduledLaunches{}, &plainCreate{}
	h := campaignHandler(CampaignHandlerDeps{Create: create, Departments: openScope{}, Scheduled: launches})
	var asked []string
	router := mux.NewRouter()
	RegisterCampaignRoutes(router, h, func(res workspace_domain.Resource, action workspace_domain.Action, next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			asked = append(asked, string(res)+":"+string(action))
			next(w, r)
		}
	})

	router.ServeHTTP(httptest.NewRecorder(), unofficialCreateRequest(unofficialScheduledBody(time.Now().Add(3*time.Hour))))
	if strings.Join(asked, ",") != "unofficial_whatsapp_campaigns:create,unofficial_whatsapp_campaigns:start" || len(launches.inputs) != 1 {
		t.Fatalf("asked %v launches %d", asked, len(launches.inputs))
	}

	asked = nil
	router.ServeHTTP(httptest.NewRecorder(), unofficialCreateRequest(`{"name":"Natal","instanceId":"inst-1","message":{"kind":"text","text":"Oi /sair"},"targets":[{"number":"5511999990000"}]}`))
	if strings.Join(asked, ",") != "unofficial_whatsapp_campaigns:create" || len(create.got) != 1 {
		t.Fatalf("asked %v creates %d", asked, len(create.got))
	}
}
