package automationneeds_repository

import (
	"context"
	"reflect"
	"testing"

	"gorm.io/gorm"

	"vozko/domain/conversationvars"
	"vozko/domain/shared"
)

func TestForgettingRemovesOnlyTheNamedVariablesOfThatConversation(t *testing.T) {
	inRolledBackTx(t, func(tx *gorm.DB) {
		repo := NewConversationVariables(tx)
		ctx := context.Background()
		ref := conversationvars.Ref{WorkspaceID: "00000000-0000-0000-0000-0000000000a1", EntryID: "entry-1", EntryType: shared.EntryTypeInstagram}
		other := conversationvars.Ref{WorkspaceID: ref.WorkspaceID, EntryID: "entry-2", EntryType: shared.EntryTypeInstagram}
		if err := repo.Save(ctx, ref, map[string]string{"turma": "A", "escola": "Prisma"}, "ai:ag-1"); err != nil {
			t.Fatal(err)
		}
		if err := repo.Save(ctx, other, map[string]string{"turma": "B"}, "ai:ag-1"); err != nil {
			t.Fatal(err)
		}

		if err := repo.Forget(ctx, ref, []string{"turma"}); err != nil {
			t.Fatal(err)
		}

		got, err := repo.Values(ctx, "entry-1", shared.EntryTypeInstagram)
		if err != nil || !reflect.DeepEqual(got, map[string]string{"escola": "Prisma"}) {
			t.Fatalf("values = %v, %v", got, err)
		}
		kept, err := repo.Values(ctx, "entry-2", shared.EntryTypeInstagram)
		if err != nil || kept["turma"] != "B" {
			t.Fatalf("another conversation keeps its value: %v, %v", kept, err)
		}
		if err := repo.Forget(ctx, ref, nil); err != nil {
			t.Fatalf("forgetting nothing is a no-op: %v", err)
		}
	})
}
