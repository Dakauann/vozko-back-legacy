package sitepublish

import (
	"errors"
	"strings"
	"testing"

	"vozko/domain/sitehtml"
)

func TestLinkWebchatStampsTheResolvedWidgetAndLeavesUnknownOnesInert(t *testing.T) {
	source := `<main><vz-webchat widget="wc_live"></vz-webchat><vz-webchat widget=" ghost " data-vz-webchat-key="forged"></vz-webchat><vz-webchat></vz-webchat></main>`
	asked := []string{}
	out, err := LinkWebchat(source, func(ref string) (string, bool, error) {
		asked = append(asked, ref)
		if ref == "wc_live" {
			return "pk_Abc-123", true, nil
		}
		return "", false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(asked, ",") != "wc_live,ghost" {
		t.Fatalf("asked %v", asked)
	}
	doc, err := sitehtml.Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	chats := doc.ByTag("vz-webchat")
	if key, _ := doc.AttrValue(chats[0], WebchatKeyAttr); key != "pk_Abc-123" {
		t.Fatalf("got %s", out)
	}
	for _, i := range chats[1:] {
		if doc.HasAttr(i, WebchatKeyAttr) {
			t.Fatalf("an unknown widget must stay inert: %s", out)
		}
	}
	if !NeedsRuntime(doc) {
		t.Fatal("a page with a chat needs the runtime")
	}
}

func TestLinkWebchatRefusesUnsafeValuesAndPassesErrors(t *testing.T) {
	source := `<vz-webchat widget="wc"></vz-webchat>`
	if _, err := LinkWebchat(source, func(string) (string, bool, error) { return `x" onclick="y`, true, nil }); !errors.Is(err, ErrWebchatKeyUnsafe) {
		t.Fatalf("got %v", err)
	}
	boom := errors.New("boom")
	if _, err := LinkWebchat(source, func(string) (string, bool, error) { return "", false, boom }); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
	if out, err := LinkWebchat("<p>no chat</p>", nil); err != nil || out != "<p>no chat</p>" {
		t.Fatalf("got %q %v", out, err)
	}
}
