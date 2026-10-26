package lead

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/domain/crmfilter"
	"vozko/domain/geo"
	"vozko/domain/leadmap"
	"vozko/domain/shared"
)

func feel(t *testing.T, db *gorm.DB, ws, leadID, sentiment string) {
	t.Helper()
	if err := db.Exec("INSERT INTO lead_sentiments (lead_id, workspace_id, sentiment, analysis_id, refreshed_at) VALUES (?, ?, ?, ?, ?)",
		leadID, ws, sentiment, uuid.NewString(), time.Now()).Error; err != nil {
		t.Fatal(err)
	}
}

func TestTheMapCountsTheSentimentOfTheLeadsAtEachPositionAgainstPostgres(t *testing.T) {
	db := mapDB(t)
	ws := uuid.NewString()
	happy := seedLead(t, db, ws, paulistaAddress("street"))
	angry := seedLead(t, db, ws, paulistaAddress("street"))
	alsoAngry := seedLead(t, db, ws, paulistaAddress("street"))
	unknown := seedLead(t, db, ws, paulistaAddress("street"))
	silent := seedLead(t, db, ws, paulistaAddress("street"))
	feel(t, db, ws, happy, "positive")
	feel(t, db, ws, angry, "negative")
	feel(t, db, ws, alsoAngry, "negative")
	feel(t, db, ws, unknown, "angry")
	feel(t, db, uuid.NewString(), silent, "positive")

	reader := NewMapReader(db)
	ctx := context.Background()
	w, err := geo.SnapWindow(geo.BBox{South: -23.6, West: -46.7, North: -23.5, East: -46.6}, 13)
	if err != nil {
		t.Fatal(err)
	}
	layer := layerIn(t, db, ws, crmfilter.Filter{}, w)
	if layer.Kind != leadmap.LayerPoints || len(layer.Points) != 1 {
		t.Fatalf("layer = %+v", layer)
	}
	p := layer.Points[0]
	if p.People != 5 || p.Sentiment != (leadmap.SentimentCounts{Positive: 1, Negative: 2}) || p.Mood != shared.SentimentNegative {
		t.Fatalf("point = %+v, want 5 people, one happy, two angry and an angry mood; the unknown and the foreign verdicts count nowhere", p)
	}

	cells, err := reader.Layer(ctx, leadmap.Scope{WorkspaceID: ws}, leadmap.LayerRequest{Claim: w.Claim(), MaxPoints: 0, CellSize: w.CellSize(geo.MaxCellsPerAxis)})
	if err != nil || cells.Kind != leadmap.LayerCells {
		t.Fatalf("cells = %+v %v", cells, err)
	}
	var summed leadmap.SentimentCounts
	for _, c := range cells.Cells {
		summed.Positive += c.Sentiment.Positive
		summed.Neutral += c.Sentiment.Neutral
		summed.Negative += c.Sentiment.Negative
	}
	if summed != (leadmap.SentimentCounts{Positive: 1, Negative: 2}) {
		t.Fatalf("cells sum %+v", summed)
	}

	peek, total, err := reader.LeadsAt(ctx, leadmap.Scope{WorkspaceID: ws}, geo.Point{Lat: -23.5614, Lng: -46.6559}, crmfilter.PlacementOnMap, leadmap.MaxPeekLeads)
	if err != nil || total != 5 {
		t.Fatalf("peek = %v %d %v", peek, total, err)
	}
	marks := map[string]shared.Sentiment{}
	for _, l := range peek {
		marks[l.ID] = l.Sentiment
	}
	want := map[string]shared.Sentiment{happy: shared.SentimentPositive, angry: shared.SentimentNegative, alsoAngry: shared.SentimentNegative, unknown: "", silent: ""}
	for id, mark := range want {
		if got, ok := marks[id]; !ok || got != mark {
			t.Fatalf("peek mark of %s = %q (listed %v), want %q", id[:8], got, ok, mark)
		}
	}
}
