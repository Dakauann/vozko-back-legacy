package publicform

import (
	"errors"
	"reflect"
	"testing"
)

func TestParseOriginPatternAcceptsOnlySiteOrigins(t *testing.T) {
	cases := map[string]struct {
		raw  string
		want string
		err  error
	}{
		"https host":                   {raw: "https://loja.example.com", want: "https://loja.example.com"},
		"trailing slash is dropped":    {raw: "https://loja.example.com/", want: "https://loja.example.com"},
		"case is folded":               {raw: "HTTPS://Loja.Example.COM", want: "https://loja.example.com"},
		"explicit port is kept":        {raw: "https://loja.example.com:8443", want: "https://loja.example.com:8443"},
		"default port is dropped":      {raw: "https://loja.example.com:443", want: "https://loja.example.com"},
		"subdomain wildcard":           {raw: "https://*.example.com", want: "https://*.example.com"},
		"http on localhost":            {raw: "http://localhost:3000", want: "http://localhost:3000"},
		"http on loopback ip":          {raw: "http://127.0.0.1:5173", want: "http://127.0.0.1:5173"},
		"bare star is refused":         {raw: "*", err: ErrOriginInvalid},
		"empty is refused":             {raw: "  ", err: ErrOriginInvalid},
		"http on a public host":        {raw: "http://loja.example.com", err: ErrOriginInsecure},
		"a path is refused":            {raw: "https://loja.example.com/chat", err: ErrOriginInvalid},
		"a query is refused":           {raw: "https://loja.example.com?x=1", err: ErrOriginInvalid},
		"credentials are refused":      {raw: "https://user:pw@loja.example.com", err: ErrOriginInvalid},
		"other schemes are refused":    {raw: "ftp://loja.example.com", err: ErrOriginInvalid},
		"no scheme is refused":         {raw: "loja.example.com", err: ErrOriginInvalid},
		"wildcard on a tld is refused": {raw: "https://*.com", err: ErrOriginInvalid},
		"inner wildcard is refused":    {raw: "https://loja.*.com", err: ErrOriginInvalid},
		"double wildcard is refused":   {raw: "https://*.*.example.com", err: ErrOriginInvalid},
		"wildcard on localhost":        {raw: "http://*.localhost", err: ErrOriginInvalid},
		"spaces inside are refused":    {raw: "https://lo ja.example.com", err: ErrOriginInvalid},
		"csp separators are refused":   {raw: "https://loja.example.com;script-src", err: ErrOriginInvalid},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ParseOriginPattern(tc.raw)
			if tc.err != nil {
				if !errors.Is(err, tc.err) {
					t.Fatalf("ParseOriginPattern(%q) error = %v, want %v", tc.raw, err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseOriginPattern(%q) unexpected error %v", tc.raw, err)
			}
			if got.String() != tc.want {
				t.Fatalf("ParseOriginPattern(%q) = %q, want %q", tc.raw, got.String(), tc.want)
			}
		})
	}
}

func TestParseExactOriginRefusesWildcards(t *testing.T) {
	cases := map[string]struct {
		raw  string
		want string
		err  error
	}{
		"exact host":            {raw: "https://loja.example.com/", want: "https://loja.example.com"},
		"exact host with port":  {raw: "https://loja.example.com:8443", want: "https://loja.example.com:8443"},
		"wildcard is refused":   {raw: "https://*.example.com", err: ErrOriginInvalid},
		"insecure is refused":   {raw: "http://loja.example.com", err: ErrOriginInsecure},
		"garbage is refused":    {raw: "loja", err: ErrOriginInvalid},
		"bare star is refused":  {raw: "*", err: ErrOriginInvalid},
		"empty string refused":  {raw: "", err: ErrOriginInvalid},
		"path is still refused": {raw: "https://loja.example.com/x", err: ErrOriginInvalid},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ParseExactOrigin(tc.raw)
			if tc.err != nil {
				if !errors.Is(err, tc.err) {
					t.Fatalf("ParseExactOrigin(%q) error = %v, want %v", tc.raw, err, tc.err)
				}
				return
			}
			if err != nil || got.String() != tc.want {
				t.Fatalf("ParseExactOrigin(%q) = %q, %v, want %q", tc.raw, got.String(), err, tc.want)
			}
		})
	}
}

