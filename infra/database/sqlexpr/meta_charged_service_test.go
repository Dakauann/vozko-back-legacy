package sqlexpr

import (
	"strings"
	"testing"

	"vozko/domain/conversation"
)

func TestTheChargedPredicateIsWhatMetaStampedBillable(t *testing.T) {
	charged := MetaChargedService("cm")
	for _, want := range []string{
		"cm.meta_pricing_billable IS TRUE",
		"cm.meta_pricing_category = '" + conversation.MetaPricingCategoryService + "'",
		"COALESCE(cm.meta_conversation_origin, '') <> '" + conversation.MetaOriginFreeEntryPoint + "'",
	} {
		if !strings.Contains(charged, want) {
			t.Errorf("charged predicate lacks %q:\n%s", want, charged)
		}
	}
}

func TestTheChargedPredicateContainsItsIndexedPart(t *testing.T) {
	if !strings.Contains(MetaChargedService("cm"), MetaBilledServiceIndexed("cm")) {
		t.Fatalf("the query predicate no longer contains the indexed part")
	}
	bare := MetaBilledServiceIndexed("")
	if strings.Contains(bare, "cm.") || !strings.Contains(bare, "meta_pricing_billable IS TRUE") {
		t.Fatalf("bare rendering = %q", bare)
	}
	if strings.Contains(bare, "meta_conversation_origin") {
		t.Fatalf("the origin is written by the status webhook and belongs in the payload, not the index predicate")
	}
}

func TestTheChargedPredicateBindsNothing(t *testing.T) {
	for _, predicate := range []string{MetaChargedService("cm"), MetaBilledServiceIndexed("")} {
		if strings.Contains(predicate, "?") || strings.Contains(predicate, "$1") {
			t.Errorf("predicate binds a parameter, which defeats the partial index:\n%s", predicate)
		}
	}
}
