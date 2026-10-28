package agenttool

import (
	"context"
	"errors"
	"fmt"

	"vozko/domain/agent"
	"vozko/domain/lint"
)

const (
	CodeNameInvalid               = "tool_name_invalid"
	CodeNameUnreadable            = "tool_name_unreadable"
	CodeNameTaken                 = "tool_name_taken"
	CodeDescriptionMissing        = "tool_description_missing"
	CodeDescriptionThin           = "tool_description_thin"
	CodeDescriptionsOverlap       = "tool_descriptions_overlap"
	CodeConfigInvalid             = "tool_config_invalid"
	CodeTypeUnknown               = "tool_type_unknown"
	CodeSecretInPlainText         = "tool_secret_in_plain_text"
	CodeSecretMissing             = "tool_secret_missing"
	CodeHTTPURLInvalid            = "http_url_invalid"
	CodeHTTPURLNotHTTPS           = "http_url_not_https"
	CodeHTTPFieldUnbound          = "http_field_unbound"
	CodeHTTPFieldUnused           = "http_field_unused"
	CodeHTTPFieldDuplicate        = "http_field_duplicate"
	CodeHTTPMethodBody            = "http_method_body"
	CodeHTTPTimeoutRange          = "http_timeout_out_of_range"
	CodeHTTPBodyNotJSON           = "http_body_not_json"
	CodeHTTPFieldUndescribed      = "http_field_undescribed"
	CodeHTTPOutputPathInvalid     = "http_output_path_invalid"
	CodeHTTPHostBlocked           = "http_host_blocked"
	CodeHTTPHostUnresolved        = "http_host_unresolved"
	CodeToolsMany                 = "agent_tools_many"
	CodeToolsTokenBudget          = "agent_tools_token_budget"
	CodeToolsExceedContext        = "agent_tools_exceed_context"
	CodeModelWithoutTools         = "agent_model_without_tools"
	CodePromptUnknownTool         = "prompt_mentions_unknown_tool"
	CodeValidationOffline         = "agent_tool_validation_unavailable"
	CodeWorkflowNotCallable       = "tool_workflow_not_callable"
	CodeSheetInvalid              = "tool_sheet_invalid"
	CodeSheetNoInputColumns       = "tool_sheet_no_input_columns"
	CodeDepartmentInvalid         = "tool_department_invalid"
	CodeKnowledgeBaseInvalid      = "tool_knowledge_base_invalid"
	CodeMCPSourceInvalid          = "tool_mcp_source_invalid"
	CodeBindingInvalid            = "tool_binding_invalid"
	CodeTransferNoDestinations    = "tool_transfer_no_destinations"
	CodeTransferSelf              = "tool_transfer_self"
	CodeTransferDuplicate         = "tool_transfer_duplicate"
	CodeTransferTargetUnusable    = "tool_transfer_target_unusable"
	CodeTransferWorkflowDraft     = "tool_transfer_workflow_not_published"
	CodeTransferWorkflowPaused    = "tool_transfer_workflow_paused"
	CodeTransferStageInvalid      = "tool_transfer_stage_invalid"
	CodeTransferVariableUnmapped  = "tool_transfer_variable_unmapped"
	CodeTransferVariableNotNeeded = "tool_transfer_variable_not_needed"
	CodeTransferVariableSource    = "tool_transfer_variable_source_invalid"
	NotPublishableCode            = "agent_not_publishable"
	NotActivatableCode            = "agent_not_activatable"
	manyToolsThreshold            = 20
	tokenBudgetAdvisory           = 10000
	charsPerToken                 = 4
	thinDescriptionSentences      = 3
	thinDescriptionChars          = 160
	overlapThreshold              = 0.8
	overlapMinWords               = 4
)

type Issue struct {
	Code     string        `json:"code"`
	Severity lint.Severity `json:"severity"`
	ToolID   string        `json:"toolId,omitempty"`
	Field    string        `json:"field,omitempty"`
	Message  string        `json:"message"`
	Hint     string        `json:"hint,omitempty"`
}

type Report struct {
	Issues []Issue `json:"issues"`
}

func (r *Report) add(issue Issue) {
	r.Issues = append(r.Issues, issue)
}

func (r Report) Blocking() []Issue {
	var out []Issue
	for _, i := range r.Issues {
		if i.Severity == lint.SeverityBlocking {
			out = append(out, i)
		}
	}
	return out
}

func (r Report) IsGreen() bool {
	return len(r.Blocking()) == 0
}

func (r Report) Merge(other Report) Report {
	r.Issues = append(append([]Issue(nil), r.Issues...), other.Issues...)
	return r
}

func (r Report) ForTool(toolID string) []Issue {
	var out []Issue
	for _, i := range r.Issues {
		if i.ToolID == toolID {
			out = append(out, i)
		}
	}
	return out
}

func Unavailable(reason string) Report {
	return Report{Issues: []Issue{{
		Code:     CodeValidationOffline,
		Severity: lint.SeverityBlocking,
		Message:  "Não foi possível conferir as ferramentas agora: " + reason,
		Hint:     "Tente de novo em instantes. Enquanto a conferência não roda, o agente não é publicado.",
	}}}
}

func EstimatedTokens(chars int) int {
	return (chars + charsPerToken - 1) / charsPerToken
}

var (
	ErrNotActivatable = errors.New("agenttool: the agent has blocking tool issues")
	ErrNotPublishable = errors.New("agenttool: the draft has blocking issues")
)

type BlockedError struct {
	Kind   error
	Report Report
}

func (e *BlockedError) Error() string {
	return fmt.Sprintf("%v (%d blocking)", e.Kind, len(e.Report.Blocking()))
}

func (e *BlockedError) Is(target error) bool { return target == e.Kind }

func BlockedBy(kind error, report Report) error {
	if report.IsGreen() {
		return nil
	}
	return &BlockedError{Kind: kind, Report: report}
}

type HostVerdict int

const (
	HostOK HostVerdict = iota
	HostBlocked
	HostNotFound
)

type ToolsValidation interface {
	Validate(ctx context.Context, agentID string, draft []agent.ToolBinding) (Report, error)
}
