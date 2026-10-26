package lead

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"vozko/domain/geo"
	"vozko/domain/leadmap"
	"vozko/domain/shared"
)

const explainLeads = 4000

func TestExplainTheSentimentCountsOfTheMapOnAModestSeedAgainstPostgres(t *testing.T) {
	db := mapDB(t)
	ws := uuid.NewString()
	sentiments := shared.SentimentValues()
	err := db.Exec(`
		WITH seeded AS (
			INSERT INTO leads (id, workspace_id, name, created_at, updated_at)
			SELECT gen_random_uuid(), ?, 'Lead ' || g, now(), now() FROM generate_series(1, ?) g
			RETURNING id
		), placed AS (
			INSERT INTO lead_addresses (id, workspace_id, lead_id, label, is_primary, position, state, latitude, longitude, geo_precision, geo_status, fingerprint, created_at, updated_at)
			SELECT gen_random_uuid(), ?, s.id, 'home', true, 0, 'SP', -23.6 + random() * 0.1, -46.7 + random() * 0.1, 'street', 'located', md5(s.id::text), now(), now()
			FROM seeded s
			RETURNING lead_id
		)
		INSERT INTO lead_sentiments (lead_id, workspace_id, sentiment, analysis_id, refreshed_at)
		SELECT p.lead_id, ?, (?::text[])[1 + floor(random() * 3)::int], gen_random_uuid(), now()
		FROM placed p WHERE random() < 0.5`, ws, explainLeads, ws, ws, pq.Array(sentiments)).Error
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO lead_sentiments (lead_id, workspace_id, sentiment, analysis_id, refreshed_at)
		SELECT gen_random_uuid(), ?, 'neutral', gen_random_uuid(), now() FROM generate_series(1, ?)`, uuid.NewString(), 3*explainLeads).Error; err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"leads", "lead_addresses", "lead_sentiments"} {
		if err := db.Exec("VACUUM ANALYZE " + table).Error; err != nil {
			t.Fatal(err)
		}
	}
	reader := NewMapReader(db).(*mapReader)
	m, err := reader.scope(leadmap.Scope{WorkspaceID: ws})
	if err != nil {
		t.Fatal(err)
	}
	w, err := geo.SnapWindow(geo.BBox{South: -23.6, West: -46.7, North: -23.5, East: -46.6}, 13)
	if err != nil {
		t.Fatal(err)
	}
	q := leadmap.NewLayerRequest(w, nil)
	cells, cellArgs := m.layerCells(q)
	points, pointArgs := m.layerPoints(q)
	for name, query := range map[string]struct {
		sql  string
		args []interface{}
	}{"cells": {cells, cellArgs}, "points": {points, pointArgs}} {
		var plan []string
		started := time.Now()
		if err := db.Raw("EXPLAIN (ANALYZE, BUFFERS) "+query.sql, query.args...).Scan(&plan).Error; err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		t.Logf("%s on %d leads (%s):\n%s", name, explainLeads, time.Since(started), strings.Join(plan, "\n"))
		if time.Since(started) > time.Second {
			t.Errorf("%s took %s on a modest seed", name, time.Since(started))
		}
	}
	var probed []string
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL enable_seqscan = off").Error; err != nil {
			return err
		}
		return tx.Raw("EXPLAIN (ANALYZE, BUFFERS) "+points, pointArgs...).Scan(&probed).Error
	})
	if err != nil {
		t.Fatal(err)
	}
	plan := strings.Join(probed, "\n")
	t.Logf("points without sequential scans:\n%s", plan)
	if !strings.Contains(plan, "idx_lead_sentiments_workspace") {
		t.Fatal("the sentiment join has no index path; a small tile of a large workspace would scan every sentiment of the workspace")
	}
}
