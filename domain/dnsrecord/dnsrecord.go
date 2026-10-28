package dnsrecord

import (
	"context"
	"strings"
)

type Status string

const (
	Verified Status = "verified"
	Pending  Status = "pending"
	Unknown  Status = "unknown"
)

func (s Status) Valid() bool {
	switch s {
	case Verified, Pending, Unknown:
		return true
	}
	return false
}

type Answer struct {
	Values []string
	Failed bool
}

type TXTLookup interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

func HostKey(name string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
}

func Observe(answers map[string]Answer, name string, matches func(string) bool) Status {
	answer, ok := answers[HostKey(name)]
	if !ok || answer.Failed {
		return Unknown
	}
	for _, v := range answer.Values {
		if matches(v) {
			return Verified
		}
	}
	return Pending
}

func Lookup(ctx context.Context, lookup func(context.Context, string) ([]string, error), names []string, failed func(name string, err error)) map[string]Answer {
	out := make(map[string]Answer, len(names))
	for _, name := range names {
		key := HostKey(name)
		if _, done := out[key]; done {
			continue
		}
		values, err := lookup(ctx, key)
		if err != nil {
			if failed != nil {
				failed(key, err)
			}
			out[key] = Answer{Failed: true}
			continue
		}
		out[key] = Answer{Values: values}
	}
	return out
}

func RelativeHost(fqdn, domain string) string {
	if fqdn == domain {
		return "@"
	}
	if strings.HasSuffix(fqdn, "."+domain) {
		return strings.TrimSuffix(fqdn, "."+domain)
	}
	return fqdn
}
