package tools

import (
	"encoding/json"
	"fmt"
	"strings"
)

func (r *ExecutionResult) ModelPayload() string {
	if r == nil {
		return "{}"
	}
	payload := map[string]interface{}{"result": r.Result, "is_error": r.IsError}
	if ctx := strings.TrimSpace(r.ContextUpdateText); ctx != "" {
		payload["context_update"] = ctx
	}
	if r.ShouldEndSession {
		payload["should_end_session"] = true
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf(`{"result":"%v","is_error":%t}`, r.Result, r.IsError)
	}
	return string(b)
}
