package agent_usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"vozko/domain/agent"
	"vozko/domain/agenttool"
	"vozko/domain/workspace"
)

type versionBook struct {
	versions []agent.Version
	pinned   map[agent.ToolPin]agent.ToolBinding
	unpub    []string
}

func (b *versionBook) PublishVersion(_ context.Context, agentID string, s agent.Snapshot, by string) (agent.Version, error) {
	v := agent.Version{AgentID: agentID, Version: len(b.versions) + 1, Snapshot: s, PublishedBy: by, PublishedAt: time.Now()}
	b.versions = append(b.versions, v)
	return v, nil
}

func (b *versionBook) CurrentVersion(context.Context, string) (*agent.Version, error) {
	if len(b.versions) == 0 {
		return nil, nil
	}
	v := b.versions[len(b.versions)-1]
	return &v, nil
}

func (b *versionBook) Versions(context.Context, string) ([]agent.Version, error) {
	return b.versions, nil
}

func (b *versionBook) VersionOf(_ context.Context, _ string, n int) (agent.Version, error) {
	if n < 1 || n > len(b.versions) {
		return agent.Version{}, agent.ErrAgentVersionMissing
	}
	return b.versions[n-1], nil
}

func (b *versionBook) PinnedTools(_ context.Context, _ string, pins []agent.ToolPin) ([]agent.ToolBinding, error) {
	var out []agent.ToolBinding
	for _, p := range pins {
		out = append(out, b.pinned[p])
	}
	return out, nil
}

func (b *versionBook) UnpublishedAgentIDs(_ context.Context, after string, _ int) ([]string, error) {
	var out []string
	for _, id := range b.unpub {
		if id > after {
			out = append(out, id)
		}
	}
	return out, nil
}

func publishFixture(v ToolValidator) (AgentPublishing, *versionBook, *agentStore) {
	_, update, store := sealingFixture(&vault{values: map[string]string{}})
	update.(ToolValidated).ValidateToolsWith(greenValidator{}, nil)
	book := &versionBook{pinned: map[agent.ToolPin]agent.ToolBinding{}}
	return NewPublishAgentUseCase(PublishDeps{Agents: store, Versions: book, Published: publishedAgents{}, Update: update, Validator: v}), book, store
}

var publisher = workspace.Requester{UserID: "u-9"}

func TestPublishingSnapshotsTheDraftOnceAndSkipsAnUnchangedDraft(t *testing.T) {
	uc, book, _ := publishFixture(greenValidator{})
	first, err := uc.Publish(context.Background(), "ag-http", publisher)
	if err != nil || first.Version != 1 || first.PublishedBy != "u-9" || len(first.Snapshot.Tools) != 1 {
		t.Fatalf("first %+v err %v", first, err)
	}
	again, err := uc.Publish(context.Background(), "ag-http", publisher)
	if err != nil || again.Version != 1 || len(book.versions) != 1 {
		t.Fatalf("an unchanged draft must not create a version: %+v", book.versions)
	}
	state, _ := uc.Publication(context.Background(), "ag-http")
	if state.PublishedVersion == nil || *state.PublishedVersion != 1 || state.HasDraftChanges {
		t.Fatalf("state %+v", state)
	}
}

func TestADraftWithBlockingIssuesIsNotPublished(t *testing.T) {
	uc, book, _ := publishFixture(&blockingValidator{})
	_, err := uc.Publish(context.Background(), "ag-http", publisher)
	var blocked *agenttool.BlockedError
	if !errors.As(err, &blocked) || !errors.Is(err, agenttool.ErrNotPublishable) || len(book.versions) != 0 {
		t.Fatalf("err %v versions %d", err, len(book.versions))
	}
	unwired, _, _ := publishFixture(nil)
	if _, err := unwired.Publish(context.Background(), "ag-http", publisher); !errors.Is(err, agenttool.ErrNotPublishable) {
		t.Fatalf("err %v", err)
	}
}

func TestAnUnpublishedAgentReportsDraftChanges(t *testing.T) {
	uc, _, store := publishFixture(greenValidator{})
	state, err := uc.Publication(context.Background(), "ag-http")
	if err != nil || state.PublishedVersion != nil || !state.HasDraftChanges {
		t.Fatalf("state %+v err %v", state, err)
	}
	_, _ = uc.Publish(context.Background(), "ag-http", publisher)
	store.byID["ag-http"].MessagingPrompt = "outro"
	if state, _ := uc.Publication(context.Background(), "ag-http"); !state.HasDraftChanges {
		t.Fatal("editing after publish leaves draft changes")
	}
}

func TestRestoringAVersionWritesItBackAsTheDraft(t *testing.T) {
	uc, book, store := publishFixture(greenValidator{})
	v1, _ := uc.Publish(context.Background(), "ag-http", publisher)
	book.pinned[v1.Snapshot.Tools[0]] = agent.ToolBinding{ID: v1.Snapshot.Tools[0].ID, Name: "http_request", Function: "http_request", Config: map[string]interface{}{"url": "https://v1.test", "method": "GET"}}
	store.byID["ag-http"].MessagingPrompt = "rascunho novo"
	restored, err := uc.Restore(context.Background(), "ag-http", 1, publisher)
	if err != nil || restored.MessagingPrompt != "p" || restored.InternalTools[0].Config["url"] != "https://v1.test" {
		t.Fatalf("restored %+v err %v", restored, err)
	}
	if _, err := uc.Restore(context.Background(), "ag-http", 7, publisher); !errors.Is(err, agent.ErrAgentVersionMissing) {
		t.Fatalf("err %v", err)
	}
}

func TestExistingAgentsArePublishedAsTheyAre(t *testing.T) {
	_, _, store := publishFixture(greenValidator{})
	book := &versionBook{unpub: []string{"ag-http"}}
	n, err := NewPublishExistingAgents(store, book, book).Run(context.Background())
	if err != nil || n != 1 || book.versions[0].Snapshot.MessagingPrompt != "p" || book.versions[0].PublishedBy != "" {
		t.Fatalf("n %d err %v versions %+v", n, err, book.versions)
	}
}
