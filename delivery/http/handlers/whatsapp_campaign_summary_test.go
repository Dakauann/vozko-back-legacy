package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"vozko/delivery/http/response"
	"vozko/domain/campaign"
	wce "vozko/domain/whatsapp_campaign_entry"
)

type failingSummary struct{ err error }

func (f failingSummary) Execute(wce.WorkspaceSummaryFilter) (*campaign.SendsSummary, error) {
	return nil, f.err
}

func TestSummaryAnswersEveryUseCaseFailureWithTheSameServerError(t *testing.T) {
	for _, cause := range []error{errors.New("entries down"), errors.New("ledger down")} {
		h := &WhatsAppCampaignHandler{summaryUseCase: failingSummary{err: cause}}
		rec := httptest.NewRecorder()
		h.Summary(rec, requestWithClaims(http.MethodGet, "/whatsapp/campaigns/summary", nil, userClaims("user-1"), "ws-1"))

		var body response.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("body %q: %v", rec.Body.String(), err)
		}
		if rec.Code != http.StatusInternalServerError || !body.Error || body.Message != "Failed to load whatsapp campaigns summary" {
			t.Fatalf("%v: status %d, body %+v", cause, rec.Code, body)
		}
	}
}

type capturingSummary struct{ got wce.WorkspaceSummaryFilter }

func (c *capturingSummary) Execute(f wce.WorkspaceSummaryFilter) (*campaign.SendsSummary, error) {
	c.got = f
	return campaign.NewSendsSummary(campaign.SendsTally{}), nil
}

func TestSummaryFiltersBySendDaysAndNeverByCampaignType(t *testing.T) {
	uc := &capturingSummary{}
	h := &WhatsAppCampaignHandler{summaryUseCase: uc}
	rec := httptest.NewRecorder()
	h.Summary(rec, requestWithClaims(http.MethodGet, "/whatsapp/campaigns/summary?from=2026-10-01&to=2026-10-07&type=standard", nil, userClaims("user-1"), "ws-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if uc.got.WorkspaceID != "ws-1" || uc.got.Window.From == nil || uc.got.Window.To == nil {
		t.Fatalf("filter = %+v", uc.got)
	}
	if uc.got.Window.From.Day() != 1 || uc.got.Window.To.Day() != 7 || uc.got.Window.To.Hour() != 23 {
		t.Fatalf("window = %v to %v, want the whole first to seventh day", uc.got.Window.From, uc.got.Window.To)
	}
}
