package webchat

import (
	"context"
	"errors"
	"strings"
	"testing"

	wcdomain "vozko/domain/webchat"
)

func TestUsageListsThePagesAndSitesThatShowTheWidget(t *testing.T) {
	repo := &fakeWidgets{byID: map[string]*wcdomain.Widget{"w-1": {ID: "w-1", WorkspaceID: "ws-1", AllowedOrigins: []string{"https://loja.com.br", "https://*.loja.com.br"}}}}
	pages := &pageOrigins{origins: []string{"https://ofertas.vozko.app", "https://promo.vozko.app"}}
	usage, err := NewUsage(repo, pages)
	if err != nil {
		t.Fatal(err)
	}
	got, err := usage.Of(context.Background(), "ws-1", "w-1")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.SiteOrigins, ",") != "https://ofertas.vozko.app,https://promo.vozko.app" || strings.Join(got.AllowedOrigins, ",") != "https://loja.com.br,https://*.loja.com.br" {
		t.Fatalf("got %+v", got)
	}
	if pages.asked[0] != "ws-1/w-1" {
		t.Fatalf("asked %v", pages.asked)
	}
}

func TestUsageAnswersEmptyListsAndFailsClosed(t *testing.T) {
	repo := &fakeWidgets{byID: map[string]*wcdomain.Widget{"w-1": {ID: "w-1", WorkspaceID: "ws-1"}}}
	usage, _ := NewUsage(repo, &pageOrigins{})
	got, err := usage.Of(context.Background(), "ws-1", "w-1")
	if err != nil || got.SiteOrigins == nil || got.AllowedOrigins == nil {
		t.Fatalf("got %+v %v", got, err)
	}
	failing, _ := NewUsage(repo, &pageOrigins{err: errors.New("db down")})
	if _, err := failing.Of(context.Background(), "ws-1", "w-1"); err == nil {
		t.Fatal("an unreadable page list is never an unused widget")
	}
	if _, err := usage.Of(context.Background(), "ws-2", "w-1"); err == nil {
		t.Fatal("another workspace never reads the widget")
	}
	if _, err := NewUsage(repo, nil); err == nil {
		t.Fatal("usage needs the page origins")
	}
}
