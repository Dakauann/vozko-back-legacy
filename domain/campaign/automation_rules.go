package campaign

import (
	"errors"
	"strconv"
	"strings"

	"vozko/domain/agent"
)

var (
	ErrWorkflowNotFound      = errors.New("campaign: the workflow does not exist")
	ErrWorkflowForbidden     = errors.New("campaign: the workflow belongs to another workspace")
	ErrWorkflowVarsMissing   = errors.New("campaign: the workflow needs campaign variables that are missing from a contact's metadata")
	ErrAgentNotFound         = errors.New("campaign: the agent does not exist in this workspace")
	ErrAgentVarsMissing      = agent.ErrAgentRequiredVariableMissing
	ErrAutomationUnavailable = errors.New("campaign: the workflow and agent checks are not available")
	ErrSheetsOfficialOnly    = errors.New("campaign: only official WhatsApp campaigns can choose a Planilha per conversation")
)

type EntryMetadata struct {
	Label     string
	Metadata  map[string]any
	Variables []string
}

func (e EntryMetadata) WithVariables(variables []string) EntryMetadata {
	e.Variables = variables
	return e
}

func (e EntryMetadata) Values() map[string]any {
	if len(e.Variables) == 0 {
		return e.Metadata
	}
	out := make(map[string]any, len(e.Metadata)+len(e.Variables))
	for k, v := range e.Metadata {
		out[k] = v
	}
	for i, v := range e.Variables {
		out[strconv.Itoa(i+1)] = v
	}
	return out
}

func EntryMetadataOf(leadID, number string, metadata map[string]any) EntryMetadata {
	label := "number " + number
	if leadID != "" {
		label = "lead " + leadID
	}
	return EntryMetadata{Label: label, Metadata: metadata}
}

func (a Automation) Configured() bool {
	return strings.TrimSpace(a.AgentID) != "" || strings.TrimSpace(a.WorkflowID) != ""
}

