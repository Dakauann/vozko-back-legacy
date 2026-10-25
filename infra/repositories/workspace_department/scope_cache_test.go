package workspace_department_repository

import (
	"errors"
	"reflect"
	"testing"
	"time"

	workspace_department "vozko/domain/workspace/workspace_department"
	redisCache "vozko/infra/cache"
)

type scopeFixture struct {
	state    *memoryState
	inner    *fakeDepartments
	versions interface {
		Version(string) (string, error)
		Bump(string) error
	}
	cached workspace_department.Repository
}

func newScopeFixture() scopeFixture {
	state := newMemoryState()
	inner := newFakeDepartments()
	versions := NewScopeVersions(state)
	cached := NewScopeCachedRepository(inner, redisCache.NewSharedStateMemo(state, time.Second), versions)
	return scopeFixture{state: state, inner: inner, versions: versions, cached: cached}
}

func TestScopeCacheAnswersHasDepartmentsFromOneReadPerVersion(t *testing.T) {
	f := newScopeFixture()
	f.inner.has["ws-1"] = true

	for i := 0; i < 3; i++ {
		got, err := f.cached.HasDepartments("ws-1")
		if err != nil || !got {
			t.Fatalf("HasDepartments = %v, %v, want true", got, err)
		}
	}
	if f.inner.hasCalls != 1 {
		t.Fatalf("the database was read %d times, want once", f.inner.hasCalls)
	}

	got, err := f.cached.HasDepartments("ws-2")
	if err != nil || got {
		t.Fatalf("another workspace = %v, %v, want its own false answer", got, err)
	}
	if f.inner.hasCalls != 2 {
		t.Fatalf("each workspace is cached on its own, got %d reads", f.inner.hasCalls)
	}
}

func TestScopeCacheKeepsTheMemberDepartmentsExactly(t *testing.T) {
	f := newScopeFixture()
	f.inner.memberIDs["ws-1|ana"] = []string{"dept-b", "dept-a", "dept-b"}
	f.inner.memberIDs["ws-1|rui"] = []string{}

	for _, tc := range []struct {
		user string
		want []string
	}{
		{"ana", []string{"dept-b", "dept-a", "dept-b"}},
		{"rui", []string{}},
		{"nobody", nil},
	} {
		for i := 0; i < 2; i++ {
			got, err := f.cached.GetMemberDepartmentIDs("ws-1", tc.user)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("%s read %d = %#v, want %#v", tc.user, i, got, tc.want)
			}
		}
	}
	if f.inner.memberCalls != 3 {
		t.Fatalf("each member is read once, got %d reads", f.inner.memberCalls)
	}
}

func TestScopeCacheNeverKeepsAnError(t *testing.T) {
	f := newScopeFixture()
	boom := errors.New("connection reset")
	f.inner.hasErr = boom
	f.inner.memberErr = boom

	if _, err := f.cached.HasDepartments("ws-1"); !errors.Is(err, boom) {
		t.Fatalf("HasDepartments error = %v, want the database error", err)
	}
	if _, err := f.cached.GetMemberDepartmentIDs("ws-1", "ana"); !errors.Is(err, boom) {
		t.Fatalf("GetMemberDepartmentIDs error = %v, want the database error", err)
	}

	f.inner.hasErr = nil
	f.inner.memberErr = nil
	f.inner.has["ws-1"] = true
	f.inner.memberIDs["ws-1|ana"] = []string{"dept-a"}

	if got, err := f.cached.HasDepartments("ws-1"); err != nil || !got {
		t.Fatalf("after recovery HasDepartments = %v, %v, want the fresh true", got, err)
	}
	if got, err := f.cached.GetMemberDepartmentIDs("ws-1", "ana"); err != nil || !reflect.DeepEqual(got, []string{"dept-a"}) {
		t.Fatalf("after recovery GetMemberDepartmentIDs = %v, %v", got, err)
	}
}

func TestScopeCacheExpiresWithTheSameBackstopAsTheMemberCache(t *testing.T) {
	f := newScopeFixture()
	if _, err := f.cached.HasDepartments("ws-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.cached.GetMemberDepartmentIDs("ws-1", "ana"); err != nil {
		t.Fatal(err)
	}
	ttls := f.state.ttlsSet()
	if len(ttls) != 2 {
		t.Fatalf("stored %d entries, want 2", len(ttls))
	}
	for _, ttl := range ttls {
		if ttl != 60*time.Second {
			t.Fatalf("ttl = %v, want 60s", ttl)
		}
	}
}

