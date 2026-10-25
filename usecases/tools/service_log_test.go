package tools_usecase

import (
	"bytes"
	"context"
	"errors"
	"log"
	"log/slog"
	"strings"
	"testing"

	"vozko/domain/tools"
)

type echoHandler struct{ fail bool }

func (echoHandler) Definition() tools.Definition { return tools.Definition{Name: "echo_tool"} }

func (h echoHandler) Execute(ctx context.Context, params map[string]interface{}) (tools.ExecutionResult, error) {
	return h.ExecuteWithConfig(ctx, nil, params)
}

func (h echoHandler) ExecuteWithConfig(_ context.Context, _ map[string]interface{}, params map[string]interface{}) (tools.ExecutionResult, error) {
	if h.fail {
		return tools.ExecutionResult{IsError: true}, errors.New("upstream said token=abc123 for cpf 529.982.247-25")
	}
	return tools.ExecutionResult{Result: map[string]interface{}{"echo": params["note"], "private": "resultado-sigiloso"}}, nil
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prevLog, prevFlags := log.Writer(), log.Flags()
	prevSlog := slog.Default()
	log.SetOutput(&buf)
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() {
		log.SetOutput(prevLog)
		log.SetFlags(prevFlags)
		slog.SetDefault(prevSlog)
	})
	return &buf
}

func TestToolExecutionLogsNeitherArgumentsNorResults(t *testing.T) {
	logs := captureLogs(t)
	svc := NewService(echoHandler{})
	args := map[string]interface{}{"note": "argumento-sigiloso"}
	if _, err := svc.Execute(context.Background(), "echo_tool", args); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ExecuteWithConfig(context.Background(), "echo_tool", map[string]interface{}{"headers": map[string]interface{}{"X": "cfg-sigilosa"}}, args); err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"argumento-sigiloso", "resultado-sigiloso", "cfg-sigilosa"} {
		if strings.Contains(logs.String(), leak) {
			t.Fatalf("%q reached the logs: %s", leak, logs.String())
		}
	}
	if !strings.Contains(logs.String(), "echo_tool") {
		t.Fatalf("the log must still name the tool: %s", logs.String())
	}
}

func TestToolFailureLogsAreRedacted(t *testing.T) {
	logs := captureLogs(t)
	_, _ = NewService(echoHandler{fail: true}).Execute(context.Background(), "echo_tool", nil)
	for _, leak := range []string{"abc123", "529.982.247"} {
		if strings.Contains(logs.String(), leak) {
			t.Fatalf("%q reached the logs: %s", leak, logs.String())
		}
	}
}
