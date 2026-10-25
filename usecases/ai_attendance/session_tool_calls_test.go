package ai_attendance_usecase

import (
	"testing"

	aa "vozko/domain/ai_attendance"
)

func TestRecordToolCallsAddsToTheOpenSessionAndOpensOneForTheFirstTurn(t *testing.T) {
	repo := newMem()
	svc := NewSessionService(repo, nil)
	in := aa.StartInput{WorkspaceID: "ws", EntryID: "e1", EntryType: "telegram", AgentID: "ag1", Channel: "telegram"}

	svc.RecordToolCalls(in, 2)
	svc.RecordToolCalls(in, 1)
	svc.RecordToolCalls(in, 0)

	open, _ := repo.FindOpenByEntry("ws", "e1", "telegram")
	if open == nil {
		t.Fatal("the first tool call must open the attendance session")
	}
	if repo.toolCalls[open.ID] != 3 {
		t.Fatalf("tool calls = %d, want 3", repo.toolCalls[open.ID])
	}
}
