package site_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/site"
)

func TestTheProjectViewCarriesTheThumbnailAddress(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	shown := h.create(t, site.StackHTML, "blank", "Com miniatura")
	plain := h.create(t, site.StackHTML, "blank", "Sem miniatura")
	gone := h.create(t, site.StackHTML, "blank", "Miniatura apagada")
	h.store.setThumbnail(shown.Project.ID, "media-1")
	h.store.setThumbnail(gone.Project.ID, "media-gone")
	h.media.urls["media-1"] = "https://cdn.vozko.test/thumb.jpg"

	v, err := h.svc.Get(ctx, wsA, shown.Project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Project.ThumbnailMediaID != "media-1" || v.Project.ThumbnailURL != "https://cdn.vozko.test/thumb.jpg" {
		t.Fatalf("got %+v", v.Project)
	}
	items, _, err := h.svc.List(ctx, ListQuery{WorkspaceID: wsA})
	if err != nil {
		t.Fatal(err)
	}
	urls := map[string]string{}
	for _, p := range items {
		urls[p.ID] = p.ThumbnailURL
	}
	if urls[shown.Project.ID] != "https://cdn.vozko.test/thumb.jpg" || urls[plain.Project.ID] != "" || urls[gone.Project.ID] != "" {
		t.Fatalf("got %v", urls)
	}
	if len(h.media.calls) != 2 || len(h.media.calls[1]) != 3 {
		t.Fatalf("the list locates its thumbnails in one call: %+v", h.media.calls)
	}
	h.media.err = errors.New("media down")
	if _, err := h.svc.Get(ctx, wsA, shown.Project.ID); err == nil {
		t.Fatal("an unreadable media library refuses")
	}
}
