package automationneeds

import (
	"context"
	"strings"
	"time"

	"vozko/domain/conversation"
)

type SlotKind string

const (
	SlotWhatsAppNumber SlotKind = "whatsapp_number"
	SlotUnofficial     SlotKind = "unofficial_whatsapp"
	SlotInstagram      SlotKind = "instagram"
	SlotFacebook       SlotKind = "facebook"
	SlotTelegram       SlotKind = "telegram"
	SlotWebchat        SlotKind = "webchat"
	SlotTikTok         SlotKind = "tiktok"
	SlotMercadoLivre   SlotKind = "mercadolivre"
	SlotEmail          SlotKind = "email"
	SlotRCS            SlotKind = "rcs"
	SlotSMS            SlotKind = "sms"
)

var slotKinds = []SlotKind{SlotWhatsAppNumber, SlotUnofficial, SlotInstagram, SlotFacebook, SlotTelegram, SlotWebchat, SlotTikTok, SlotMercadoLivre, SlotEmail, SlotRCS, SlotSMS}

func SlotKinds() []SlotKind {
	return append([]SlotKind(nil), slotKinds...)
}

func (k SlotKind) Valid() bool {
	for _, known := range slotKinds {
		if known == k {
			return true
		}
	}
	return false
}

type Slot struct {
	Kind        SlotKind
	ID          string
	Name        string
	WorkspaceID string
	Running     []conversation.Automation
}

type SwitchOff struct {
	Slot       Slot
	Automation Link
	Needs      Needs
}

type SwitchOffRecord struct {
	ID            string
	WorkspaceID   string
	SlotKind      SlotKind
	SlotID        string
	SlotName      string
	Automation    Link
	Variables     []string
	SwitchedOffBy string
	SwitchedOffAt time.Time
	ResolvedAt    *time.Time
}

type Slots interface {
	InWorkspace(ctx context.Context, workspaceID string) ([]Slot, error)
}

type SwitchOffRecords interface {
	Open(ctx context.Context, workspaceID string, kind SlotKind, slotID string) (*SwitchOffRecord, error)
	Resolve(ctx context.Context, workspaceID string, kind SlotKind, slotID string, at time.Time) (int, error)
}

func RunningOf(c conversation.ChannelAutomation) []conversation.Automation {
	var out []conversation.Automation
	if c.EnableAgentResponses && c.HasAgent() {
		out = append(out, conversation.Automation{Kind: conversation.AutomationAgent, ID: strings.TrimSpace(*c.AgentID)})
	}
	if c.EnableWorkflow && c.WorkflowRef() != "" {
		out = append(out, conversation.Automation{Kind: conversation.AutomationWorkflow, ID: c.WorkflowRef()})
	}
	return out
}

func Impact(slots []Slot, needsOf func(conversation.Automation) (Needs, error)) ([]SwitchOff, error) {
	var out []SwitchOff
	for _, slot := range slots {
		for _, running := range slot.Running {
			needs, err := needsOf(running)
			if err != nil {
				return nil, err
			}
			if len(needs) == 0 {
				continue
			}
			out = append(out, SwitchOff{Slot: slot, Automation: rootLink(running, needs), Needs: needs})
		}
	}
	return out, nil
}

func rootLink(running conversation.Automation, needs Needs) Link {
	for _, need := range needs {
		if len(need.Via) > 0 && need.Via[0].Automation == running {
			return need.Via[0]
		}
	}
	return Link{Automation: running}
}

type ReceptiveGuard interface {
	Admit(workspaceID string, config conversation.ChannelAutomation) error
	Saved(ctx context.Context, workspaceID string, kind SlotKind, slotID string, config conversation.ChannelAutomation)
}

func (r SwitchOffRecord) SettledBy(config conversation.ChannelAutomation) bool {
	if len(RunningOf(config)) > 0 {
		return true
	}
	chosen := ""
	switch r.Automation.Kind {
	case conversation.AutomationAgent:
		if config.AgentID != nil {
			chosen = strings.TrimSpace(*config.AgentID)
		}
	case conversation.AutomationWorkflow:
		chosen = config.WorkflowRef()
	}
	return chosen != r.Automation.ID
}
