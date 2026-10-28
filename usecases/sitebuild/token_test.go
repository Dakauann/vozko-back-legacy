package sitebuild

import (
	"strings"
	"testing"
)

func TestTheGrantMACDomainIsANeutralVersionedLabel(t *testing.T) {
	if macDomain != "site-preview-grant-v1\x00" || strings.Contains(strings.ToLower(macDomain), "vozko") {
		t.Fatalf("mac domain %q", macDomain)
	}
}
