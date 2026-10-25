package conversation

import (
	"testing"

	"vozko/domain/conversation"
	"vozko/domain/workspace"
)

var _ conversation.EntryAccessRevoker = (*Authorizer)(nil)

type movableResolver struct {
	stubResolver
	department string
}

func (r *movableResolver) GetEntryDepartmentID(string, string) (string, error) {
	return r.department, nil
}

func departmentScopedAuthorizer(resolver *movableResolver) *Authorizer {
	return NewAuthorizer(
		stubEntryAccess{},
		&stubPlacements{},
		stubMembership{permissions: map[workspace.Action]bool{}},
		stubDepartments{},
		&stubAssignments{},
		resolver,
		&stubShared{data: map[string]string{}},
	).(*Authorizer)
}

func TestRevokingEntryAccessDropsTheCachedGrantAtOnce(t *testing.T) {
	resolver := &movableResolver{department: testDepartment}
	a := departmentScopedAuthorizer(resolver)

	if !a.CanAccessEntry(viewer, testWorkspace, testEntry, testEntryType, false) {
		t.Fatal("a member of the conversation's department must open it")
	}
	resolver.department = "dept-elsewhere"
	if !a.CanAccessEntry(viewer, testWorkspace, testEntry, testEntryType, false) {
		t.Fatal("precondition: the grant is cached")
	}

	if err := a.RevokeEntryAccess(testEntry); err != nil {
		t.Fatal(err)
	}

	if a.CanAccessEntry(viewer, testWorkspace, testEntry, testEntryType, false) {
		t.Fatal("the old department kept access after the conversation was transferred")
	}
}

func TestAGrantComputedBeforeARevokeIsNeverHonoured(t *testing.T) {
	a := departmentScopedAuthorizer(&movableResolver{department: testDepartment})
	cacheKey := viewer + ":" + testWorkspace

	stale := a.grantFor(cacheKey, testEntry)
	if err := a.RevokeEntryAccess(testEntry); err != nil {
		t.Fatal(err)
	}
	a.remember(stale)

	if a.grantFor(cacheKey, testEntry).cached {
		t.Fatal("a grant decided before the transfer was stored after it and honoured")
	}
}

func TestRevokingOneConversationKeepsTheOthersCached(t *testing.T) {
	a := departmentScopedAuthorizer(&movableResolver{department: testDepartment})
	cacheKey := viewer + ":" + testWorkspace
	a.remember(a.grantFor(cacheKey, "entry-other"))

	if err := a.RevokeEntryAccess(testEntry); err != nil {
		t.Fatal(err)
	}

	if !a.grantFor(cacheKey, "entry-other").cached {
		t.Fatal("revoking one conversation dropped another conversation's grant")
	}
}
