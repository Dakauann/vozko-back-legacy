package repotest

import (
	"fmt"
	"math/rand/v2"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/infra/database/schema"
)

func LeadSentimentModels() []any {
	return append(LeadStoreModels(), &schema.LeadSentiment{}, &schema.AudienceAnalysis{})
}

func MigrateLeadSentiments(t testing.TB, db *gorm.DB) {
	t.Helper()
	db.Config.DisableForeignKeyConstraintWhenMigrating = true
	if err := db.AutoMigrate(LeadSentimentModels()...); err != nil {
		t.Fatalf("migrate lead sentiments: %v", err)
	}
}

func InsertRow(t testing.TB, db *gorm.DB, table string, values map[string]interface{}) {
	t.Helper()
	var required []struct{ Name, DataType string }
	if err := db.Raw(`SELECT column_name AS name, data_type FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = ? AND is_nullable = 'NO' AND column_default IS NULL`, table).Scan(&required).Error; err != nil {
		t.Fatal(err)
	}
	row := map[string]interface{}{}
	for _, c := range required {
		row[c.Name] = fillerOf(c.DataType)
	}
	for k, v := range values {
		row[k] = v
	}
	names := make([]string, 0, len(row))
	for k := range row {
		names = append(names, k)
	}
	sort.Strings(names)
	args := make([]interface{}, 0, len(names))
	for _, n := range names {
		args = append(args, row[n])
	}
	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, strings.Join(names, ", "), strings.TrimSuffix(strings.Repeat("?, ", len(names)), ", "))
	if err := db.Exec(sql, args...).Error; err != nil {
		t.Fatalf("insert into %s: %v", table, err)
	}
}

func fillerOf(dataType string) interface{} {
	switch dataType {
	case "uuid":
		return uuid.NewString()
	case "integer", "bigint", "smallint", "numeric", "double precision", "real":
		return rand.IntN(1 << 30)
	case "boolean":
		return false
	case "timestamp with time zone", "timestamp without time zone", "date":
		return time.Now()
	case "jsonb", "json", "ARRAY":
		return "{}"
	}
	return "f" + uuid.NewString()[:12]
}
