package agent

import (
	"testing"

	"vozko/domain/agent"
)

type publishedRepo struct {
	*fakeAgentRepo
	reads int
}

func (p *publishedRepo) FindPublished(id string) (*agent.Agent, error) {
	p.reads++
	return &agent.Agent{ID: id, Name: "Publicado", RunningVersion: 4}, nil
}

func TestTheCachedPublishedAgentKeepsTheVersionItRuns(t *testing.T) {
	inner := &publishedRepo{fakeAgentRepo: newFakeAgentRepo()}
	shared := newFakeShared()
	repo := NewCachedRepository(inner, shared).(*CachedRepository)
	first, err := repo.FindPublished("ag-1")
	if err != nil || first.RunningVersion != 4 {
		t.Fatalf("first %+v %v", first, err)
	}
	cached, err := repo.FindPublished("ag-1")
	if err != nil || cached.RunningVersion != 4 || cached.Name != "Publicado" || inner.reads != 1 {
		t.Fatalf("cached %+v %v reads %d", cached, err, inner.reads)
	}
	shared.data[publishedCacheKey("ag-1")] = `{"id":"ag-1","name":"formato antigo"}`
	if again, _ := repo.FindPublished("ag-1"); again.RunningVersion != 4 || inner.reads != 2 {
		t.Fatalf("an entry from before the version was cached must be read again: %+v reads %d", again, inner.reads)
	}
}
