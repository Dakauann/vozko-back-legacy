package database

import (
	"errors"

	"gorm.io/gorm"

	"vozko/domain/workflow"
)

var errNodeOutputsUnwired = errors.New("the tool mode repair needs the node output catalog")

func toolModeGraphRepair(outputs workflow.DynamicHandleResolver) graphRepair {
	return graphRepair{name: "tool_mode", path: workflow.ToolModeRetirementGraphPath(), rewrite: func(raw []byte) (rewrittenGraph, error) {
		result, err := workflow.RetireToolModeJSON(raw, outputs)
		return rewrittenGraph{graph: result.Graph, changed: result.Changed, held: result.Held}, err
	}}
}

func retireToolModes(outputs workflow.DynamicHandleResolver) func(*gorm.DB) error {
	return func(tx *gorm.DB) error {
		if outputs == nil {
			return errNodeOutputsUnwired
		}
		if !storedGraphTablesExist(tx) {
			return nil
		}
		return repairStoredGraphs(tx, toolModeGraphRepair(outputs), workflowGraphCanonicalBatch)
	}
}
