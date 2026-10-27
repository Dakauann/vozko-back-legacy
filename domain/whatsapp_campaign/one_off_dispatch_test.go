package whatsapp_campaign

import (
	"testing"

	wce "vozko/domain/whatsapp_campaign_entry"
)

func TestOnlyTheNumbersReceptiveContainerRecordsAOneOffSendOnItsEntry(t *testing.T) {
	receptive := &Campaign{ID: "organic-1", Type: CampaignTypeOrganic}
	launch := &Campaign{ID: "launch-1", Type: CampaignTypeStandard}

	cases := []struct {
		name      string
		container *Campaign
		entry     *wce.WhatsAppCampaignEntry
		want      bool
	}{
		{"a conversation the receptive container holds keeps the one-off send", receptive, &wce.WhatsAppCampaignEntry{CampaignID: "organic-1"}, true},
		{"a campaign's entry only ever reflects what that campaign sent", launch, &wce.WhatsAppCampaignEntry{CampaignID: "launch-1"}, false},
		{"an entry of another campaign found by number is not the container's", receptive, &wce.WhatsAppCampaignEntry{CampaignID: "launch-1"}, false},
		{"no entry records nothing", receptive, nil, false},
		{"no container records nothing", nil, &wce.WhatsAppCampaignEntry{CampaignID: "organic-1"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.container.RecordsOneOffSendOn(tc.entry); got != tc.want {
				t.Fatalf("RecordsOneOffSendOn = %v, want %v", got, tc.want)
			}
		})
	}
}
