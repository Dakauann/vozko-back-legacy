package hostname

import "testing"

func TestNormalizeLowersTrimsAndDropsThePortAndTrailingDot(t *testing.T) {
	cases := map[string]string{
		"Loja.Example.COM":      "loja.example.com",
		"  loja.example.com.  ": "loja.example.com",
		"loja.example.com:8443": "loja.example.com",
		"LOJA.example.com.:443": "loja.example.com",
		"[2001:db8::1]:443":     "2001:db8::1",
		"":                      "",
		"abc.preview.vozko.dev": "abc.preview.vozko.dev",
	}
	for raw, want := range cases {
		if got := Normalize(raw); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestSubdomainLabelTakesOneLabelUnderTheDomain(t *testing.T) {
	cases := []struct {
		host, label string
		ok          bool
	}{
		{"abc.preview.test", "abc", true},
		{"ABC.Preview.Test.:443", "abc", true},
		{"a.b.preview.test", "", false},
		{"preview.test", "", false},
		{"abc.other.test", "", false},
	}
	for _, c := range cases {
		if label, ok := SubdomainLabel(c.host, "Preview.Test"); label != c.label || ok != c.ok {
			t.Errorf("%s: got %q %v", c.host, label, ok)
		}
	}
	if _, ok := SubdomainLabel("abc.preview.test", ""); ok {
		t.Error("no domain, no label")
	}
}

func TestLoopbackNamesAreLocalhostOnly(t *testing.T) {
	for host, want := range map[string]bool{"localhost": true, "x.localhost": true, "X.Localhost.:4000": true, "localhost.example": false, "notlocalhost": false} {
		if got := Loopback(host); got != want {
			t.Errorf("%s: got %v", host, got)
		}
	}
}
