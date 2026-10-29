package aiusage_repository

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"vozko/domain/aiusage"
	"vozko/infra/database/schema"
	"vozko/infra/repositories/repotest"
)

func TestUsageUnderAReferenceSumsTokensAndCountsCallsAndBilledRows(t *testing.T) {
	db := repotest.IsolatedDB(t, "ai_usage", &schema.AIUsageRecord{})
	repo := NewUsageRepository(db)
	ws := uuid.New().String()
	other := uuid.New().String()
	records := []aiusage.Record{
		{ReferenceID: "aichat:th-1:a", WorkspaceID: ws, Model: "m", Billed: true, Tokens: aiusage.Tokens{Input: 1000, Output: 100, CacheRead: 800, CacheWrite: 50, Reasoning: 20}},
		{ReferenceID: "aichat:th-1:b", WorkspaceID: ws, Model: "m", Tokens: aiusage.Tokens{Input: 2000, Output: 200, CacheRead: 1500}},
		{ReferenceID: "aichat:th-1:job", WorkspaceID: ws, Model: "imagen", Billed: true},
		{ReferenceID: "aichat:th-10:c", WorkspaceID: ws, Model: "m", Billed: true, Tokens: aiusage.Tokens{Input: 9999}},
		{ReferenceID: "aichat:th-1:e", WorkspaceID: other, Model: "m", Billed: true, Tokens: aiusage.Tokens{Input: 9999}},
		{ReferenceID: "aichat:t_1:f", WorkspaceID: ws, Model: "m", Billed: true, Tokens: aiusage.Tokens{Input: 9999}},
	}
	for _, record := range records {
		if err := repo.Record(record); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.Record(records[0]); err != nil {
		t.Fatalf("recording the same reference again must be a no-op, got %v", err)
	}

	totals, err := repo.TotalsUnder(ws, "aichat:th-1:")
	if err != nil {
		t.Fatal(err)
	}
	want := aiusage.Totals{Calls: 2, Billed: 2, LatestInput: 2000, Tokens: aiusage.Tokens{Input: 3000, Output: 300, CacheRead: 2300, CacheWrite: 50, Reasoning: 20}}
	if totals != want {
		t.Fatalf("totals = %+v, want %+v", totals, want)
	}
}

func TestUsageUnderAnUnknownReferenceIsEmpty(t *testing.T) {
	db := repotest.IsolatedDB(t, "ai_usage", &schema.AIUsageRecord{})
	totals, err := NewUsageRepository(db).TotalsUnder(uuid.New().String(), "aichat:none:")
	if err != nil {
		t.Fatal(err)
	}
	if totals != (aiusage.Totals{}) {
		t.Fatalf("totals = %+v, want zero", totals)
	}
}

func TestTheContextIsTheInputOfTheLatestModelCall(t *testing.T) {
	db := repotest.IsolatedDB(t, "ai_usage", &schema.AIUsageRecord{})
	repo := NewUsageRepository(db)
	ws := uuid.New().String()
	base := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	rows := []schema.AIUsageRecord{
		{ReferenceID: "aichat:th-1:a", WorkspaceID: ws, Model: "m", Billed: true, InputTokens: 12_000, OutputTokens: 300, CreatedAt: base},
		{ReferenceID: "aichat:th-1:b", WorkspaceID: ws, Model: "m", Billed: true, InputTokens: 30_000, OutputTokens: 200, CreatedAt: base.Add(time.Minute)},
		{ReferenceID: "aichat:th-1:job", WorkspaceID: ws, Model: "imagen", Billed: true, CreatedAt: base.Add(2 * time.Minute)},
		{ReferenceID: "aichat:th-2:c", WorkspaceID: ws, Model: "m", Billed: true, InputTokens: 99_000, OutputTokens: 1, CreatedAt: base.Add(3 * time.Minute)},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	totals, err := repo.TotalsUnder(ws, "aichat:th-1:")
	if err != nil {
		t.Fatal(err)
	}
	if totals.LatestInput != 30_000 || totals.Tokens.Input != 42_000 {
		t.Fatalf("totals = %+v, want the latest call's 30000 input as context", totals)
	}
}
