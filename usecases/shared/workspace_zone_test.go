package shared

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fixedZones struct {
	loc         *time.Location
	err         error
	workspaceID string
	department  string
}

func (z *fixedZones) Location(_ context.Context, workspaceID, departmentID string) (*time.Location, error) {
	z.workspaceID, z.department = workspaceID, departmentID
	return z.loc, z.err
}

func TestTheWorkspaceZoneIsTheWorkspaceScheduleZone(t *testing.T) {
	recife, err := time.LoadLocation("America/Recife")
	if err != nil {
		t.Fatal(err)
	}
	zones := &fixedZones{loc: recife}

	got := WorkspaceZone(context.Background(), zones, "ws-1")

	if got != recife {
		t.Fatalf("zone = %v, want America/Recife", got)
	}
	if zones.workspaceID != "ws-1" || zones.department != "" {
		t.Fatalf("asked for %q / %q, want the workspace-wide zone of ws-1", zones.workspaceID, zones.department)
	}
}

func TestTheWorkspaceZoneFailsClosedToUTC(t *testing.T) {
	cases := map[string]struct {
		zones       WorkspaceZones
		workspaceID string
	}{
		"the lookup fails":      {zones: &fixedZones{err: errors.New("db down")}, workspaceID: "ws-1"},
		"the lookup finds none": {zones: &fixedZones{}, workspaceID: "ws-1"},
		"no lookup is wired":    {zones: nil, workspaceID: "ws-1"},
		"no workspace is known": {zones: &fixedZones{loc: time.Local}, workspaceID: "  "},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := WorkspaceZone(context.Background(), tc.zones, tc.workspaceID); got != time.UTC {
				t.Fatalf("zone = %v, want UTC", got)
			}
		})
	}
}
