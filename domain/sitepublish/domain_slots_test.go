package sitepublish

import (
	"errors"
	"testing"
	"time"

	"vozko/domain/dnsrecord"
)

func custom(id string, status HostnameStatus, created time.Time) HostnameRecord {
	return HostnameRecord{ID: id, Hostname: id + ".loja.com.br", Kind: HostnameCustom, Status: status, CreatedAt: created}
}

func TestEverySlotHolderButRemovingOnesUsesASlot(t *testing.T) {
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	records := []HostnameRecord{
		custom("c", HostnameActive, base.Add(3*time.Hour)),
		custom("a", HostnamePendingDNS, base.Add(time.Hour)),
		custom("gone", HostnameRemoving, base),
		custom("b", HostnameSuspended, base.Add(2*time.Hour)),
		{ID: "platform", Kind: HostnamePlatform, Status: HostnameActive, CreatedAt: base},
	}
	if got := SlotsInUse(records); got != 3 {
		t.Fatalf("slots in use = %d, want the 3 custom domains that are not being removed", got)
	}
	entitled := Entitled(records, 2)
	if !entitled["a"] || !entitled["b"] || entitled["c"] || entitled["gone"] || entitled["platform"] {
		t.Fatalf("with 2 slots the 2 oldest keep theirs and the newest goes offline first, got %v", entitled)
	}
	if len(Entitled(records, 0)) != 0 {
		t.Fatal("no slot means no domain is entitled")
	}
}

func TestOwnershipRecordIsATXTOnTheHostname(t *testing.T) {
	record := OwnershipRecord("www.loja.com.br", "abc123")
	if record.Type != "TXT" || record.Name != "_vz-domain.www.loja.com.br" || record.Value != "vz-domain-verification=abc123" {
		t.Fatalf("ownership record = %+v", record)
	}
	answers := map[string]dnsrecord.Answer{"_vz-domain.www.loja.com.br": {Values: []string{"google-site-verification=x", " vz-domain-verification=abc123 "}}}
	if got := ObserveOwnership(answers, "www.loja.com.br", "abc123"); got != dnsrecord.Verified {
		t.Fatalf("the token among other TXT values proves ownership, got %s", got)
	}
	if got := ObserveOwnership(answers, "www.loja.com.br", "other"); got != dnsrecord.Pending {
		t.Fatalf("another token proves nothing, got %s", got)
	}
	if got := ObserveOwnership(answers, "www.loja.com.br", ""); got != dnsrecord.Pending {
		t.Fatalf("an empty token never verifies, got %s", got)
	}
}

func TestSuspendedDomainsResumeOrLeave(t *testing.T) {
	for _, from := range []HostnameStatus{HostnamePendingDNS, HostnamePendingSSL, HostnameActive, HostnameFailed} {
		if err := CheckHostnameTransition(HostnameCustom, from, HostnameSuspended); err != nil {
			t.Errorf("%s must be able to go offline: %v", from, err)
		}
	}
	for _, to := range []HostnameStatus{HostnamePendingSSL, HostnamePendingDNS, HostnameRemoving} {
		if err := CheckHostnameTransition(HostnameCustom, HostnameSuspended, to); err != nil {
			t.Errorf("suspended must be able to move to %s: %v", to, err)
		}
	}
	if err := CheckHostnameTransition(HostnameCustom, HostnameSuspended, HostnameActive); !errors.Is(err, ErrHostnameTransition) {
		t.Error("a suspended domain must get a certificate again before it serves")
	}
	if !HostnameSuspended.Valid() {
		t.Error("suspended is a known status")
	}
}

func TestRecordsAreShownRelativeToTheRegistrableZone(t *testing.T) {
	cases := map[string]string{
		"www.clinicasorriso.com.br": "clinicasorriso.com.br",
		"loja.exemplo.com":          "exemplo.com",
		"exemplo.com":               "exemplo.com",
	}
	for host, zone := range cases {
		if got := DNSZone(host); got != zone {
			t.Errorf("DNSZone(%s) = %s, want %s", host, got, zone)
		}
	}
	if got := dnsrecord.RelativeHost(OwnershipName("www.clinicasorriso.com.br"), DNSZone("www.clinicasorriso.com.br")); got != "_vz-domain.www" {
		t.Fatalf("the TXT host typed at the DNS provider = %s", got)
	}
	if record := CNAMERecord("www.clinicasorriso.com.br", "cname.vozko.app"); record.Type != "CNAME" || record.Value != "cname.vozko.app" {
		t.Fatalf("cname = %+v", record)
	}
}
