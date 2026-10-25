package database

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/infra/database/schema"
	"vozko/infra/database/sqlexpr"
	"vozko/infra/repositories/repotest"
)

func readModelTx(t *testing.T) *gorm.DB {
	t.Helper()
	tx := postgresTx(t)
	schemaName := "bl_read_model_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	for _, ddl := range []string{`CREATE SCHEMA ` + schemaName, `SET LOCAL search_path TO ` + schemaName} {
		if err := tx.Exec(ddl).Error; err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}
	if err := tx.AutoMigrate(&schema.User{}, &schema.Workspace{}, &schema.Balance{}, &schema.BalanceTransaction{},
		&schema.Lead{}, &schema.WhatsAppCampaign{}, &schema.WhatsAppCampaignEntry{}, &schema.WhatsAppTemplateSend{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return tx
}

type readModelSeed struct {
	tx *gorm.DB
	t  *testing.T
}

func (s readModelSeed) workspace() (workspaceID, balanceID string) {
	s.t.Helper()
	owner := schema.User{ID: uuid.NewString(), Username: "u-" + uuid.NewString()[:8], Email: uuid.NewString()[:8] + "@example.com", Password: "x"}
	ws := schema.Workspace{ID: uuid.NewString(), OwnerID: owner.ID, Name: "ws"}
	bal := schema.Balance{ID: uuid.NewString(), WorkspaceID: ws.ID, Currency: "USD"}
	for _, row := range []interface{}{&owner, &ws, &bal} {
		if err := s.tx.Create(row).Error; err != nil {
			s.t.Fatalf("seed: %v", err)
		}
	}
	return ws.ID, bal.ID
}

func (s readModelSeed) movement(workspaceID, balanceID, txType, resource, service string, amount int64, refund bool, reference *string) string {
	s.t.Helper()
	row := schema.BalanceTransaction{
		ID: uuid.NewString(), BalanceID: balanceID, WorkspaceID: workspaceID, Type: txType, Amount: amount,
		ResourceType: resource, ServiceType: service, ReferenceID: reference, IsRefund: refund,
	}
	if err := s.tx.Create(&row).Error; err != nil {
		s.t.Fatalf("seed movement: %v", err)
	}
	return row.ID
}

func (s readModelSeed) oldBinaryWrite(workspaceID, balanceID string, amount int64) {
	s.t.Helper()
	s.movement(workspaceID, balanceID, "credit", "money", "top_up", amount, false, nil)
	if err := s.tx.Exec(`UPDATE balances SET amount = amount + ?, updated_at = updated_at + interval '1 second' WHERE id = ?`, amount, balanceID).Error; err != nil {
		s.t.Fatalf("old binary write: %v", err)
	}
}

type storedTotals struct {
	TotalMoneyCredits int64
	TotalMoneyDebits  int64
	Current           bool
	UpdatedAt         time.Time
}

func storedMoneyTotals(t *testing.T, tx *gorm.DB, balanceID string) storedTotals {
	t.Helper()
	var out storedTotals
	if err := tx.Raw(`SELECT total_money_credits, total_money_debits, COALESCE(money_totals_at = updated_at, false) AS current, updated_at FROM balances WHERE id = ?`, balanceID).Scan(&out).Error; err != nil {
		t.Fatalf("stored totals: %v", err)
	}
	return out
}

func assertReconciled(t *testing.T, tx *gorm.DB, balanceID string) storedTotals {
	t.Helper()
	credits, debits := repotest.TodaysMoneyTotals(t, tx, balanceID)
	got := storedMoneyTotals(t, tx, balanceID)
	if !got.Current || got.TotalMoneyCredits != credits || got.TotalMoneyDebits != debits {
		t.Fatalf("balance %s stored %+v, today's SQL says credits %d debits %d", balanceID, got, credits, debits)
	}
	return got
}

func TestTheMoneyTotalsBackfillEqualsTodaysSumAndIsIdempotent(t *testing.T) {
	tx := readModelTx(t)
	seed := readModelSeed{tx: tx, t: t}
	busyWS, busy := seed.workspace()
	_, empty := seed.workspace()
	oddWS, odd := seed.workspace()

	seed.movement(busyWS, busy, "credit", "money", "top_up", 50_000_000, false, nil)
	seed.movement(busyWS, busy, "debit", "money", "whatsapp_campaign", 300_000, false, nil)
	seed.movement(busyWS, busy, "debit", "money", "ai", 1_234, false, nil)
	seed.movement(busyWS, busy, "credit", "money", "whatsapp_campaign", 300_000, true, nil)
	seed.movement(busyWS, busy, "debit", "messages", "whatsapp_campaign", 9_999, false, nil)
	seed.movement(busyWS, busy, "debit", "money", "manual_adjustment", 70, true, nil)
	seed.movement(oddWS, odd, "credit", "money", "top_up", 10, false, nil)
	seed.movement(oddWS, odd, "reversal", "money", "top_up", 3, false, nil)

	if err := backfillBalanceReadModel(tx); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	first := assertReconciled(t, tx, busy)
	assertReconciled(t, tx, empty)
	if storedMoneyTotals(t, tx, odd).Current {
		t.Fatal("a balance with a movement type today's sum cannot place must stay on the exact sum")
	}

	if err := backfillBalanceReadModel(tx); err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	again := assertReconciled(t, tx, busy)
	if again.TotalMoneyCredits != first.TotalMoneyCredits || again.TotalMoneyDebits != first.TotalMoneyDebits || !again.UpdatedAt.Equal(first.UpdatedAt) {
		t.Fatalf("a second run changed a reconciled balance: %+v then %+v", first, again)
	}
}

func TestTheMoneyTotalsBackfillRecountsABalanceAnOlderBinaryMoved(t *testing.T) {
	tx := readModelTx(t)
	seed := readModelSeed{tx: tx, t: t}
	ws, bal := seed.workspace()
	seed.movement(ws, bal, "credit", "money", "top_up", 1_000, false, nil)
	if err := backfillBalanceReadModel(tx); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	seed.oldBinaryWrite(ws, bal, 500)
	if storedMoneyTotals(t, tx, bal).Current {
		t.Fatal("a write that skipped the totals must leave them stale")
	}
	if err := backfillBalanceReadModel(tx); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if got := assertReconciled(t, tx, bal); got.TotalMoneyCredits != 1_500 {
		t.Fatalf("credits = %d, want 1500", got.TotalMoneyCredits)
	}
}

func TestTheChargeCampaignBackfillResolvesEveryCampaignChargeByItsOwner(t *testing.T) {
	tx := readModelTx(t)
	seed := readModelSeed{tx: tx, t: t}
	ws, bal := seed.workspace()

	campaign := schema.WhatsAppCampaign{ID: uuid.NewString(), WorkspaceID: ws, Name: "c"}
	lead := schema.Lead{ID: uuid.NewString(), WorkspaceID: ws, Number: "5511999990001"}
	otherLead := schema.Lead{ID: uuid.NewString(), WorkspaceID: ws, Number: "5511999990002"}
	live := schema.WhatsAppCampaignEntry{ID: uuid.NewString(), CampaignID: campaign.ID, LeadID: lead.ID}
	gone := schema.WhatsAppCampaignEntry{ID: uuid.NewString(), CampaignID: campaign.ID, LeadID: otherLead.ID}
	campaignID := campaign.ID
	send := schema.WhatsAppTemplateSend{ID: uuid.NewString(), WorkspaceID: ws, UserID: uuid.NewString(), IdempotencyKey: "a", BusinessPhoneID: uuid.NewString(), TemplateID: uuid.NewString(), CampaignID: &campaignID}
	loose := schema.WhatsAppTemplateSend{ID: uuid.NewString(), WorkspaceID: ws, UserID: uuid.NewString(), IdempotencyKey: "b", BusinessPhoneID: uuid.NewString(), TemplateID: uuid.NewString()}
	for _, row := range []interface{}{&campaign, &lead, &otherLead, &live, &gone, &send, &loose} {
		if err := tx.Create(row).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	if err := tx.Delete(&gone).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	ref := func(s string) *string { return &s }
	want := map[string]string{}
	charge := func(txType string, refund bool, reference *string, campaign string) {
		id := seed.movement(ws, bal, txType, "money", "whatsapp_campaign", 1, refund, reference)
		want[id] = campaign
	}
	charge("debit", false, ref(live.ID), campaign.ID)
	charge("debit", false, ref(live.ID+":2"), campaign.ID)
	charge("credit", true, ref("refund:"+live.ID+":2"), campaign.ID)
	charge("debit", false, ref(gone.ID), campaign.ID)
	charge("debit", false, ref("waba:"+send.ID), campaign.ID)
	charge("credit", true, ref("refund:waba:"+send.ID), campaign.ID)
	charge("debit", false, ref(campaign.ID), campaign.ID)
	charge("debit", false, ref("waba:"+loose.ID), sqlexpr.NoChargeCampaign)
	charge("debit", false, ref(strings.ToUpper(live.ID)), sqlexpr.NoChargeCampaign)
	charge("debit", false, ref("not-an-owner"), sqlexpr.NoChargeCampaign)
	charge("debit", false, nil, sqlexpr.NoChargeCampaign)
	untouched := seed.movement(ws, bal, "debit", "money", "ai", 1, false, ref(live.ID))

	if err := backfillChargeCampaigns(tx, chargeCampaignBackfill{batch: 2, perRun: 100}); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	for id, campaign := range want {
		var got *string
		if err := tx.Raw(`SELECT charge_campaign_id::text FROM balance_transactions WHERE id = ?`, id).Scan(&got).Error; err != nil {
			t.Fatalf("read: %v", err)
		}
		if got == nil || *got != campaign {
			t.Fatalf("row %s resolved to %v, want %s", id, got, campaign)
		}
	}
	var other *string
	if err := tx.Raw(`SELECT charge_campaign_id::text FROM balance_transactions WHERE id = ?`, untouched).Scan(&other).Error; err != nil || other != nil {
		t.Fatalf("a charge outside WhatsApp campaigns must stay unresolved, got %v %v", other, err)
	}

	if err := tx.Exec(`UPDATE balance_transactions SET charge_campaign_id = ? WHERE id <> ? AND service_type = 'whatsapp_campaign'`, sqlexpr.NoChargeCampaign, untouched).Error; err != nil {
		t.Fatalf("pin: %v", err)
	}
	if err := backfillChargeCampaigns(tx, chargeCampaignBackfill{batch: 2, perRun: 100}); err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	var rewritten int64
	if err := tx.Raw(`SELECT COUNT(*) FROM balance_transactions WHERE service_type = 'whatsapp_campaign' AND charge_campaign_id <> ?`, sqlexpr.NoChargeCampaign).Scan(&rewritten).Error; err != nil || rewritten != 0 {
		t.Fatalf("a second run must leave resolved rows alone, rewrote %d (%v)", rewritten, err)
	}
}

func TestTheBalanceReadModelBackfillRunsAtBoot(t *testing.T) {
	for _, name := range repairNames() {
		if name == "bl_balance_read_model" {
			return
		}
	}
	t.Fatal("the balance read model backfill must run at boot")
}

func TestTheChargeCampaignBackfillStopsAtItsShareForOneBoot(t *testing.T) {
	tx := readModelTx(t)
	seed := readModelSeed{tx: tx, t: t}
	ws, bal := seed.workspace()
	for i := 0; i < 7; i++ {
		reference := uuid.NewString()
		seed.movement(ws, bal, "debit", "money", "whatsapp_campaign", 1, false, &reference)
	}
	unresolved := func() int64 {
		var n int64
		if err := tx.Raw(`SELECT COUNT(*) FROM balance_transactions WHERE charge_campaign_id IS NULL`).Scan(&n).Error; err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}
	share := chargeCampaignBackfill{batch: 2, perRun: 3}
	for _, want := range []int64{4, 1, 0} {
		if err := backfillChargeCampaigns(tx, share); err != nil {
			t.Fatalf("backfill: %v", err)
		}
		if got := unresolved(); got != want {
			t.Fatalf("unresolved after a boot = %d, want %d", got, want)
		}
	}
}
