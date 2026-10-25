package sqlexpr

import (
	"strings"
	"testing"
)

func TestChargeOwnerKeyKeepsTheLedgerOwnerRule(t *testing.T) {
	want := `CASE WHEN regexp_replace(bt.reference_id, '^refund:', '') LIKE 'waba:%' THEN regexp_replace(bt.reference_id, '^refund:', '') ELSE split_part(regexp_replace(bt.reference_id, '^refund:', ''), ':', 1) END`
	if got := ChargeOwnerKey("bt.reference_id"); got != want {
		t.Fatalf("ChargeOwnerKey = %q", got)
	}
}

func TestChargeCampaignOfReadsTheReferenceOnceAndBindsNothing(t *testing.T) {
	got := ChargeCampaignOf("r.reference_id")
	if strings.Count(got, "r.reference_id") != strings.Count(ChargeOwnerKey("r.reference_id"), "r.reference_id") {
		t.Fatalf("the reference must only feed the owner key once: %s", got)
	}
	if strings.Contains(got, "?") || strings.Contains(got, "$") {
		t.Fatalf("the expression must not carry placeholders: %s", got)
	}
	if !strings.Contains(got, "'"+NoChargeCampaign+"'::uuid") {
		t.Fatalf("an owner without a campaign must resolve to the no-campaign marker: %s", got)
	}
}
