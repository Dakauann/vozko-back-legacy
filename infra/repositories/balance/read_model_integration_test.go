package balance_repository

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vozko/domain/balance"
	"vozko/infra/database/schema"
	"vozko/infra/database/sqlexpr"
	"vozko/infra/repositories/repotest"
)

func seedBareWorkspace(t *testing.T, db *gorm.DB) string {
	t.Helper()
	owner := schema.User{ID: uuid.NewString(), Username: "owner-" + uuid.NewString()[:8], Email: uuid.NewString()[:8] + "@example.com", Password: "x"}
	ws := schema.Workspace{ID: uuid.NewString(), OwnerID: owner.ID, Name: "bare"}
	for _, row := range []interface{}{&owner, &ws} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return ws.ID
}

func storedTotalsCurrent(t *testing.T, db *gorm.DB, workspaceID string) bool {
	t.Helper()
	var current bool
	if err := db.Raw(`SELECT COALESCE(money_totals_at = updated_at, false) FROM balances WHERE workspace_id = ?`, workspaceID).Scan(&current).Error; err != nil {
		t.Fatalf("marker: %v", err)
	}
	return current
}

func assertSummaryIsTodays(t *testing.T, db *gorm.DB, repo balance.Repository, workspaceID string) {
	t.Helper()
	summary, err := repo.GetFullBalanceSummary(workspaceID)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	credits, debits := repotest.TodaysMoneyTotals(t, db, balanceIDOf(t, db, workspaceID))
	if summary.TotalMoneyCredits != credits || summary.TotalMoneyDebits != debits {
		t.Fatalf("summary credits %d debits %d, today's SQL says %d and %d", summary.TotalMoneyCredits, summary.TotalMoneyDebits, credits, debits)
	}
	var stored schema.Balance
	if err := db.Where("workspace_id = ?", workspaceID).First(&stored).Error; err != nil {
		t.Fatalf("balance: %v", err)
	}
	if summary.Balance == nil || summary.Balance.Amount != stored.Amount {
		t.Fatalf("summary balance %+v, stored amount %d", summary.Balance, stored.Amount)
	}
}

func TestRunningTotalsFollowEveryLedgerWrite(t *testing.T) {
	db := sendCapIntegrationDB(t)
	repo := NewRepository(db)
	ws := seedBareWorkspace(t, db)
	if _, err := repo.EnsureBalanceExists(ws, "usd"); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if !storedTotalsCurrent(t, db, ws) {
		t.Fatal("a new balance starts with current totals")
	}
	assertSummaryIsTodays(t, db, repo, ws)

	charge := uuid.NewString()
	writes := []func() error{
		func() error {
			_, err := repo.CreditBalance(balance.CreditBalanceInput{WorkspaceID: ws, Amount: 10_000_000, ServiceType: balance.ServiceTopUp, ExchangeRateMicros: 6_000_000})
			return err
		},
		func() error {
			_, err := repo.DebitBalance(balance.DebitBalanceInput{WorkspaceID: ws, Amount: 300_000, ServiceType: balance.ServiceWhatsAppCampaign, ReferenceID: &charge, OncePerReference: true, ExchangeRateMicros: 6_000_000})
			return err
		},
		func() error {
			refund := "refund:" + charge
			_, err := repo.CreditBalance(balance.CreditBalanceInput{WorkspaceID: ws, Amount: 300_000, ServiceType: balance.ServiceWhatsAppCampaign, ReferenceID: &refund, IsRefund: true, RefundOf: &charge, OncePerReference: true, ExchangeRateMicros: 6_000_000})
			return err
		},
		func() error {
			_, err := repo.DebitBalance(balance.DebitBalanceInput{WorkspaceID: ws, Amount: 1_234, ServiceType: balance.ServiceAI, ExchangeRateMicros: 6_000_000})
			return err
		},
		func() error {
			_, err := repo.DebitBalance(balance.DebitBalanceInput{WorkspaceID: ws, Amount: 50_000_000, ServiceType: balance.ServiceVoiceCall, AllowNegative: true, ExchangeRateMicros: 6_000_000})
			return err
		},
	}
	for i, write := range writes {
		if err := write(); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
		if !storedTotalsCurrent(t, db, ws) {
			t.Fatalf("write %d left the totals stale", i)
		}
		assertSummaryIsTodays(t, db, repo, ws)
	}
	if _, err := repo.DebitBalance(balance.DebitBalanceInput{WorkspaceID: ws, Amount: 1, ServiceType: balance.ServiceAI}); err == nil {
		t.Fatal("a refused debit must stay refused")
	}
	assertSummaryIsTodays(t, db, repo, ws)
}

