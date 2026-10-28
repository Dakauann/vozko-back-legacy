package emailsending

import "vozko/domain/dnsrecord"

type RecordStatus = dnsrecord.Status

const (
	RecordVerified = dnsrecord.Verified
	RecordPending  = dnsrecord.Pending
	RecordUnknown  = dnsrecord.Unknown
)

func (r DNSRecord) CurrentStatus() RecordStatus {
	if r.Status.Valid() {
		return r.Status
	}
	return RecordPending
}

type DNSAnswer = dnsrecord.Answer

type DNSAnswers struct {
	MX  map[string]DNSAnswer
	TXT map[string]DNSAnswer
}

func RecordLookups(records []DNSRecord) (mx, txt []string) {
	for _, r := range records {
		switch r.Purpose {
		case RecordReplyMX:
			mx = append(mx, hostKey(r.Name))
		case RecordDMARC:
			txt = append(txt, hostKey(r.Name))
		}
	}
	return mx, txt
}

func CheckRecords(records []DNSRecord, state IdentityState, answers DNSAnswers) []DNSRecord {
	out := make([]DNSRecord, len(records))
	for i, r := range records {
		r.Status = recordStatus(r, state, answers)
		out[i] = r
	}
	return out
}

func recordStatus(r DNSRecord, state IdentityState, answers DNSAnswers) RecordStatus {
	switch r.Purpose {
	case RecordDKIM:
		return verifiedWhen(state.DKIMVerified)
	case RecordMailFromMX, RecordMailFromSPF:
		return verifiedWhen(state.MailFromVerified)
	case RecordReplyMX:
		want := hostKey(r.Value)
		return observed(answers.MX, r.Name, func(v string) bool { return hostKey(v) == want })
	case RecordDMARC:
		return observed(answers.TXT, r.Name, DMARCCompatible)
	}
	return RecordUnknown
}

func verifiedWhen(ok bool) RecordStatus {
	if ok {
		return RecordVerified
	}
	return RecordPending
}

func observed(answers map[string]DNSAnswer, name string, matches func(string) bool) RecordStatus {
	return dnsrecord.Observe(answers, name, matches)
}

func hostKey(name string) string {
	return dnsrecord.HostKey(name)
}
