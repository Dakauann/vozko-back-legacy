package whatsapplink

import (
	"net/url"
	"testing"
)

func TestURLEscapesTheMessageLikeTheBrowser(t *testing.T) {
	cases := map[string]struct {
		digits string
		text   string
		want   string
	}{
		"no message":   {"5584999990000", "", "https://wa.me/5584999990000"},
		"spaces":       {"5584999990000", "Quero saber mais", "https://wa.me/5584999990000?text=Quero%20saber%20mais"},
		"accents":      {"5511999998888", "Olá, quero o plano Pro! (50% off) ~*'", "https://wa.me/5511999998888?text=Ol%C3%A1%2C%20quero%20o%20plano%20Pro!%20(50%25%20off)%20~*'"},
		"emoji":        {"5511999998888", "Oi 👋", "https://wa.me/5511999998888?text=Oi%20%F0%9F%91%8B"},
		"line breaks":  {"5511999998888", "linha 1\nlinha 2\r\nfim", "https://wa.me/5511999998888?text=linha%201%0Alinha%202%0D%0Afim"},
		"query breaks": {"5511999998888", "a&b=c+d?#/", "https://wa.me/5511999998888?text=a%26b%3Dc%2Bd%3F%23%2F"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := URL(tc.digits, tc.text)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			if tc.text == "" {
				return
			}
			parsed, err := url.Parse(got)
			if err != nil || parsed.Query().Get("text") != tc.text {
				t.Fatalf("the message does not round trip: %q %v", parsed.Query().Get("text"), err)
			}
		})
	}
}
