package emailcampaignhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"vozko/domain/auth"
	"vozko/domain/emailconsent"
	"vozko/infra/http/middleware"
	emailconsent_usecase "vozko/usecases/emailconsent"
)

type recordingConsent struct {
	fakeConsent
	recorded []emailconsent_usecase.SubscribeInput
	err      error
}

func (c *recordingConsent) Record(_ context.Context, _ string, in emailconsent_usecase.SubscribeInput) (emailconsent.Subscription, error) {
	c.recorded = append(c.recorded, in)
	if c.err != nil {
		return emailconsent.Subscription{}, c.err
	}
	return emailconsent.Subscription{Address: "ana@example.com", Source: in.Source, Status: emailconsent.StatusSubscribed}, nil
}

func serveSubscribe(consent Consent, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/email/subscriptions", strings.NewReader(body))
	ctx := context.WithValue(req.Context(), middleware.WorkspaceIDContextKey, "ws-1")
	ctx = context.WithValue(ctx, middleware.ClaimsContextKey, &auth.Claims{UserID: "user-1"})
	rec := httptest.NewRecorder()
	NewConsentHandler(consent).Subscribe(rec, req.WithContext(ctx))
	return rec
}

func TestTheOperatorConsentGoesThroughTheOperatorRule(t *testing.T) {
	consent := &recordingConsent{}
	rec := serveSubscribe(consent, `{"address":"ana@example.com","source":"operator"}`)
	if rec.Code != http.StatusOK || len(consent.recorded) != 1 || consent.recorded[0].Source != emailconsent.SourceOperator {
		t.Fatalf("status %d recorded %+v", rec.Code, consent.recorded)
	}
}

func TestTheOperatorConsentRefusesAPageFormSource(t *testing.T) {
	consent := &recordingConsent{err: emailconsent.ErrSourceReserved}
	rec := serveSubscribe(consent, `{"address":"ana@example.com","source":"site_form","ip":"Zm9yLXRlc3RzLW9ubHktYS1oYXNoZWQtaXAtdmFsdWU"}`)
	var body struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusBadRequest || body.Code != "email_source_reserved" {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}