func TestOriginPatternMatchesOnlyItsOwnSites(t *testing.T) {
	cases := []struct {
		pattern string
		origin  string
		want    bool
	}{
		{"https://loja.example.com", "https://loja.example.com", true},
		{"https://loja.example.com", "https://LOJA.example.com", true},
		{"https://loja.example.com", "https://loja.example.com:443", true},
		{"https://loja.example.com", "http://loja.example.com", false},
		{"https://loja.example.com", "https://loja.example.com:8443", false},
		{"https://loja.example.com", "https://evil-loja.example.com", false},
		{"https://loja.example.com", "https://loja.example.com.evil.io", false},
		{"https://*.example.com", "https://blog.example.com", true},
		{"https://*.example.com", "https://a.b.example.com", true},
		{"https://*.example.com", "https://example.com", false},
		{"https://*.example.com", "https://evilexample.com", false},
		{"https://*.example.com", "null", false},
		{"https://loja.example.com", "", false},
	}
	for _, tc := range cases {
		p, err := ParseOriginPattern(tc.pattern)
		if err != nil {
			t.Fatalf("pattern %q: %v", tc.pattern, err)
		}
		if got := p.Matches(tc.origin); got != tc.want {
			t.Errorf("%q matches %q = %v, want %v", tc.pattern, tc.origin, got, tc.want)
		}
	}
}

func TestOriginAllowListsAdmitOnlyListedSites(t *testing.T) {
	patterns := AllowOriginPatterns([]string{"https://loja.example.com", "https://*.example.org", "nonsense"})
	exact := AllowExactOrigins([]string{"https://loja.example.com", "https://*.example.org", "nonsense"})
	cases := []struct {
		name   string
		list   OriginAllowList
		origin string
		want   bool
	}{
		{"listed exact site", patterns, "https://loja.example.com", true},
		{"wildcard subdomain", patterns, "https://blog.example.org", true},
		{"unlisted site", patterns, "https://evil.example.com", false},
		{"empty origin", patterns, "", false},
		{"exact list admits its site", exact, "https://loja.example.com", true},
		{"exact list drops wildcards", exact, "https://blog.example.org", false},
		{"exact list refuses lookalikes", exact, "https://loja.example.com.evil.io", false},
		{"empty list admits nothing", AllowExactOrigins(nil), "https://loja.example.com", false},
		{"zero list admits nothing", OriginAllowList{}, "https://loja.example.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.list.Allows(tc.origin); got != tc.want {
				t.Fatalf("Allows(%q) = %v, want %v", tc.origin, got, tc.want)
			}
		})
	}
}

func TestOriginAllowListReportsItsCanonicalOrigins(t *testing.T) {
	cases := []struct {
		name string
		list OriginAllowList
		want []string
	}{
		{"patterns keep wildcards", AllowOriginPatterns([]string{"HTTPS://Loja.example.com/", "https://*.example.org", "bad"}), []string{"https://loja.example.com", "https://*.example.org"}},
		{"exact drops wildcards", AllowExactOrigins([]string{"https://loja.example.com", "https://*.example.org"}), []string{"https://loja.example.com"}},
		{"nothing valid", AllowExactOrigins([]string{"bad"}), []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.list.Origins(); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Origins() = %v, want %v", got, tc.want)
			}
			if tc.list.Empty() != (len(tc.want) == 0) {
				t.Fatalf("Empty() = %v with origins %v", tc.list.Empty(), tc.want)
			}
		})
	}
}

func TestAllowListsCombineWithoutWideningEither(t *testing.T) {
	combined := AllowOriginPatterns([]string{"https://*.loja.com.br"}).With(AllowExactOrigins([]string{"https://ofertas.vozko.site", "https://*.wild.example"}))
	for origin, want := range map[string]bool{
		"https://www.loja.com.br":        true,
		"https://ofertas.vozko.site":     true,
		"https://sub.ofertas.vozko.site": false,
		"https://a.wild.example":         false,
	} {
		if got := combined.Allows(origin); got != want {
			t.Errorf("Allows(%q) = %v, want %v", origin, got, want)
		}
	}
	if len(combined.Origins()) != 2 {
		t.Fatalf("origins = %v", combined.Origins())
	}
}
