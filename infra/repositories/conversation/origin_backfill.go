package conversation_repository

import (
	"context"
	"sync"

	"github.com/lib/pq"
	"gorm.io/gorm"

	"vozko/domain/shared/deps"
	"vozko/infra/database/sqlexpr"
)

const (
	originBackfillStart   = "00000000-0000-0000-0000-000000000000"
	originBackfillBatch   = 1000
	originBackfillPerRun  = 50
	originBackfillNoEntry = ""
)

const entriesAfterSQL = `WITH RECURSIVE walk(entry_id, entry_type, step) AS (
		(SELECT cm.entry_id, cm.entry_type::text, 1
			FROM conversation_messages cm
			WHERE (cm.entry_id, cm.entry_type) > (?::uuid, ?::text)
			ORDER BY cm.entry_id, cm.entry_type
			LIMIT 1)
		UNION ALL
		SELECT next_entry.entry_id, next_entry.entry_type::text, walk.step + 1
		FROM walk
		CROSS JOIN LATERAL (
			SELECT cm.entry_id, cm.entry_type
			FROM conversation_messages cm
			WHERE (cm.entry_id, cm.entry_type) > (walk.entry_id, walk.entry_type)
			ORDER BY cm.entry_id, cm.entry_type
			LIMIT 1
		) next_entry
		WHERE walk.step < ?
	)
	SELECT entry_id::text AS entry_id, entry_type FROM walk ORDER BY step`

func unsettledEntriesSQL() string {
	return `SELECT e.entry_id, e.entry_type FROM unnest(?::uuid[], ?::text[]) AS e(entry_id, entry_type)
		WHERE NOT EXISTS (SELECT 1 FROM ` + sqlexpr.ConversationOriginTable + ` settled
			WHERE settled.entry_id = e.entry_id AND settled.entry_type = e.entry_type)`
}

type entryKey struct {
	EntryID   string
	EntryType string
}

type OriginBackfill struct {
	db     *gorm.DB
	batch  int
	perRun int

	mu    sync.Mutex
	after entryKey
	done  bool
}

func NewOriginBackfill(db *gorm.DB) (*OriginBackfill, error) {
	if err := deps.Require("conversation origin backfill", map[string]any{"database": db}); err != nil {
		return nil, err
	}
	return newOriginBackfill(db, originBackfillBatch, originBackfillPerRun), nil
}

func newOriginBackfill(db *gorm.DB, batch, perRun int) *OriginBackfill {
	return &OriginBackfill{
		db: db, batch: batch, perRun: perRun,
		after: entryKey{EntryID: originBackfillStart, EntryType: originBackfillNoEntry},
	}
}

func (b *OriginBackfill) Run(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for chunk := 0; chunk < b.perRun && !b.done; chunk++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := b.fillNextChunk(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (b *OriginBackfill) fillNextChunk(ctx context.Context) error {
	db := b.db.WithContext(ctx)
	var keys []entryKey
	if err := db.Raw(entriesAfterSQL, b.after.EntryID, b.after.EntryType, b.batch).Scan(&keys).Error; err != nil {
		return err
	}
	if len(keys) == 0 {
		b.done = true
		return nil
	}
	ids := make([]string, len(keys))
	types := make([]string, len(keys))
	for i, key := range keys {
		ids[i], types[i] = key.EntryID, key.EntryType
	}
	if err := db.Exec(recordOriginsSQL(unsettledEntriesSQL()), pq.Array(ids), pq.Array(types)).Error; err != nil {
		return err
	}
	b.after = keys[len(keys)-1]
	b.done = len(keys) < b.batch
	return nil
}
