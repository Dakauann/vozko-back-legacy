package automationneeds_repository

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	domain "vozko/domain/automationneeds"
	"vozko/domain/conversation"
	"vozko/domain/conversationvars"
	"vozko/domain/shared"
	"vozko/infra/database/schema"
	"vozko/infra/testenv"
)

var errRollback = errors.New("rollback")

func inRolledBackTx(t *testing.T, run func(tx *gorm.DB)) {
	t.Helper()
	db, err := gorm.Open(postgres.Open(testenv.Postgres(t).DSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		schemaName := "automation_needs_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		for _, ddl := range []string{"CREATE SCHEMA " + schemaName, "SET LOCAL search_path TO " + schemaName} {
			if err := tx.Exec(ddl).Error; err != nil {
				t.Fatalf("fixture: %v", err)
			}
		}
		if err := tx.AutoMigrate(
			&schema.ConversationVariable{}, &schema.AutomationSwitchOff{},
			&schema.WhatsAppCampaign{}, &schema.WhatsAppCampaignEntry{}, &schema.WhatsAppBusinessPhoneNumber{},
			&schema.UnofficialWhatsAppCampaign{}, &schema.UnofficialWhatsAppCampaignEntry{}, &schema.UnofficialWhatsAppInstance{},
			&schema.InstagramAccount{}, &schema.FacebookPage{}, &schema.TelegramAccount{}, &schema.TikTokAccount{},
			&schema.MercadoLivreAccount{}, &schema.WebchatWidget{}, &schema.EmailInbox{}, &schema.RCSSender{}, &schema.SMSRoute{},
		); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		run(tx)
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("transaction: %v", err)
	}
}

func TestConversationVariablesAreUpsertedPerConversation(t *testing.T) {
	inRolledBackTx(t, func(tx *gorm.DB) {
		repo := NewConversationVariables(tx)
		ctx := context.Background()
		ref := conversationvars.Ref{WorkspaceID: "00000000-0000-0000-0000-0000000000a1", EntryID: "entry-1", EntryType: shared.EntryTypeInstagram}
		if err := repo.Save(ctx, ref, map[string]string{"turma": "A", "escola": "Prisma"}, "user-1"); err != nil {
			t.Fatal(err)
		}
		if err := repo.Save(ctx, ref, map[string]string{"turma": "B"}, "user-2"); err != nil {
			t.Fatal(err)
		}
		got, err := repo.Values(ctx, "entry-1", shared.EntryTypeInstagram)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, map[string]string{"turma": "B", "escola": "Prisma"}) {
			t.Fatalf("values = %v", got)
		}
		other, err := repo.Values(ctx, "entry-1", shared.EntryTypeTelegram)
		if err != nil || len(other) != 0 {
			t.Fatalf("another channel's conversation = %v, %v", other, err)
		}
	})
}

func TestCampaignValuesOfAChannelWithoutCampaignsAreEmpty(t *testing.T) {
	inRolledBackTx(t, func(tx *gorm.DB) {
		got, err := NewConversationVariables(tx).CampaignOf(context.Background(), "entry-1", shared.EntryTypeInstagram)
		if err != nil || got.Name != "" || len(got.Values) != 0 {
			t.Fatalf("campaign = %+v, %v", got, err)
		}
	})
}

func TestCampaignQueriesRunAgainstTheRealSchema(t *testing.T) {
	inRolledBackTx(t, func(tx *gorm.DB) {
		repo := NewConversationVariables(tx)
		for _, entryType := range []shared.EntryType{shared.EntryTypeWhatsApp, shared.EntryTypeUnofficialWhatsApp} {
			got, err := repo.CampaignOf(context.Background(), "00000000-0000-0000-0000-000000000000", entryType)
			if err != nil || got.Name != "" {
				t.Fatalf("%s: campaign = %+v, %v", entryType, got, err)
			}
		}
	})
}

func TestSlotQueriesRunAgainstTheRealSchema(t *testing.T) {
	inRolledBackTx(t, func(tx *gorm.DB) {
		empty := "00000000-0000-0000-0000-000000000000"
		slots, err := NewSlots(tx).InWorkspace(context.Background(), empty)
		if err != nil || len(slots) != 0 {
			t.Fatalf("slots = %+v, %v", slots, err)
		}
		var offs []domain.SwitchOff
		for _, kind := range domain.SlotKinds() {
			offs = append(offs, domain.SwitchOff{Slot: domain.Slot{Kind: kind, ID: empty}, Automation: domain.Link{Automation: conversation.Automation{Kind: conversation.AutomationAgent, ID: empty}}})
		}
		if _, err := SwitchOffIn(tx, empty, offs, "user", time.Now().UTC()); err != nil {
			t.Fatalf("switch off: %v", err)
		}
		open, err := NewSwitchOffRecords(tx).Open(context.Background(), empty, domain.SlotInstagram, empty)
		if err != nil || open == nil || open.Automation.ID != empty {
			t.Fatalf("open = %+v, %v", open, err)
		}
	})
}

