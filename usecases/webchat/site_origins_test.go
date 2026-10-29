package webchat

import (
	"context"
	"errors"
	"testing"

	wcdomain "vozko/domain/webchat"
)

type pageOrigins struct {
	origins []string
	err     error
	asked   []string
}

func (p *pageOrigins) WidgetOrigins(_ context.Context, workspaceID, widgetID string) ([]string, error) {
	p.asked = append(p.asked, workspaceID+"/"+widgetID)
	return p.origins, p.err
}

func TestAPublishedPageEmbeddingTheWidgetIsAcceptedWithoutEditingIt(t *testing.T) {
	h := newHarness(t, nil)
	pages := &pageOrigins{origins: []string{"https://ofertas.vozko.site"}}
	h.svc.SiteOrigins = pages
	if _, err := h.svc.Challenge(context.Background(), h.widget.PublicKey, "https://ofertas.vozko.site"); err != nil {
		t.Fatalf("a published page = %v", err)
	}
	if len(pages.asked) != 1 || pages.asked[0] != "ws-1/widget-1" {
		t.Fatalf("asked %v", pages.asked)
	}
	frame, err := h.svc.WidgetForFrame(context.Background(), h.widget.PublicKey, "")
	if err != nil || !frame.Widget.AllowsOrigin("https://ofertas.vozko.site") {
		t.Fatalf("frame widget %v", err)
	}
	pages.origins = nil
	if _, err := h.svc.Challenge(context.Background(), h.widget.PublicKey, "https://ofertas.vozko.site"); !errors.Is(err, wcdomain.ErrOriginNotAllowed) {
		t.Fatalf("an unpublished page = %v", err)
	}
}

func TestUnreadablePageOriginsRefuseTheWidget(t *testing.T) {
	h := newHarness(t, nil)
	h.svc.SiteOrigins = &pageOrigins{err: errors.New("db down")}
	if _, err := h.svc.Challenge(context.Background(), h.widget.PublicKey, siteOrigin); err == nil {
		t.Fatal("unreadable page origins must refuse")
	}
}
