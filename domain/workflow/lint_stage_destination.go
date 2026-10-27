package workflow

import "fmt"

const LintStageDestination LintIssueCode = "NODE_STAGE_DESTINATION"

func StageDestinationIssue(nodeID, stageID, pipelineName, stageName string) LintIssue {
	return LintIssue{
		Code:     LintStageDestination,
		Severity: SeverityInfo,
		NodeID:   nodeID,
		Field:    "stage_id",
		Message:  fmt.Sprintf("Conversas de outro funil vão para o funil %s, na etapa %s", pipelineName, stageName),
		Values:   map[string]string{"pipeline": pipelineName, "stage": stageName, "stage_id": stageID},
	}
}
