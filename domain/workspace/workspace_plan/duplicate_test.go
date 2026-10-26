package workspace_plan

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestDuplicateNameSuffixesTheSourceNameWithTheFirstFreeCopyNumber(t *testing.T) {
	cases := []struct {
		name   string
		source string
		taken  []string
		want   string
	}{
		{"first copy", "Pro", []string{"Pro"}, "Pro (cópia)"},
		{"first copy taken", "Pro", []string{"Pro", "Pro (cópia)"}, "Pro (cópia 2)"},
		{"several copies taken", "Pro", []string{"Pro (cópia)", "Pro (cópia 2)", "Pro (cópia 3)"}, "Pro (cópia 4)"},
		{"a gap is reused", "Pro", []string{"Pro (cópia)", "Pro (cópia 3)"}, "Pro (cópia 2)"},
		{"taken names compare without case", "Pro", []string{"PRO (CÓPIA)"}, "Pro (cópia 2)"},
		{"taken names compare trimmed", "Pro", []string{"  Pro (cópia)  "}, "Pro (cópia 2)"},
		{"source name is trimmed", "  Pro  ", nil, "Pro (cópia)"},
		{"copying a copy numbers from the original", "Pro (cópia)", []string{"Pro", "Pro (cópia)"}, "Pro (cópia 2)"},
		{"copying a numbered copy numbers from the original", "Pro (cópia 7)", []string{"Pro", "Pro (cópia 7)"}, "Pro (cópia)"},
		{"a name that only looks like a suffix stays whole", "(cópia)", nil, "(cópia) (cópia)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DuplicateName(tc.source, tc.taken)
			if err != nil {
				t.Fatalf("DuplicateName: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDuplicateNameFitsThePlanNameColumnWithoutSplittingCharacters(t *testing.T) {
	source := strings.Repeat("é", PlanNameMaxRunes)
	for _, taken := range [][]string{nil, {strings.Repeat("é", PlanNameMaxRunes-len([]rune(" (cópia)"))) + " (cópia)"}} {
		got, err := DuplicateName(source, taken)
		if err != nil {
			t.Fatalf("DuplicateName: %v", err)
		}
		if n := utf8.RuneCountInString(got); n > PlanNameMaxRunes {
			t.Fatalf("name has %d characters, the column holds %d", n, PlanNameMaxRunes)
		}
		if !utf8.ValidString(got) || !strings.Contains(got, "(cópia") {
			t.Fatalf("name %q is not a readable copy name", got)
		}
	}
}

func TestDuplicateNameRefusesWhenEveryCopyNameIsTaken(t *testing.T) {
	taken := []string{"Pro (cópia)"}
	for n := 2; n <= DuplicateNameAttempts; n++ {
		taken = append(taken, fmt.Sprintf("Pro (cópia %d)", n))
	}
	if _, err := DuplicateName("Pro", taken); !errors.Is(err, ErrDuplicateNameUnavailable) {
		t.Fatalf("err = %v, want ErrDuplicateNameUnavailable", err)
	}
}

func TestDuplicateNameRefusesABlankSourceName(t *testing.T) {
	if _, err := DuplicateName("   ", nil); !errors.Is(err, ErrInvalidPlanName) {
		t.Fatalf("err = %v, want ErrInvalidPlanName", err)
	}
}

func sourcePlanWithEverythingSet() *PlanDefinition {
	affiliate := "aff-1"
	archived := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	stamp := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	return &PlanDefinition{
		ID:                             "source",
		Name:                           "Pro",
		Description:                    "Tudo incluso",
		BasePriceBRLCents:              49900,
		MaxCallChannels:                4,
		IncludedWhatsAppBusinessPhones: 2,
		MaxBranches:                    3,
		IsGloballyVisible:              true,
		ExclusiveAffiliateID:           &affiliate,
		AllowedWorkspaceIDs:            []string{"ws-1"},
		AllowedWorkspaces:              []AllowedWorkspace{{ID: "ws-1", Name: "Loja"}},
		ArchivedAt:                     &archived,
		CreatedAt:                      stamp,
		UpdatedAt:                      stamp,
		PricingItems: []PlanPricingItem{
			{ID: "item-1", PlanDefinitionID: "source", Category: "whatsapp", Service: "marketing", Metric: "per_message", CostMicros: 11, PriceMicros: 22, MarkupPct: 1.5, Currency: "USD", CreatedAt: stamp, UpdatedAt: stamp},
			{ID: "item-2", PlanDefinitionID: "source", Category: "telephony", Service: "landline", Metric: "per_minute", CostMicros: 33, PriceMicros: 44, MarkupPct: 0, Currency: "BRL", CreatedAt: stamp, UpdatedAt: stamp},
		},
	}
}

func TestDuplicateCopiesEverySettingAndEveryPricingItemPriceAndCost(t *testing.T) {
	source := sourcePlanWithEverythingSet()
	copied := source.Duplicate("copy")

	if copied.ID != "copy" || copied.Name != source.Name || copied.Description != source.Description ||
		copied.BasePriceBRLCents != source.BasePriceBRLCents || copied.MaxCallChannels != source.MaxCallChannels ||
		copied.IncludedWhatsAppBusinessPhones != source.IncludedWhatsAppBusinessPhones || copied.MaxBranches != source.MaxBranches {
		t.Fatalf("settings were not copied: %+v", copied)
	}
	if len(copied.PricingItems) != len(source.PricingItems) {
		t.Fatalf("copied %d pricing items, want %d", len(copied.PricingItems), len(source.PricingItems))
	}
	for i, item := range copied.PricingItems {
		original := source.PricingItems[i]
		if item.Category != original.Category || item.Service != original.Service || item.Metric != original.Metric ||
			item.CostMicros != original.CostMicros || item.PriceMicros != original.PriceMicros ||
			item.MarkupPct != original.MarkupPct || item.Currency != original.Currency {
			t.Fatalf("item %d = %+v, want the values of %+v", i, item, original)
		}
		if item.ID != "" || item.PlanDefinitionID != "copy" || !item.CreatedAt.IsZero() || !item.UpdatedAt.IsZero() {
			t.Fatalf("item %d keeps the source identity: %+v", i, item)
		}
	}
	copied.PricingItems[0].PriceMicros = 999
	if source.PricingItems[0].PriceMicros != 22 {
		t.Fatal("changing the copy changed the source plan")
	}
}

func TestDuplicateIsPrivateUnarchivedAndCarriesNoIdentityOrAudience(t *testing.T) {
	copied := sourcePlanWithEverythingSet().Duplicate("copy")
	if copied.IsGloballyVisible {
		t.Fatal("the copy is public")
	}
	if copied.ExclusiveAffiliateID != nil || copied.AllowedWorkspaceIDs != nil || copied.AllowedWorkspaces != nil {
		t.Fatalf("the copy reaches an audience: %+v", copied)
	}
	if copied.ArchivedAt != nil || !copied.CreatedAt.IsZero() || !copied.UpdatedAt.IsZero() {
		t.Fatalf("the copy keeps source lifecycle stamps: %+v", copied)
	}
	if copied.IsVisibleTo("ws-1") || copied.IsVisibleToAffiliate("aff-1") {
		t.Fatal("the copy is visible to the source audience")
	}
}

func TestDuplicateDecidesForEveryPlanField(t *testing.T) {
	decided := map[string]bool{
		"ID": true, "Name": true, "Description": true, "BasePriceBRLCents": true, "MaxCallChannels": true,
		"IncludedWhatsAppBusinessPhones": true, "MaxBranches": true, "IsGloballyVisible": true,
		"ExclusiveAffiliateID": true, "PricingItems": true, "AllowedWorkspaceIDs": true, "AllowedWorkspaces": true,
		"ArchivedAt": true, "CreatedAt": true, "UpdatedAt": true,
	}
	fields := reflect.TypeOf(PlanDefinition{})
	for i := 0; i < fields.NumField(); i++ {
		if name := fields.Field(i).Name; !decided[name] {
			t.Errorf("PlanDefinition.%s is new: decide in Duplicate whether a copy keeps it, then list it here", name)
		}
	}
	itemDecided := map[string]bool{
		"ID": true, "PlanDefinitionID": true, "Category": true, "Service": true, "Metric": true, "CostMicros": true,
		"PriceMicros": true, "MarkupPct": true, "Currency": true, "CreatedAt": true, "UpdatedAt": true,
	}
	items := reflect.TypeOf(PlanPricingItem{})
	for i := 0; i < items.NumField(); i++ {
		if name := items.Field(i).Name; !itemDecided[name] {
			t.Errorf("PlanPricingItem.%s is new: decide in Duplicate whether a copy keeps it, then list it here", name)
		}
	}
}

func TestDuplicatePlanInputNeedsASourceARequesterAndABoundedKey(t *testing.T) {
	valid := DuplicatePlanInput{SourcePlanID: "p1", RequestedBy: "u1", IdempotencyKey: "k1"}
	cases := []struct {
		name  string
		input DuplicatePlanInput
		want  error
	}{
		{"valid", valid, nil},
		{"no source", DuplicatePlanInput{RequestedBy: "u1", IdempotencyKey: "k1"}, ErrPlanNotFound},
		{"no requester", DuplicatePlanInput{SourcePlanID: "p1", IdempotencyKey: "k1"}, ErrDuplicateRequesterRequired},
		{"no key", DuplicatePlanInput{SourcePlanID: "p1", RequestedBy: "u1"}, ErrDuplicateKeyRequired},
		{"blank key", DuplicatePlanInput{SourcePlanID: "p1", RequestedBy: "u1", IdempotencyKey: "   "}, ErrDuplicateKeyRequired},
		{"key too long", DuplicatePlanInput{SourcePlanID: "p1", RequestedBy: "u1", IdempotencyKey: strings.Repeat("k", DuplicateKeyMaxRunes+1)}, ErrDuplicateKeyRequired},
		{"key at the limit", DuplicatePlanInput{SourcePlanID: "p1", RequestedBy: "u1", IdempotencyKey: strings.Repeat("ç", DuplicateKeyMaxRunes)}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.input.Validate(); !errors.Is(err, tc.want) || (tc.want == nil && err != nil) {
				t.Fatalf("Validate() = %v, want %v", err, tc.want)
			}
		})
	}
}