func TestEnabledSlotsAreListedAndSwitchedOffWithTheirRecord(t *testing.T) {
	inRolledBackTx(t, func(tx *gorm.DB) {
		ws := uuid.NewString()
		agentID := uuid.NewString()
		phoneID := uuid.NewString()
		ig := schema.InstagramAccount{ID: uuid.NewString(), WorkspaceID: ws, IGUserID: "ig-1", Username: "loja", AgentID: &agentID, EnableAgentResponses: true}
		if err := tx.Create(&ig).Error; err != nil {
			t.Fatal(err)
		}
		off := schema.InstagramAccount{ID: uuid.NewString(), WorkspaceID: ws, IGUserID: "ig-2", Username: "parada", AgentID: &agentID}
		if err := tx.Create(&off).Error; err != nil {
			t.Fatal(err)
		}
		organic := schema.WhatsAppCampaign{ID: uuid.NewString(), WorkspaceID: ws, Name: "Receptivo", Type: "organic", BusinessPhoneID: &phoneID, AgentID: &agentID, EnableAgentResponses: true}
		if err := tx.Create(&organic).Error; err != nil {
			t.Fatal(err)
		}
		slots, err := NewSlots(tx).InWorkspace(context.Background(), ws)
		if err != nil {
			t.Fatal(err)
		}
		if len(slots) != 2 {
			t.Fatalf("slots = %+v, want the enabled account and the number", slots)
		}
		agent := domain.Link{Automation: conversation.Automation{Kind: conversation.AutomationAgent, ID: agentID}, Name: "Escola"}
		offs := []domain.SwitchOff{
			{Slot: domain.Slot{Kind: domain.SlotInstagram, ID: ig.ID, Name: "loja"}, Automation: agent, Needs: domain.Needs{{Name: "turma"}}},
			{Slot: domain.Slot{Kind: domain.SlotWhatsAppNumber, ID: phoneID, Name: "+55"}, Automation: agent, Needs: domain.Needs{{Name: "turma"}}},
		}
		containers, err := SwitchOffIn(tx, ws, offs, "user-1", time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		if len(containers) != 1 || containers[0] != organic.ID {
			t.Fatalf("containers = %v", containers)
		}
		after, err := NewSlots(tx).InWorkspace(context.Background(), ws)
		if err != nil || len(after) != 0 {
			t.Fatalf("after = %+v, %v", after, err)
		}
		record, err := NewSwitchOffRecords(tx).Open(context.Background(), ws, domain.SlotInstagram, ig.ID)
		if err != nil || record == nil || !reflect.DeepEqual(record.Variables, []string{"turma"}) || record.Automation.Name != "Escola" {
			t.Fatalf("record = %+v, %v", record, err)
		}
		if n, err := NewSwitchOffRecords(tx).Resolve(context.Background(), ws, domain.SlotInstagram, ig.ID, time.Now().UTC()); err != nil || n != 1 {
			t.Fatalf("resolve = %d, %v", n, err)
		}
	})
}

func TestHeldConversationsAreTheAutomationsOnThatChannelWithoutADelegation(t *testing.T) {
	inRolledBackTx(t, func(tx *gorm.DB) {
		if err := tx.AutoMigrate(&schema.InboxAssignment{}, &schema.ConversationDelegation{}); err != nil {
			t.Fatal(err)
		}
		ws, agentID, account := uuid.NewString(), uuid.NewString(), uuid.NewString()
		held, delegated, elsewhere := uuid.NewString(), uuid.NewString(), uuid.NewString()
		for _, row := range []schema.InboxAssignment{
			{ID: uuid.NewString(), WorkspaceID: ws, BusinessPhoneID: &account, EntryID: held, EntryType: "instagram", AssignedUserID: agentID, AssigneeKind: "ai"},
			{ID: uuid.NewString(), WorkspaceID: ws, BusinessPhoneID: &account, EntryID: delegated, EntryType: "instagram", AssignedUserID: agentID, AssigneeKind: "ai"},
			{ID: uuid.NewString(), WorkspaceID: ws, EntryID: elsewhere, EntryType: "instagram", AssignedUserID: agentID, AssigneeKind: "ai"},
		} {
			if err := tx.Create(&row).Error; err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Create(&schema.ConversationDelegation{EntryID: delegated, EntryType: "instagram", WorkspaceID: ws, AutomationKind: "agent", AutomationID: agentID, DelegatedBy: "u"}).Error; err != nil {
			t.Fatal(err)
		}
		got, err := NewHeld(tx).HeldOn(context.Background(), ws, conversation.Automation{Kind: conversation.AutomationAgent, ID: agentID}, account, 10)
		if err != nil || len(got) != 1 || got[0].EntryID != held {
			t.Fatalf("held = %+v, %v", got, err)
		}
	})
}

func TestStoredValuesComeWithWhoSetThem(t *testing.T) {
	inRolledBackTx(t, func(tx *gorm.DB) {
		repo := NewConversationVariables(tx)
		ctx := context.Background()
		ref := conversationvars.Ref{WorkspaceID: uuid.NewString(), EntryID: uuid.NewString(), EntryType: shared.EntryTypeWhatsApp}
		if err := repo.Save(ctx, ref, map[string]string{"turno": "Tarde", "matricula": "1"}, "u-7"); err != nil {
			t.Fatal(err)
		}
		rows, err := repo.Detailed(ctx, ref.EntryID, ref.EntryType)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 || rows[0].Name != "matricula" || rows[1].Value != "Tarde" || rows[1].SetBy != "u-7" || rows[1].SetAt.IsZero() {
			t.Fatalf("rows = %+v", rows)
		}
	})
}
