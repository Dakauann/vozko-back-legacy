package advertising

import (
	"strings"
	"time"
)

const (
	EventNameWebsiteLead = "Lead"
	ActionSourceWebsite  = "website"
)

type WebsiteLead struct {
	SubmissionID string
	At           time.Time
	Phone        string
	Email        string
	Fbp          string
	Fbc          string
	UserAgent    string
	SourceURL    string
}

func WebsiteLeadFor(settings ConversionSettings, lead WebsiteLead, now time.Time) (*ConversionEvent, SkipReason) {
	if !settings.Enabled {
		return nil, SkipNotEnabled
	}
	if !settings.SendLeads {
		return nil, SkipEventOff
	}
	event := &ConversionEvent{EventID: strings.TrimSpace(lead.SubmissionID), Name: EventNameWebsiteLead, Time: lead.At, ActionSource: ActionSourceWebsite}
	switch {
	case strings.TrimSpace(settings.PixelID) != "":
		event.Target, event.TargetID = TargetPixel, strings.TrimSpace(settings.PixelID)
	case strings.TrimSpace(settings.DatasetID) != "":
		event.Target, event.TargetID = TargetDataset, strings.TrimSpace(settings.DatasetID)
	default:
		return nil, SkipNoDataset
	}
	if now.Sub(lead.At) > conversionMaxAge {
		return nil, SkipTooOld
	}
	event.PhoneHash = HashMatch(MatchPhone, lead.Phone, "")
	event.EmailHash = HashMatch(MatchEmail, lead.Email, "")
	event.Fbp, event.Fbc = strings.TrimSpace(lead.Fbp), strings.TrimSpace(lead.Fbc)
	event.ClientUserAgent, event.SourceURL = strings.TrimSpace(lead.UserAgent), strings.TrimSpace(lead.SourceURL)
	matchable := event.PhoneHash != "" || event.EmailHash != "" || event.Fbp != "" || event.Fbc != ""
	if event.EventID == "" || event.ClientUserAgent == "" || !matchable {
		return nil, SkipNoAdIdentity
	}
	return event, ""
}