func TestRunningTotalsAreNeverServedForABalanceWithoutThem(t *testing.T) {
	db := sendCapIntegrationDB(t)
	repo := NewRepository(db)
	ws := seedWorkspace(t, db, "legacy", 5_000_000)
	record(t, db, ws, "credit", "top-up", 5_000_000, 6_000_000, false)
	record(t, db, ws, "debit", "spent", 100, 6_000_000, false)

	if storedTotalsCurrent(t, db, ws) {
		t.Fatal("a balance created before the totals existed has none")
	}
	assertSummaryIsTodays(t, db, repo, ws)

	if _, err := repo.DebitBalance(balance.DebitBalanceInput{WorkspaceID: ws, Amount: 200, ServiceType: balance.ServiceAI, ExchangeRateMicros: 6_000_000}); err != nil {
		t.Fatalf("debit: %v", err)
	}
	if storedTotalsCurrent(t, db, ws) {
		t.Fatal("a write must never turn totals it did not count into current ones")
	}
	assertSummaryIsTodays(t, db, repo, ws)
}

func TestRunningTotalsStepAsideAfterAWriteThatSkippedThem(t *testing.T) {
	db := sendCapIntegrationDB(t)
	repo := NewRepository(db)
	ws := seedBareWorkspace(t, db)
	if _, err := repo.EnsureBalanceExists(ws, "usd"); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if _, err := repo.CreditBalance(balance.CreditBalanceInput{WorkspaceID: ws, Amount: 9_000, ServiceType: balance.ServiceTopUp, ExchangeRateMicros: 6_000_000}); err != nil {
		t.Fatalf("credit: %v", err)
	}
	record(t, db, ws, "debit", "older-binary", 400, 6_000_000, false)
	if err := db.Exec(`UPDATE balances SET amount = amount - 400, updated_at = NOW() WHERE workspace_id = ?`, ws).Error; err != nil {
		t.Fatalf("older binary write: %v", err)
	}
	if storedTotalsCurrent(t, db, ws) {
		t.Fatal("a write that skipped the totals must leave them stale")
	}
	assertSummaryIsTodays(t, db, repo, ws)

	if _, err := repo.CreditBalance(balance.CreditBalanceInput{WorkspaceID: ws, Amount: 1_000, ServiceType: balance.ServiceTopUp, ExchangeRateMicros: 6_000_000}); err != nil {
		t.Fatalf("credit: %v", err)
	}
	if storedTotalsCurrent(t, db, ws) {
		t.Fatal("stale totals stay stale until the backfill recounts them")
	}
	assertSummaryIsTodays(t, db, repo, ws)
}

func TestCampaignChargesCarryTheirCampaignFromTheWrite(t *testing.T) {
	f := newRoundFixture(t)
	ref := f.send(t)
	f.fail(t, ref)
	if _, err := f.ledger.DebitBalance(balance.DebitBalanceInput{WorkspaceID: f.workspace, Amount: 10, ServiceType: balance.ServiceAI, ReferenceID: &ref, ExchangeRateMicros: 6_000_000}); err != nil {
		t.Fatalf("ai debit: %v", err)
	}
	var rows []struct {
		ServiceType      string
		ReferenceID      string
		ChargeCampaignID *string
	}
	if err := f.db.Raw(`SELECT service_type, reference_id, charge_campaign_id::text AS charge_campaign_id FROM balance_transactions WHERE workspace_id = ?`, f.workspace).Scan(&rows).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	for _, row := range rows {
		if row.ServiceType != string(balance.ServiceWhatsAppCampaign) {
			if row.ChargeCampaignID != nil {
				t.Fatalf("a charge outside WhatsApp campaigns carries no campaign, got %v", *row.ChargeCampaignID)
			}
			continue
		}
		if row.ChargeCampaignID == nil || *row.ChargeCampaignID != f.campaign {
			t.Fatalf("%s carries %v, want campaign %s", row.ReferenceID, row.ChargeCampaignID, f.campaign)
		}
	}
}

