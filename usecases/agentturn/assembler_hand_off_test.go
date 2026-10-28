package agentturn

import (
	"context"
	"strings"
	"testing"

	"vozko/domain/ai"
)

type handOffLineStub struct {
	line  string
	asked []string
}

func (s *handOffLineStub) HandOffLine(workspaceID, entryID, entryType, agentID string) string {
	s.asked = append(s.asked, workspaceID+"/"+entryID+"/"+entryType+"/"+agentID)
	return s.line
}

func TestTheHandOffNoteRidesInTheTrailingNoteForTheAgentThatReceivedIt(t *testing.T) {
	handOffs := &handOffLineStub{line: "Agente Prospecção transferiu esta conversa para você há 2 min. Motivo e resumo de quem transferiu: é aluno."}
	a := cachedAssembler(nil)
	a.TrackHandOffs(handOffs)
	req := turnRequest(leadIdentity("5511", "Rosalinda", "conv-ana", nil), []ai.Message{{Role: ai.RoleUser, Content: "oi"}}, "quero a segunda via")
	req.ToolSeed = map[string]interface{}{"__entry_id": "entry-1", "__entry_type": "whatsapp"}

	out := a.Assemble(context.Background(), req)

	note := out.Input.Messages[len(out.Input.Messages)-1]
	if !strings.Contains(note.Content, handOffHeading) || !strings.Contains(note.Content, "é aluno") {
		t.Fatalf("note = %q", note.Content)
	}
	if strings.Contains(out.Input.SystemPrompt, "é aluno") {
		t.Fatal("the hand-off note must stay out of the cached system prompt")
	}
	if len(handOffs.asked) != 1 || handOffs.asked[0] != "ws-1/entry-1/whatsapp/"+req.Agent.ID {
		t.Fatalf("asked = %v", handOffs.asked)
	}
}

func TestNoHandOffMeansNoHandOffSection(t *testing.T) {
	a := cachedAssembler(nil)
	a.TrackHandOffs(&handOffLineStub{})

	out := a.Assemble(context.Background(), turnRequest(leadIdentity("5511", "Rosalinda", "conv-ana", nil), nil, "oi"))

	if strings.Contains(out.Input.Messages[len(out.Input.Messages)-1].Content, handOffHeading) {
		t.Fatalf("note = %q", out.Input.Messages[len(out.Input.Messages)-1].Content)
	}
}
