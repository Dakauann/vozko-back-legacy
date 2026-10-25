package assignment_history_repository

import (
	"testing"

	ia "vozko/domain/inbox_assignment"
)

func TestAnUnresolvedDepartmentIsStoredAndReadBack(t *testing.T) {
	rec := toSchema(&ia.AssignmentHistory{WorkspaceID: "ws", EntryID: "e1", EntryType: "whatsapp", AssignedActorID: "u1", DepartmentUnresolved: true})
	if !rec.DepartmentUnresolved || rec.DepartmentID != "" {
		t.Fatalf("stored = %+v, want the department marked unresolved", rec)
	}
	if back := toDomain(rec); !back.DepartmentUnresolved {
		t.Fatalf("read back = %+v, want the department marked unresolved", back)
	}
}
