package unofficial_whatsapp

import (
	"context"
	"errors"
	"testing"
	"time"

	"vozko/domain/leadexit"
	"vozko/domain/shared/deps"
	uw "vozko/domain/unofficial_whatsapp"
)

type openLinkGate struct{}

func (openLinkGate) Admit(context.Context, *uw.Instance, string) error { return nil }

type quietConnections struct{}

func (quietConnections) InstanceConnected(context.Context, *uw.Instance) {}

type noLineSiblings struct{ uw.LineRepository }

func (noLineSiblings) ListSameNumber(context.Context, *uw.Instance) ([]*uw.Instance, error) {
	return nil, nil
}

type roomyAllowance struct{}

func (roomyAllowance) AllowanceFor(context.Context, string) (uw.InstanceAllowance, error) {
	return uw.InstanceAllowance{Limit: 1000}, nil
}

type quietVoice struct{}

func (quietVoice) ToVoiceNote(context.Context, string) ([]byte, error) { return nil, nil }

type quietGroupWatcher struct{}

func (quietGroupWatcher) GroupMessage(context.Context, *uw.Instance, string, bool) {}
func (quietGroupWatcher) GroupChanged(context.Context, *uw.Instance, string, uw.GroupParticipantChange) {
}
func (quietGroupWatcher) SeatSynced(context.Context, *uw.Instance, *uw.Group) {}

type quietCampaignSink struct{}

func (quietCampaignSink) AdvanceFromDelivery(string, uw.DeliveryStatus) {}

type noCampaignAutomation struct{}

func (noCampaignAutomation) AutomationForConversation(string) (*CampaignAutomation, bool) {
	return nil, false
}

type noLeadExit struct{}

func (noLeadExit) Run(context.Context, leadexit.Inbound, leadexit.Replier) (leadexit.Result, error) {
	return leadexit.Result{}, nil
}

func connectFor(instances uw.InstanceRepository, servers uw.ServerRepository, provider uw.ProviderAPI) *ConnectInstanceUseCase {
	return NewConnectInstanceUseCase(instances, servers, provider, openLinkGate{}, quietConnections{})
}

func healthFor(instances uw.InstanceRepository, servers uw.ServerRepository, provider uw.ProviderAPI, baseURL string) *CheckInstanceHealthUseCase {
	return NewCheckInstanceHealthUseCase(instances, servers, provider, baseURL, openLinkGate{}, quietConnections{}, NewLineHandover(noLineSiblings{}))
}

func provisionFor(servers uw.ServerRepository, instances uw.InstanceRepository, provider uw.ProviderAPI, baseURL string) *ProvisionInstanceUseCase {
	return NewProvisionInstanceUseCase(servers, instances, provider, baseURL, roomyAllowance{})
}

func adapterFor(instances uw.InstanceRepository, servers uw.ServerRepository, contacts uw.ContactRepository, conversations uw.ConversationRepository, messaging uw.MessagingAPI) *channelAdapter {
	adapter := newTextAdapter(instances, servers, contacts, conversations, messaging)
	adapter.voice = quietVoice{}
	return adapter
}

func webhookFor(d HandleWebhookDeps) *HandleWebhookUseCase {
	if d.LinkGate == nil {
		d.LinkGate = openLinkGate{}
	}
	if d.Listener == nil {
		d.Listener = quietConnections{}
	}
	if d.Handover == nil {
		d.Handover = NewLineHandover(noLineSiblings{})
	}
	if d.HistoryWindow == 0 {
		d.HistoryWindow = 30 * 24 * time.Hour
	}
	if d.GroupWatcher == nil {
		d.GroupWatcher = quietGroupWatcher{}
	}
	if d.CampaignSink == nil {
		d.CampaignSink = quietCampaignSink{}
	}
	if d.CampaignAutomation == nil {
		d.CampaignAutomation = noCampaignAutomation{}
	}
	if d.ExitStep == nil {
		d.ExitStep = noLeadExit{}
	}
	if d.AutomationGate == nil {
		d.AutomationGate = openAutomationGate()
	}
	uc, err := NewHandleWebhookUseCase(d)
	if err != nil {
		panic(err)
	}
	return uc
}

func webhookDefaults(uc *HandleWebhookUseCase) *HandleWebhookUseCase {
	if uc.campaignStatus == nil {
		uc.campaignStatus = quietCampaignSink{}
	}
	if uc.campaignAutomation == nil {
		uc.campaignAutomation = noCampaignAutomation{}
	}
	if uc.groupWatcher == nil {
		uc.groupWatcher = quietGroupWatcher{}
	}
	if uc.exitStep == nil {
		uc.exitStep = noLeadExit{}
	}
	if uc.handover == nil {
		uc.handover = NewLineHandover(noLineSiblings{})
	}
	if uc.historyWindow == 0 {
		uc.historyWindow = 30 * 24 * time.Hour
	}
	if uc.sync.gate == nil {
		uc.sync.gate = openLinkGate{}
	}
	if uc.sync.listener == nil {
		uc.sync.listener = quietConnections{}
	}
	if uc.gate == nil {
		uc.gate = openAutomationGate()
	}
	return uc
}

func TestTheUnofficialWebhookRefusesAMissingHistoryWindow(t *testing.T) {
	_, err := NewHandleWebhookUseCase(HandleWebhookDeps{
		LinkGate: openLinkGate{}, Listener: quietConnections{}, Handover: NewLineHandover(noLineSiblings{}),
		GroupWatcher: quietGroupWatcher{}, CampaignSink: quietCampaignSink{}, CampaignAutomation: noCampaignAutomation{}, ExitStep: noLeadExit{},
		AutomationGate: openAutomationGate(),
	})
	if !errors.Is(err, ErrHistoryWindowRequired) {
		t.Fatalf("err = %v, want ErrHistoryWindowRequired", err)
	}
	if _, err := NewHistorySyncUseCase(HistorySyncDeps{}); !errors.Is(err, ErrHistoryWindowRequired) {
		t.Fatalf("history sync err = %v, want ErrHistoryWindowRequired", err)
	}
}

func TestTheUnofficialWebhookRefusesMissingSessionPorts(t *testing.T) {
	_, err := NewHandleWebhookUseCase(HandleWebhookDeps{HistoryWindow: time.Hour})
	var missing *deps.MissingError
	if !errors.As(err, &missing) || len(missing.Missing) != 8 {
		t.Fatalf("err = %v, want every session port refused", err)
	}
}
