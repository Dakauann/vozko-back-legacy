package whatsapp_campaign_usecase

import (
	"context"
	"strings"

	"vozko/domain/campaign"
	"vozko/domain/campaign/launchcheck"
	"vozko/domain/conversation"
	wc "vozko/domain/whatsapp_campaign"
	automationneeds_usecase "vozko/usecases/automationneeds"
)

const unreadSheets = "sheets"

type CampaignSheets interface {
	CampaignSheets(ctx context.Context, workspaceID string, targets []conversation.Automation, values map[string]string) (automationneeds_usecase.SheetLaunch, error)
}

func (c *LaunchCheck) readSheets(ctx context.Context, camp *wc.Campaign, facts *launchFacts) {
	targets := automationTargets(camp.Automation())
	if len(targets) == 0 || c.d.Sheets == nil {
		return
	}
	verdict, err := c.d.Sheets.CampaignSheets(ctx, camp.WorkspaceID, targets, fixedValues(camp))
	if err != nil {
		facts.unread(unreadSheets, err)
		return
	}
	if verdict.Checked {
		facts.snapshot.Sheets = &launchcheck.SheetFacts{Unfit: verdict.Unfit, Attention: verdict.Attention}
	}
}

func automationTargets(a campaign.Automation) []conversation.Automation {
	var out []conversation.Automation
	if id := strings.TrimSpace(a.WorkflowID); a.EnableWorkflow && id != "" {
		out = append(out, conversation.Automation{Kind: conversation.AutomationWorkflow, ID: id})
	}
	if id := strings.TrimSpace(a.AgentID); a.EnableAgentResponses && id != "" {
		out = append(out, conversation.Automation{Kind: conversation.AutomationAgent, ID: id})
	}
	return out
}

func fixedValues(camp *wc.Campaign) map[string]string {
	out := map[string]string{}
	if camp.Recipients == nil {
		return out
	}
	for _, v := range camp.Recipients.Spec.Variables {
		if v.Source == campaign.BindLiteral && strings.TrimSpace(v.Value) != "" {
			out[strings.TrimSpace(v.Key)] = strings.TrimSpace(v.Value)
		}
	}
	return out
}
