package systemconfig

import (
	"encoding/json"
	"strings"
	"testing"

	"vozko/domain/config"
)

func TestTheSystemConfigNoLongerOffersABasePromptNothingReads(t *testing.T) {
	raw, err := json.Marshal(toSystemConfigResponse(&config.SystemConfig{ID: "default", MaxConcurrentCalls: 30}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "baseSystemPrompt") || !strings.Contains(string(raw), "maxConcurrentCalls") {
		t.Fatalf("got %s", raw)
	}
}
