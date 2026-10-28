package siterender

import (
	"strings"
	"testing"
)

func TestProbeExposesEveryCallTheClientMakes(t *testing.T) {
	for _, call := range []string{"visit", "settled", "pageHeight", "tokens", "describe", "scrollOnce"} {
		if !strings.Contains(probeSource, call+",") && !strings.Contains(probeSource, call+" }") {
			t.Fatalf("probe.js does not export %s", call)
		}
	}
}

func TestAllowedSchemesAreWebOnly(t *testing.T) {
	for scheme, want := range map[string]bool{"https": true, "http": true, "wss": true, "ws": true, "file": false, "chrome": false, "ftp": false, "": false} {
		if allowedScheme(scheme) != want {
			t.Fatalf("allowedScheme(%q) != %v", scheme, want)
		}
	}
}