type chargeWorld struct {
	t       *testing.T
	db      *gorm.DB
	repo    *BalanceRepositoryImpl
	ws      string
	deptA   string
	deptB   string
	early   time.Time
	middle  time.Time
	late    time.Time
	entries map[string]string
}

func (w *chargeWorld) campaign(workspaceID string, department *string, kind string) string {
	w.t.Helper()
	c := schema.WhatsAppCampaign{ID: uuid.NewString(), WorkspaceID: workspaceID, DepartmentID: department, Name: "c", Type: kind}
	if err := w.db.Create(&c).Error; err != nil {
		w.t.Fatalf("seed campaign: %v", err)
	}
	return c.ID
}

func (w *chargeWorld) entry(campaignID string) string {
	w.t.Helper()
	lead := schema.Lead{ID: uuid.NewString(), WorkspaceID: w.ws, Number: schema.OptionalText("55119" + uuid.NewString()[:8])}
	e := schema.WhatsAppCampaignEntry{ID: uuid.NewString(), CampaignID: campaignID, LeadID: lead.ID}
	for _, row := range []interface{}{&lead, &e} {
		if err := w.db.Create(row).Error; err != nil {
			w.t.Fatalf("seed entry: %v", err)
		}
	}
	return e.ID
}

func (w *chargeWorld) send(campaignID *string) string {
	w.t.Helper()
	s := schema.WhatsAppTemplateSend{ID: uuid.NewString(), WorkspaceID: w.ws, UserID: uuid.NewString(), IdempotencyKey: uuid.NewString(), BusinessPhoneID: uuid.NewString(), TemplateID: uuid.NewString(), CampaignID: campaignID}
	if err := w.db.Create(&s).Error; err != nil {
		w.t.Fatalf("seed send: %v", err)
	}
	return "waba:" + s.ID
}

func (w *chargeWorld) charge(reference, category string, at time.Time) {
	w.t.Helper()
	ref := reference
	tx, err := w.repo.DebitBalance(balance.DebitBalanceInput{WorkspaceID: w.ws, Amount: 1_000, ServiceType: balance.ServiceWhatsAppCampaign,
		ReferenceID: &ref, Description: "Template WhatsApp " + category + " (ref: " + reference + ")", OncePerReference: true, ExchangeRateMicros: 6_000_000})
	if err != nil {
		w.t.Fatalf("charge %s: %v", reference, err)
	}
	w.date(tx.ID, at)
}

func (w *chargeWorld) refund(reference, category string, at time.Time) {
	w.t.Helper()
	ref, of := "refund:"+reference, reference
	tx, err := w.repo.CreditBalance(balance.CreditBalanceInput{WorkspaceID: w.ws, Amount: 1_000, ServiceType: balance.ServiceWhatsAppCampaign,
		ReferenceID: &ref, Description: "Reembolso: template WhatsApp " + category + " não enviado (ref: " + reference + ")", IsRefund: true, RefundOf: &of, OncePerReference: true, ExchangeRateMicros: 6_000_000})
	if err != nil {
		w.t.Fatalf("refund %s: %v", reference, err)
	}
	w.date(tx.ID, at)
}

func (w *chargeWorld) legacy(txType, reference, category string, refund bool, at time.Time) {
	w.t.Helper()
	ref := reference
	row := schema.BalanceTransaction{ID: uuid.NewString(), BalanceID: balanceIDOf(w.t, w.db, w.ws), WorkspaceID: w.ws, Type: txType, Amount: 1_000,
		ResourceType: "money", ServiceType: string(balance.ServiceWhatsAppCampaign), ReferenceID: &ref, IsRefund: refund,
		Description: "Template WhatsApp " + category + " (ref: " + reference + ")", ExchangeRateMicros: 6_000_000}
	if err := w.db.Create(&row).Error; err != nil {
		w.t.Fatalf("legacy row: %v", err)
	}
	w.date(row.ID, at)
}

