package whatsapp_campaign_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/campaign"
	"vozko/domain/campaign/launchcheck"
	"vozko/domain/campaignaudience"
	"vozko/domain/conversation"
	wc "vozko/domain/whatsapp_campaign"
	automationneeds_usecase "vozko/usecases/automationneeds"
)

type campaignSheets struct {
	verdict automationneeds_usecase.SheetLaunch
	err     error
	targets []conversation.Automation
	values  map[string]string
}

func (c *campaignSheets) CampaignSheets(_ context.Context, _ string, targets []conversation.Automation, values map[string]string) (automationneeds_usecase.SheetLaunch, error) {
	c.targets, c.values = targets, values
	return c.verdict, c.err
}

func franchiseCampaign() *wc.Campaign {
	return &wc.Campaign{
		ID: "camp-1", WorkspaceID: "ws-1", TemplateID: "tpl-1", BusinessPhoneID: "phone-1", TemplateCategory: "UTILITY",
		WorkflowID: "wf-1", EnableWorkflow: true,
		Recipients: &campaignaudience.Request{Spec: campaignaudience.Spec{Variables: []campaignaudience.Variable{
			{Key: "planilha", Source: campaign.BindLiteral, Value: "sp-centro"},
			{Key: "matricula", Column: "VAR3"},
		}}},
	}
}

func TestTheLaunchCheckBlocksASpreadsheetThatDoesNotFitOrNeedsAttention(t *testing.T) {
	sheets := &campaignSheets{verdict: automationneeds_usecase.SheetLaunch{Checked: true, Unfit: true, Attention: true}}
	deps := launchDeps()
	deps.Sheets = sheets
	report, _ := NewLaunchCheck(deps).Preview(context.Background(), franchiseCampaign(), 10)
	if !found(report, launchcheck.Block, launchcheck.CodeSheetVariableUnfit) || !found(report, launchcheck.Block, launchcheck.CodeSheetVariableAttention) {
		t.Fatalf("findings = %+v", report.Findings)
	}
	if len(sheets.targets) != 1 || sheets.targets[0].ID != "wf-1" || sheets.values["planilha"] != "sp-centro" || len(sheets.values) != 1 {
		t.Fatalf("asked targets %+v values %v", sheets.targets, sheets.values)
	}
}

func TestAnUnreadableSpreadsheetCheckBlocksTheLaunch(t *testing.T) {
	deps := launchDeps()
	deps.Sheets = &campaignSheets{err: errors.New("sheets down")}
	report, _ := NewLaunchCheck(deps).Preview(context.Background(), franchiseCampaign(), 10)
	if !found(report, launchcheck.Block, launchcheck.CodeHealthUnreadable) {
		t.Fatalf("findings = %+v", report.Findings)
	}
	calm := launchDeps()
	calm.Sheets = &campaignSheets{verdict: automationneeds_usecase.SheetLaunch{Checked: true}}
	if report, _ := NewLaunchCheck(calm).Preview(context.Background(), franchiseCampaign(), 10); found(report, launchcheck.Block, launchcheck.CodeSheetVariableUnfit) {
		t.Fatalf("a fitting spreadsheet = %+v", report.Findings)
	}
}
