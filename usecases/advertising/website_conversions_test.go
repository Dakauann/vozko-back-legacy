package advertising

import (
	"context"
	"errors"
	"testing"

	ads "vozko/domain/advertising"
)

func websiteLead() ads.WebsiteLead {
	return ads.WebsiteLead{SubmissionID: "sub-1", At: testNow, Email: "ana@example.com", Fbp: "fb.1.1.2", UserAgent: "Mozilla/5.0", SourceURL: "https://loja.vozko.site/"}
}

func TestAWebsiteLeadIsSentToTheConnectedDataset(t *testing.T) {
	w, uc, _ := conversionsWorld(nil)
	if err := uc.SendWebsiteLead(context.Background(), "ws-1", websiteLead()); err != nil {
		t.Fatal(err)
	}
	if len(w.gateway.sent) != 1 || w.gateway.sent[0].EventID != "sub-1" || w.gateway.sent[0].ActionSource != ads.ActionSourceWebsite || w.gateway.sent[0].TargetID != "ds-1" {
		t.Fatalf("sent %+v", w.gateway.sent)
	}
}

func TestAWebsiteLeadIsNotSentWhenNothingIsConnected(t *testing.T) {
	w, uc, _ := conversionsWorld(nil)
	uc.settings.(*fakeSettings).s = nil
	if err := uc.SendWebsiteLead(context.Background(), "ws-1", websiteLead()); err != nil {
		t.Fatal(err)
	}
	_, off, _ := conversionsWorld(nil)
	off.settings.(*fakeSettings).s.DatasetID = ""
	if err := off.SendWebsiteLead(context.Background(), "ws-1", websiteLead()); err != nil {
		t.Fatal(err)
	}
	if len(w.gateway.sent) != 0 {
		t.Fatalf("sent %+v", w.gateway.sent)
	}
}

func TestAWebsiteLeadSendFailureIsReported(t *testing.T) {
	w, uc, _ := conversionsWorld(nil)
	w.gateway.failOn, w.gateway.failWith = "send_events", errors.New("meta down")
	if err := uc.SendWebsiteLead(context.Background(), "ws-1", websiteLead()); err == nil {
		t.Fatal("a failed send must be reported")
	}
}
