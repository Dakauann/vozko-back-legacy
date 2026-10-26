package node_executors

import (
	"strings"

	"vozko/domain/workflow"
)

type writeValues struct {
	ctx        *workflow.NodeContext
	unresolved []string
}

func writeValuesOf(ctx *workflow.NodeContext) *writeValues {
	return &writeValues{ctx: ctx}
}

func (w *writeValues) field(key string) string {
	raw, _ := w.ctx.Node.Config[key].(string)
	return w.text(raw)
}

func (w *writeValues) text(raw string) string {
	resolved, unresolved := workflow.InterpolateForWrite(raw, w.ctx.State)
	if len(unresolved) > 0 {
		w.ctx.Trace.RecordUnresolved(unresolved...)
		w.unresolved = append(w.unresolved, unresolved...)
		return ""
	}
	return strings.TrimSpace(resolved)
}

func (w *writeValues) refusal() (string, bool) {
	if len(w.unresolved) == 0 {
		return "", false
	}
	return workflow.UnresolvedWriteReason(w.unresolved), true
}