func (w *chargeWorld) date(id string, at time.Time) {
	w.t.Helper()
	if err := w.db.Model(&schema.BalanceTransaction{}).Where("id = ?", id).Update("created_at", at).Error; err != nil {
		w.t.Fatalf("date: %v", err)
	}
}

func (w *chargeWorld) softDelete(model interface{}, id string) {
	w.t.Helper()
	if err := w.db.Where("id = ?", id).Delete(model).Error; err != nil {
		w.t.Fatalf("soft delete: %v", err)
	}
}

func newChargeWorld(t *testing.T) *chargeWorld {
	t.Helper()
	db := sendCapIntegrationDB(t)
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	w := &chargeWorld{t: t, db: db, repo: &BalanceRepositoryImpl{db: db}, deptA: uuid.NewString(), deptB: uuid.NewString(),
		early: base, middle: base.Add(10 * 24 * time.Hour), late: base.Add(20 * 24 * time.Hour)}
	w.ws = seedWorkspace(t, db, "charges", 1_000_000_000)
	other := seedWorkspace(t, db, "other", 0)

	campaignA := w.campaign(w.ws, &w.deptA, "standard")
	campaignB := w.campaign(w.ws, &w.deptB, "recurring")
	campaignLoose := w.campaign(w.ws, nil, "standard")
	campaignGone := w.campaign(w.ws, &w.deptA, "standard")
	campaignForeign := w.campaign(other, &w.deptA, "standard")

	liveA, laterGoneA, goneA := w.entry(campaignA), w.entry(campaignA), w.entry(campaignA)
	w.softDelete(&schema.WhatsAppCampaignEntry{}, goneA)
	liveB, liveLoose, inGone, foreign := w.entry(campaignB), w.entry(campaignLoose), w.entry(campaignGone), w.entry(campaignForeign)
	sendA, sendGoneA, sendB, sendLoose := w.send(&campaignA), w.send(&campaignA), w.send(&campaignB), w.send(nil)
	w.softDelete(&schema.WhatsAppTemplateSend{}, sendGoneA[len("waba:"):])

	w.charge(liveA, "marketing", w.early)
	w.charge(liveA+":1", "marketing", w.middle)
	w.refund(liveA+":1", "marketing", w.late)
	w.charge(laterGoneA, "utility", w.middle)
	w.charge(goneA, "marketing", w.middle)
	w.charge(liveB, "authentication", w.middle)
	w.charge(liveB+":3", "utility", w.late)
	w.charge(liveLoose, "marketing", w.middle)
	w.charge(inGone, "marketing", w.middle)
	w.charge(foreign, "marketing", w.middle)
	w.charge(sendA, "utility", w.early)
	w.refund(sendA, "utility", w.middle)
	w.charge(sendGoneA, "marketing", w.middle)
	w.charge(sendB, "marketing", w.late)
	w.charge(sendLoose, "marketing", w.middle)
	w.charge(campaignA, "marketing", w.middle)
	w.charge(uuid.NewString(), "marketing", w.middle)
	w.charge("free text", "service", w.middle)

	w.legacy("debit", liveA+":7", "marketing", false, w.early)
	w.legacy("credit", "refund:"+liveA+":7", "marketing", true, w.middle)
	w.legacy("debit", liveB+":9", "authentication", false, w.late)
	w.legacy("debit", goneA+":2", "utility", false, w.middle)
	w.legacy("debit", "x"+liveA, "marketing", false, w.middle)

	w.softDelete(&schema.WhatsAppCampaignEntry{}, laterGoneA)
	w.softDelete(&schema.WhatsAppCampaign{}, campaignGone)
	return w
}

