package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"vozko/infra/database"
	"vozko/infra/database/schema"
	"vozko/infra/repositories/leadsentiment"
)

func main() {
	only := flag.String("workspace", "", "refresh only this workspace")
	fromWorkspace := flag.String("from-workspace", "", "resume at this workspace (as printed by an interrupted run)")
	afterLead := flag.String("after-lead", "", "resume after this lead inside -from-workspace (as printed by an interrupted run)")
	chunk := flag.Int("chunk", 500, "leads refreshed per transaction")
	pause := flag.Duration("pause", 50*time.Millisecond, "pause between transactions")
	flag.Parse()
	if *chunk <= 0 || *chunk > leadsentiment.MaxLeadsPerRefresh {
		log.Fatalf("leadsentiments: -chunk must be between 1 and %d", leadsentiment.MaxLeadsPerRefresh)
	}
	if *afterLead != "" && *fromWorkspace == "" && *only == "" {
		log.Fatal("leadsentiments: -after-lead needs -from-workspace or -workspace")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	db, err := database.NewGormDatabase()
	if err != nil {
		log.Fatalf("leadsentiments: database: %v", err)
	}
	db = db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Error)})
	if !db.Migrator().HasTable(&schema.LeadSentiment{}) {
		log.Fatal("leadsentiments: lead_sentiments does not exist yet; start the server once so its migrations create it")
	}
	workspaces, err := workspacesFrom(ctx, db, *only, *fromWorkspace)
	if err != nil {
		log.Fatalf("leadsentiments: workspaces: %v", err)
	}
	total := 0
	for i, ws := range workspaces {
		after := ""
		if i == 0 {
			after = *afterLead
		}
		visited := 0
		for {
			next, n, err := leadsentiment.RefreshChunk(ctx, db, ws, after, *chunk)
			if err != nil {
				log.Fatalf("leadsentiments: stopped; resume with -from-workspace %s -after-lead %q: %v (%d leads refreshed so far)", ws, after, err, total)
			}
			visited += n
			total += n
			if next == "" {
				break
			}
			after = next
			select {
			case <-ctx.Done():
				log.Fatalf("leadsentiments: interrupted; resume with -from-workspace %s -after-lead %s (%d leads refreshed so far)", ws, after, total)
			case <-time.After(*pause):
			}
		}
		log.Printf("leadsentiments: workspace %s done, %d leads (%d/%d workspaces, %d leads in all)", ws, visited, i+1, len(workspaces), total)
	}
	log.Printf("leadsentiments: %d leads refreshed in %d workspaces", total, len(workspaces))
}

func workspacesFrom(ctx context.Context, db *gorm.DB, only, from string) ([]string, error) {
	if only != "" {
		return []string{only}, nil
	}
	var ids []string
	q := db.WithContext(ctx).Model(&schema.Workspace{}).Select("id::text")
	if from != "" {
		q = q.Where("id >= ?", from)
	}
	return ids, q.Order("id").Pluck("id", &ids).Error
}
