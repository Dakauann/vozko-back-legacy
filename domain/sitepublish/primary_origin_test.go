package sitepublish

import "testing"

func TestPrimaryOriginPrefersAnActiveCustomHostname(t *testing.T) {
	cases := map[string]struct {
		list []HostnameRecord
		want string
	}{
		"none":          {nil, ""},
		"platform only": {[]HostnameRecord{{Hostname: "clinica.vozko.site", Kind: HostnamePlatform, Status: HostnameActive}}, "https://clinica.vozko.site"},
		"custom wins": {[]HostnameRecord{
			{Hostname: "clinica.vozko.site", Kind: HostnamePlatform, Status: HostnameActive},
			{Hostname: "www.clinica.com.br", Kind: HostnameCustom, Status: HostnameActive},
		}, "https://www.clinica.com.br"},
		"pending custom ignored": {[]HostnameRecord{
			{Hostname: "www.clinica.com.br", Kind: HostnameCustom, Status: HostnamePendingDNS},
			{Hostname: "clinica.vozko.site", Kind: HostnamePlatform, Status: HostnameActive},
		}, "https://clinica.vozko.site"},
		"nothing active": {[]HostnameRecord{{Hostname: "clinica.vozko.site", Kind: HostnamePlatform, Status: HostnameRemoving}}, ""},
		"local platform": {[]HostnameRecord{{Hostname: "clinica.paginas.localhost", Kind: HostnamePlatform, Status: HostnameActive}}, "http://clinica.paginas.localhost:4000"},
	}
	for name, c := range cases {
		origin, ok := PrimaryOrigin(c.list, Origins{LoopbackPort: "4000"})
		if origin != c.want || ok != (c.want != "") {
			t.Fatalf("%s: got %q %v", name, origin, ok)
		}
	}
}