func (w *chargeWorld) filters() map[string]balance.WhatsAppChargeFilter {
	from, to := w.middle, w.middle.Add(time.Hour)
	unknown := uuid.NewString()
	return map[string]balance.WhatsAppChargeFilter{
		"department a":             {WorkspaceID: w.ws, DepartmentIDs: []string{w.deptA}},
		"departments a and b":      {WorkspaceID: w.ws, DepartmentIDs: []string{w.deptA, w.deptB}},
		"recurring type":           {WorkspaceID: w.ws, CampaignType: "recurring"},
		"standard in department a": {WorkspaceID: w.ws, CampaignType: "standard", DepartmentIDs: []string{w.deptA}},
		"standard type":            {WorkspaceID: w.ws, CampaignType: "standard"},
		"unknown department":       {WorkspaceID: w.ws, DepartmentIDs: []string{unknown}},
		"department a from middle": {WorkspaceID: w.ws, DepartmentIDs: []string{w.deptA}, From: &from},
		"department a window":      {WorkspaceID: w.ws, DepartmentIDs: []string{w.deptA}, From: &from, To: &to},
		"department a after late":  {WorkspaceID: w.ws, DepartmentIDs: []string{w.deptA}, From: &w.late},
		"whole workspace":          {WorkspaceID: w.ws},
	}
}

func netsOf(rows []templateChargeRow) map[string]int64 {
	out := map[string]int64{}
	for _, row := range rows {
		if row.Net != 0 {
			out[row.Category] += row.Net
		}
	}
	return out
}

func sameNets(a, b map[string]int64) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func (w *chargeWorld) todays(filter balance.WhatsAppChargeFilter) []templateChargeRow {
	w.t.Helper()
	rows, err := scanTemplateCharges(w.db, chargesByOwner(filter, filter.WorkspaceID))
	if err != nil {
		w.t.Fatalf("today's charges: %v", err)
	}
	return rows
}

func (w *chargeWorld) assertAggregateIsTodays(name string, filter balance.WhatsAppChargeFilter) {
	w.t.Helper()
	got, err := w.repo.AggregateWhatsAppTemplateCharges(filter)
	if err != nil {
		w.t.Fatalf("%s: aggregate: %v", name, err)
	}
	if want := foldTemplateCharges(w.todays(filter)); *got != *want {
		w.t.Fatalf("%s: aggregate %+v, today's SQL %+v", name, *got, *want)
	}
}

func TestCampaignChargesByCampaignEqualTodaysOwnerScan(t *testing.T) {
	w := newChargeWorld(t)

	for name, filter := range w.filters() {
		w.assertAggregateIsTodays(name, filter)
	}
	pending, err := scanTemplateCharges(w.db, chargesByCampaign(w.filters()["department a"], w.ws))
	if err != nil {
		t.Fatalf("by campaign: %v", err)
	}
	if !anyUnresolved(pending) {
		t.Fatal("rows written before the column existed must be reported as unresolved")
	}

	if err := w.db.Exec(`UPDATE balance_transactions bt SET charge_campaign_id = ` + sqlexpr.ChargeCampaignOf("bt.reference_id") +
		` WHERE bt.service_type = 'whatsapp_campaign' AND bt.charge_campaign_id IS NULL`).Error; err != nil {
		t.Fatalf("resolve legacy rows: %v", err)
	}

	nonEmpty := 0
	for name, filter := range w.filters() {
		w.assertAggregateIsTodays(name, filter)
		if len(filter.DepartmentIDs) == 0 && filter.CampaignType == "" {
			continue
		}
		fast, err := scanTemplateCharges(w.db, chargesByCampaign(filter, w.ws))
		if err != nil {
			t.Fatalf("%s: by campaign: %v", name, err)
		}
		if anyUnresolved(fast) {
			t.Fatalf("%s: every row is resolved now", name)
		}
		want := netsOf(w.todays(filter))
		if got := netsOf(fast); !sameNets(got, want) {
			t.Fatalf("%s: by campaign %v, today's owner scan %v", name, got, want)
		}
		if len(want) > 0 {
			nonEmpty++
		}
	}
	if nonEmpty < 5 {
		t.Fatalf("the fixture must exercise most filters, only %d had charges", nonEmpty)
	}
}
