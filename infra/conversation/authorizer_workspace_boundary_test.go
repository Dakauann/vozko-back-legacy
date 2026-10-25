package conversation

import (
	"testing"

	"vozko/domain/workspace"
)

type entryOfWorkspace struct{ workspaceID string }

func (e entryOfWorkspace) CanUserAccessEntry(workspaceID, _ string, _ bool) (bool, error) {
	return workspaceID == e.workspaceID, nil
}

func (entryOfWorkspace) GetAccessibleEntryIDs(string, bool) ([]string, error) { return nil, nil }

func TestANonAdminNeverReachesAnEntryFromAnotherWorkspace(t *testing.T) {
	members := stubMembership{permissions: map[workspace.Action]bool{workspace.ActionViewOthers: true}}
	a := NewAuthorizer(entryOfWorkspace{workspaceID: testWorkspace}, &stubPlacements{}, members, &scriptedDepartments{},
		&stubAssignments{}, stubResolver{}, &stubShared{data: map[string]string{}}).(*Authorizer)

	if !a.ResolveEntryAccess(testWorkspace, testEntry, testEntryType)(viewer, false) {
		t.Fatal("the entry's own workspace must reach it, which also stores a grant")
	}

	for _, other := range []string{"ws-2", ""} {
		if a.ResolveEntryAccess(other, testEntry, testEntryType)(viewer, false) {
			t.Fatalf("a non-admin connected to %q reached an entry of %s", other, testWorkspace)
		}
		if a.CanAccessEntry(viewer, other, testEntry, testEntryType, false) {
			t.Fatalf("CanAccessEntry let a non-admin connected to %q reach an entry of %s", other, testWorkspace)
		}
	}
}
