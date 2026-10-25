package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"vozko/delivery/http/response"
	wc "vozko/domain/whatsapp_campaign"
	wce "vozko/domain/whatsapp_campaign_entry"
)

type failingSummary struct{ err error }

func (f failingSummary) Execute(wce.WorkspaceSummaryFilter) (*wc.CampaignMetrics, error) {
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
