package database

import (
	"fmt"

	"gorm.io/gorm"

	"vozko/domain/workflow"
)

type dataRepair struct {
	name string
	run  func(*gorm.DB) error
}

func repairNames() []string {
	all := dataRepairs(nil)
	names := make([]string, 0, len(all))
	for _, r := range all {
		names = append(names, r.name)
	}
	return names
}

func dataRepairs(nodeOutputs workflow.DynamicHandleResolver) []dataRepair {
	return []dataRepair{
		{"wc_consume_used_schedules", consumeUsedSchedules},
		{"drop_retired_setting_columns", dropRetiredSettingColumns},
		{"lx_drop_workspace_exit_settings", dropWorkspaceExitSettings},
		{"em_drop_list_validation", dropEmailListValidation},
		{"em_drop_run_charge_columns", dropEmailRunChargeColumns},
		{"cm_backfill_awaiting_reply", backfillAwaitingReply},
		{"cm_mark_note_images", markNoteImages},
		{"cm_mark_email_bodies", markEmailBodies},
		{"wf_publish_running_workflows", publishRunningWorkflows},
		{"wf_canonical_structured_configs", canonicalizeWorkflowGraphs},
		{"wf_canonical_node_types", canonicalizeLegacyNodeTypes},
		{"wf_retire_tool_mode", retireToolModes(nodeOutputs)},
		{"inv_external_id_per_provider", repairInvoiceExternalIDs},
		{"em_recheck_unconfirmed_no_mail", recheckUnconfirmedNoMail},
		{"ag_tool_instances", copyAgentToolInstances},
		{"ag_drop_agent_bindings_index", dropAgentBindingsIndex},
		{"ag_canonical_http_configs", canonicalizeAgentHTTPConfigs},
		{"ag_mcp_collection_instances", copyAgentMCPInstances},
		{"bl_balance_read_model", backfillBalanceReadModel},
	}
}

func runDataRepairs(tx *gorm.DB, nodeOutputs workflow.DynamicHandleResolver) error {
	for _, r := range dataRepairs(nodeOutputs) {
		if err := r.run(tx); err != nil {
			return fmt.Errorf("data repair %s: %w", r.name, err)
		}
	}
	return nil
}

func dropWorkspaceExitSettings(tx *gorm.DB) error {
	return tx.Exec("DROP TABLE IF EXISTS workspace_exit_settings").Error
}
