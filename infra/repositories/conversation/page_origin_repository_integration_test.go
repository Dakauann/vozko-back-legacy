package conversation_repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/infra/database/schema"
	"vozko/infra/repositories/repotest"
)

func TestOnlyTheFirstPageOfAConversationIsKeptEvenUnderRaces(t *testing.T) {
	repo := NewPageOriginRepository(repotest.IsolatedDB(t, "pageorigin_test", &schema.ConversationPageOrigin{}))
	ctx := context.Background()
	entry, workspace, project := uuid.NewString(), uuid.NewString(), uuid.NewString()
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			route := "/primeira"
			if i > 0 {
				route = "/concorrente"
			}
			claimed, err := repo.Claim(ctx, conversation.PageOrigin{EntryID: entry, EntryType: shared.EntryTypeWhatsApp, WorkspaceID: workspace, ProjectID: project, Route: route, Reference: "LP7K2", ArrivedAt: time.Now()})
			if err != nil {
				t.Error(err)
			}
			if claimed {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("wins = %d, want exactly one", wins)
	}
	got, err := repo.Get(ctx, entry, shared.EntryTypeWhatsApp)
	if err != nil || got == nil || got.ProjectID != project || got.Reference != "LP7K2" || got.WorkspaceID != workspace {
		t.Fatalf("got %+v %v", got, err)
	}
	none, err := repo.Get(ctx, uuid.NewString(), shared.EntryTypeWhatsApp)
	if err != nil || none != nil {
		t.Fatalf("none = %+v %v", none, err)
	}
}
