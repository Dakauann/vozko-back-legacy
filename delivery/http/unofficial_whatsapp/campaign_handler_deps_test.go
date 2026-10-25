package unofficial_whatsapp

import (
	"errors"
	"testing"

	"vozko/domain/campaign"
	"vozko/domain/shared/deps"
	uwc "vozko/domain/unofficial_whatsapp_campaign"
	workspace_domain "vozko/domain/workspace"
)

type unusedCreate struct{ uwc.CreateCampaignUseCase }
type unusedUpdate struct{ uwc.UpdateCampaignUseCase }
type unusedGet struct{ uwc.GetCampaignUseCase }
type unusedAccess struct{ uwc.CampaignAccessUseCase }
type unusedActions struct{ uwc.CampaignActionUseCase }
type unusedList struct{ uwc.ListCampaignsUseCase }
type unusedDelete struct{ uwc.DeleteCampaignUseCase }
type unusedAssignDep struct{ uwc.AssignDepartmentUseCase }
type unusedSummary struct{ uwc.GetSummaryUseCase }
type unusedEntries struct{ uwc.ListEntriesUseCase }
type unusedReset struct{ uwc.ResetCampaignUseCase }
type unusedClear struct{ uwc.ClearHistoryUseCase }
type unusedAddEntry struct{ uwc.AddEntriesUseCase }
type unusedUpdateEntry struct{ uwc.UpdateEntryUseCase }
type unusedDeleteEntry struct{ uwc.DeleteEntryUseCase }
type unusedQuickSend struct{ uwc.QuickSendUseCase }
type unusedValidate struct{ uwc.ValidateTargetsUseCase }
type unusedEstimate struct{ uwc.EstimateCampaignUseCase }
type unusedDepartments struct{ DepartmentScopeResolver }
type unusedPermissions struct{ workspace_domain.CheckAccessUseCase }
type unusedProgress struct{ campaign.ProgressUseCase }
type unusedTimeline struct{ campaign.TimelineUseCase }
type unusedScheduled struct{ ScheduledLauncher }

func campaignHandler(d CampaignHandlerDeps) *CampaignHandler {
	fill(&d)
	h, err := NewCampaignHandler(d)
	if err != nil {
		panic(err)
	}
	return h
}

func fill(d *CampaignHandlerDeps) {
	if d.Create == nil {
		d.Create = unusedCreate{}
	}
	if d.Update == nil {
		d.Update = unusedUpdate{}
	}
	if d.Get == nil {
		d.Get = unusedGet{}
	}
	if d.Access == nil {
		d.Access = unusedAccess{}
	}
	if d.Actions == nil {
		d.Actions = unusedActions{}
	}
	if d.List == nil {
		d.List = unusedList{}
	}
	if d.Delete == nil {
		d.Delete = unusedDelete{}
	}
	if d.AssignDep == nil {
		d.AssignDep = unusedAssignDep{}
	}
	if d.Summary == nil {
		d.Summary = unusedSummary{}
	}
	if d.Entries == nil {
		d.Entries = unusedEntries{}
	}
	if d.Reset == nil {
		d.Reset = unusedReset{}
	}
	if d.Clear == nil {
		d.Clear = unusedClear{}
	}
	if d.AddEntry == nil {
		d.AddEntry = unusedAddEntry{}
	}
	if d.UpdateEntry == nil {
		d.UpdateEntry = unusedUpdateEntry{}
	}
	if d.DeleteEntry == nil {
		d.DeleteEntry = unusedDeleteEntry{}
	}
	if d.QuickSend == nil {
		d.QuickSend = unusedQuickSend{}
	}
	if d.Validate == nil {
		d.Validate = unusedValidate{}
	}
	if d.Estimate == nil {
		d.Estimate = unusedEstimate{}
	}
	if d.Departments == nil {
		d.Departments = unusedDepartments{}
	}
	if d.Permissions == nil {
		d.Permissions = unusedPermissions{}
	}
	if d.Progress == nil {
		d.Progress = unusedProgress{}
	}
	if d.Timeline == nil {
		d.Timeline = unusedTimeline{}
	}
	if d.Scheduled == nil {
		d.Scheduled = unusedScheduled{}
	}
}

func TestTheUnofficialCampaignHandlerRefusesMissingDependencies(t *testing.T) {
	h, err := NewCampaignHandler(CampaignHandlerDeps{})
	var missing *deps.MissingError
	if h != nil || !errors.As(err, &missing) || len(missing.Missing) != 23 {
		t.Fatalf("h = %v err = %v, want every dependency refused", h, err)
	}
}
