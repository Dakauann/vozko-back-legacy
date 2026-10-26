package leadsentiment

import (
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"

	"vozko/infra/database"
)

const explainedLeads = 3000

func TestExplainTheRefreshRuleOnAModestSeedAgainstPostgres(t *testing.T) {
	db := projectionDB(t)
	s := newSeed(t, db)
	err := db.Exec(`
		WITH seeded AS (
			INSERT INTO leads (id, workspace_id, name, created_at, updated_at)
			SELECT gen_random_uuid(), ?, 'Lead ' || g, now(), now() FROM generate_series(1, ?) g
			RETURNING id
		), contacts AS (
			INSERT INTO telegram_contacts (id, workspace_id, account_id, tg_user_id, tg_chat_id, lead_id, created_at, updated_at)
			SELECT gen_random_uuid(), ?, gen_random_uuid(), (random() * 1e12)::bigint, (random() * 1e12)::bigint, s.id, now(), now() FROM seeded s
			RETURNING id, tg_chat_id
		), conversations AS (
			INSERT INTO telegram_conversations (id, workspace_id, account_id, contact_id, tg_chat_id, created_at, updated_at)
			SELECT gen_random_uuid(), ?, gen_random_uuid(), c.id, c.tg_chat_id, now(), now() FROM contacts c
			RETURNING id
		)
		INSERT INTO audience_analyses (id, workspace_id, subject_kind, revision, source, account_id, container_id, subject_id, author_external_id, status, sentiment, occurred_at, created_at, updated_at)
		SELECT gen_random_uuid(), ?, 'conversation', r::text, 'telegram', gen_random_uuid(), c.id::text, c.id::text, 'x',
			CASE WHEN r = 3 THEN 'pending' ELSE 'analyzed' END, (?::text[])[r], now() - (4 - r) * interval '1 hour', now(), now()
		FROM conversations c CROSS JOIN generate_series(1, 3) r`,
		s.ws, explainedLeads, s.ws, s.ws, s.ws, pq.Array([]string{"positive", "negative", ""})).Error
	if err != nil {
		t.Fatal(err)
	}
	revisions, ok := database.PerformanceIndexSQL("idx_ca_latest_revision")
	if !ok {
		t.Fatal("idx_ca_latest_revision is not declared")
	}
	for _, sql := range append(database.LeadSentimentIndexStatements(), revisions) {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []string{"leads", "telegram_contacts", "telegram_conversations", "audience_analyses", "lead_sentiments"} {
		if err := db.Exec("VACUUM ANALYZE " + table).Error; err != nil {
			t.Fatal(err)
		}
	}
	var ids []string
	if err := db.Raw("SELECT id::text FROM leads WHERE workspace_id = ? ORDER BY id LIMIT ?", s.ws, MaxLeadsPerRefresh).Scan(&ids).Error; err != nil {
		t.Fatal(err)
	}
	sql, args := statement(s.ws, "SELECT unnest(?::uuid[])", []interface{}{pq.Array(ids)})
	var plan []string
	started := time.Now()
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL jit = off").Error; err != nil {
			return err
		}
		return tx.Raw("EXPLAIN (ANALYZE, BUFFERS) "+sql, args...).Scan(&plan).Error
	})
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(started)
	t.Logf("refreshing %d of %d leads (%s):\n%s", len(ids), explainedLeads, elapsed, strings.Join(plan, "\n"))
	if elapsed > 5*time.Second {
		t.Errorf("one refresh chunk took %s", elapsed)
	}
	var written int
	db.Raw("SELECT count(*) FROM lead_sentiments WHERE workspace_id = ? AND sentiment = 'negative'", s.ws).Scan(&written)
	if written != len(ids) {
		t.Fatalf("%d leads read negative, want the %d refreshed", written, len(ids))
	}
}
