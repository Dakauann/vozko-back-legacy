package hostname

import (
	"net"
	"strings"
)

func Normalize(host string) string {
	name := strings.ToLower(strings.TrimSpace(host))
	if h, _, err := net.SplitHostPort(name); err == nil {
		name = h
	}
	return strings.TrimSuffix(name, ".")
}

func SubdomainLabel(host, domain string) (string, bool) {
	domain = Normalize(domain)
	if domain == "" {
		return "", false
	}
	label, ok := strings.CutSuffix(Normalize(host), "."+domain)
	if !ok || label == "" || strings.Contains(label, ".") {
		return "", false
	}
	return label, true
}

func Loopback(host string) bool {
	name := Normalize(host)
	return name == "localhost" || strings.HasSuffix(name, ".localhost")
}