func TestABumpedWorkspaceIsReadAgainAndOthersStayCached(t *testing.T) {
	f := newScopeFixture()
	f.inner.memberIDs["ws-1|ana"] = []string{"dept-a"}
	f.inner.memberIDs["ws-2|ana"] = []string{"dept-z"}

	for _, ws := range []string{"ws-1", "ws-2"} {
		if _, err := f.cached.HasDepartments(ws); err != nil {
			t.Fatal(err)
		}
		if _, err := f.cached.GetMemberDepartmentIDs(ws, "ana"); err != nil {
			t.Fatal(err)
		}
	}

	f.inner.has["ws-1"] = true
	f.inner.memberIDs["ws-1|ana"] = []string{"dept-a", "dept-b"}
	if err := f.versions.Bump("ws-1"); err != nil {
		t.Fatal(err)
	}

	has, err := f.cached.HasDepartments("ws-1")
	if err != nil || !has {
		t.Fatalf("after the bump HasDepartments = %v, %v, want the new true", has, err)
	}
	ids, err := f.cached.GetMemberDepartmentIDs("ws-1", "ana")
	if err != nil || !reflect.DeepEqual(ids, []string{"dept-a", "dept-b"}) {
		t.Fatalf("after the bump GetMemberDepartmentIDs = %v, %v", ids, err)
	}
	if _, err := f.cached.HasDepartments("ws-2"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.cached.GetMemberDepartmentIDs("ws-2", "ana"); err != nil {
		t.Fatal(err)
	}
	if f.inner.hasCalls != 3 || f.inner.memberCalls != 3 {
		t.Fatalf("only the bumped workspace is re-read: has=%d member=%d, want 3 and 3", f.inner.hasCalls, f.inner.memberCalls)
	}
}

func TestScopeCacheReadsTheDatabaseWhenTheVersionIsUnreadable(t *testing.T) {
	state := newMemoryState()
	inner := newFakeDepartments()
	inner.has["ws-1"] = true
	cached := NewScopeCachedRepository(inner, redisCache.NewSharedStateMemo(state, time.Second), unreadableVersions{})

	for i := 0; i < 2; i++ {
		if got, err := cached.HasDepartments("ws-1"); err != nil || !got {
			t.Fatalf("HasDepartments = %v, %v", got, err)
		}
		if _, err := cached.GetMemberDepartmentIDs("ws-1", "ana"); err != nil {
			t.Fatal(err)
		}
	}
	if inner.hasCalls != 2 || inner.memberCalls != 2 {
		t.Fatalf("without a version every read goes to the database: has=%d member=%d", inner.hasCalls, inner.memberCalls)
	}
	if len(state.ttlsSet()) != 0 {
		t.Fatal("nothing may be stored without a version")
	}
}

func TestScopeCacheDoesNotCacheAnEmptyWorkspace(t *testing.T) {
	f := newScopeFixture()
	for i := 0; i < 2; i++ {
		if _, err := f.cached.HasDepartments(""); err != nil {
			t.Fatal(err)
		}
	}
	if f.inner.hasCalls != 2 {
		t.Fatalf("an empty workspace id is passed straight through, got %d reads", f.inner.hasCalls)
	}
}

func TestADepartmentWriteThroughTheVersionedRepositoryRefreshesTheCache(t *testing.T) {
	f := newScopeFixture()
	f.inner.departments["dept-a"] = &workspace_department.Department{ID: "dept-a", WorkspaceID: "ws-1"}
	versioned := NewScopeVersionedRepository(f.inner, f.versions)

	if ids, _ := f.cached.GetMemberDepartmentIDs("ws-1", "ana"); len(ids) != 0 {
		t.Fatalf("ana starts in no department, got %v", ids)
	}
	f.inner.memberIDs["ws-1|ana"] = []string{"dept-a"}
	if err := versioned.AddMember(&workspace_department.DepartmentMember{DepartmentID: "dept-a", MemberID: "member-ana"}); err != nil {
		t.Fatal(err)
	}

	ids, err := f.cached.GetMemberDepartmentIDs("ws-1", "ana")
	if err != nil || !reflect.DeepEqual(ids, []string{"dept-a"}) {
		t.Fatalf("after joining, ana's departments = %v, %v, want [dept-a] at once", ids, err)
	}
}
