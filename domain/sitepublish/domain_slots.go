package sitepublish

import (
	"sort"
	"strings"
	"time"

	"golang.org/x/net/publicsuffix"

	"vozko/domain/dnsrecord"
)

const (
	ownershipLabel  = "_vz-domain."
	ownershipPrefix = "vz-domain-verification="

	MaxCustomDomainAddsPerDay = 10
	CustomDomainAddWindow     = 24 * time.Hour
)

type OfflineReason string

const (
	OfflineBilling OfflineReason = "billing"
	OfflineSlots   OfflineReason = "slots"
)

func DNSZone(hostname string) string {
	if zone, err := publicsuffix.EffectiveTLDPlusOne(hostname); err == nil {
		return zone
	}
	return hostname
}

func CNAMERecord(hostname, target string) DNSRecord {
	return DNSRecord{Type: "CNAME", Name: hostname, Value: target}
}

func OwnershipName(hostname string) string {
	return ownershipLabel + hostname
}

func OwnershipRecord(hostname, token string) DNSRecord {
	return DNSRecord{Type: "TXT", Name: OwnershipName(hostname), Value: ownershipPrefix + token}
}

func ObserveOwnership(answers map[string]dnsrecord.Answer, hostname, token string) dnsrecord.Status {
	want := ownershipPrefix + token
	return dnsrecord.Observe(answers, OwnershipName(hostname), func(v string) bool {
		return token != "" && strings.TrimSpace(v) == want
	})
}

func holdsSlot(rec HostnameRecord) bool {
	return rec.Kind == HostnameCustom && rec.Status != HostnameRemoving
}

func SlotHolders(records []HostnameRecord) []HostnameRecord {
	out := make([]HostnameRecord, 0, len(records))
	for _, rec := range records {
		if holdsSlot(rec) {
			out = append(out, rec)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func SlotsInUse(records []HostnameRecord) int {
	return len(SlotHolders(records))
}

func Entitled(records []HostnameRecord, allowed int) map[string]bool {
	out := map[string]bool{}
	for i, rec := range SlotHolders(records) {
		if i >= allowed {
			break
		}
		out[rec.ID] = true
	}
	return out
}
