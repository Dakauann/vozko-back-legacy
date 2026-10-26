package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"vozko/domain/conversation"
	"vozko/domain/shared"
	uw "vozko/domain/unofficial_whatsapp"
	"vozko/infra/database"
	conversation_repository "vozko/infra/repositories/conversation"
	workflow_repository "vozko/infra/repositories/workflow"
	workflow_usecase "vozko/usecases/workflow"
)

var shapers = map[shared.EntryType]conversation.InteractiveShaper{
	shared.EntryTypeUnofficialWhatsApp: uw.ShownInteractive,
}

func main() {
	channel := flag.String("channel", string(shared.EntryTypeUnofficialWhatsApp), "channel whose workflow menus are restored")
	page := flag.Int("page", 200, "sent menu steps read per round")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	db, err := database.NewGormDatabase()
	if err != nil {
		log.Fatalf("promptbackfill: database: %v", err)
	}
	db = db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Error)})
	repair, err := workflow_usecase.NewPromptTranscriptRepair(workflow_usecase.PromptTranscriptRepairDeps{
		Prompts:  workflow_repository.NewSentPromptRepository(db),
		Messages: conversation_repository.NewInteractiveBackfill(db),
		Shapers:  shapers,
	})
	if err != nil {
		log.Fatalf("promptbackfill: %v", err)
	}
	tally, err := repair.Run(ctx, shared.EntryType(*channel), *page)
	log.Printf("promptbackfill: %s: %d sent menus read, %d restored, %d already stored or not the workflow's row, %d unreadable",
		*channel, tally.Scanned, tally.Repaired, tally.Untouched, tally.Unreadable)
	if err != nil {
		log.Fatalf("promptbackfill: %v", err)
	}
}
