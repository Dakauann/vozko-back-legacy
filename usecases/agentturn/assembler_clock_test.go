package agentturn

import (
	"context"
	"errors"
	"testing"
	"time"

	"vozko/domain/tools"
)

type workspaceZones struct {
	loc   *time.Location
	err   error
	asked []string
}

func (z *workspaceZones) Location(_ context.Context, workspaceID, _ string) (*time.Location, error) {
	z.asked = append(z.asked, workspaceID)
	return z.loc, z.err
}

func zonedAssembler(zones *workspaceZones) *Assembler {
	reg := stubRegistry{defs: []tools.Definition{{Name: "book_meeting"}, {Name: "manage_entry_stage"}}}
	return New(reg, echoRAG{}, nil, zones)
}

func TestTheTurnTellsTheModelTheWorkspaceZone(t *testing.T) {
	manaus, err := time.LoadLocation("America/Manaus")
	if err != nil {
		t.Fatal(err)
	}
	zones := &workspaceZones{loc: manaus}

	out := zonedAssembler(zones).Assemble(context.Background(), turnRequest(leadIdentity("5511", "Rosalinda", "conv-ana", nil), nil, "o comprovante é de hoje"))

	if out.Input.Zone != manaus {
		t.Fatalf("zone = %v, want America/Manaus", out.Input.Zone)
	}
	if len(zones.asked) != 1 || zones.asked[0] != "ws-1" {
		t.Fatalf("zone lookups = %v, want one for ws-1", zones.asked)
	}
}

func TestAnUnreadableWorkspaceZoneFallsBackToUTC(t *testing.T) {
	out := zonedAssembler(&workspaceZones{err: errors.New("config down")}).Assemble(context.Background(), turnRequest(leadIdentity("5511", "Rosalinda", "conv-ana", nil), nil, "oi"))

	if out.Input.Zone != time.UTC {
		t.Fatalf("zone = %v, want UTC", out.Input.Zone)
	}
}

func TestTheZoneLeavesTheCachedSystemPromptAlone(t *testing.T) {
	recife, err := time.LoadLocation("America/Recife")
	if err != nil {
		t.Fatal(err)
	}
	req := turnRequest(leadIdentity("5511", "Rosalinda", "conv-ana", nil), nil, "oi")

	utc := zonedAssembler(&workspaceZones{loc: time.UTC}).Assemble(context.Background(), req)
	local := zonedAssembler(&workspaceZones{loc: recife}).Assemble(context.Background(), req)

	if utc.Input.SystemPrompt != local.Input.SystemPrompt {
		t.Fatalf("the zone changed the system prompt:\n%s\n---\n%s", utc.Input.SystemPrompt, local.Input.SystemPrompt)
	}
}
