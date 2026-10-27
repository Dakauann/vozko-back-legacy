package whatsapp_campaign

import wce "vozko/domain/whatsapp_campaign_entry"

func (c *Campaign) RecordsOneOffSendOn(entry *wce.WhatsAppCampaignEntry) bool {
	return c != nil && entry != nil && c.IsOrganic() && entry.CampaignID == c.ID
}
