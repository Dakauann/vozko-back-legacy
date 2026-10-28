package sitepublish

import (
	"errors"
	"strings"
	"testing"
)

func TestLinkWhatsAppRewritesEveryReferencedAnchor(t *testing.T) {
	source := `<!DOCTYPE html><html><head></head><body>` +
		`<a href="#" class="btn" data-vz-whatsapp="phone-1" data-vz-text="Quero um orçamento &amp; prazo">Fale</a>` +
		`<a data-vz-whatsapp="inst-2">Sem href</a>` +
		`<a href="#" data-vz-whatsapp="">Sem número</a>` +
		`<vz-whatsapp channel="phone-1" text="Oi">Chat</vz-whatsapp>` +
		`<a href="/contato">Contato</a>` +
		`</body></html>`
	var seen []WhatsAppAnchor
	out, err := LinkWhatsApp(source, func(a WhatsAppAnchor) (string, bool, error) {
		seen = append(seen, a)
		return "https://vzk.to/" + a.NumberRef + "?t=" + a.Text, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<a href="https://vzk.to/phone-1?t=Quero um orçamento &amp; prazo" class="btn" data-vz-whatsapp="phone-1"`,
		`<a href="https://vzk.to/inst-2?t=" data-vz-whatsapp="inst-2">Sem href</a>`,
		`<a href="#" data-vz-whatsapp="">Sem número</a>`,
		`<vz-whatsapp href="https://vzk.to/phone-1?t=Oi" channel="phone-1" text="Oi">`,
		`<a href="/contato">Contato</a>`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %s in\n%s", want, out)
		}
	}
	if len(seen) != 3 || seen[0].Text != "Quero um orçamento & prazo" || seen[2].NumberRef != "phone-1" {
		t.Fatalf("got %+v", seen)
	}
}

func TestLinkWhatsAppLeavesUnresolvedAnchorsAndRefusesUnsafeLinks(t *testing.T) {
	source := `<html><body><a href="#" data-vz-whatsapp="phone-1">Fale</a></body></html>`
	out, err := LinkWhatsApp(source, func(WhatsAppAnchor) (string, bool, error) { return "", false, nil })
	if err != nil || out != source {
		t.Fatalf("got %s %v", out, err)
	}
	boom := errors.New("number gone")
	if _, err := LinkWhatsApp(source, func(WhatsAppAnchor) (string, bool, error) { return "", false, boom }); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
	if _, err := LinkWhatsApp(source, func(WhatsAppAnchor) (string, bool, error) { return "javascript:alert(1)", true, nil }); !errors.Is(err, ErrWhatsAppLinkUnsafe) {
		t.Fatalf("got %v", err)
	}
	if _, err := LinkWhatsApp(`<html><body><a href="#" data-vz-whatsapp="x"></body>`, func(WhatsAppAnchor) (string, bool, error) { return "https://x", true, nil }); err == nil {
		t.Fatal("malformed html refuses")
	}
}
