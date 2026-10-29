package webchat

import (
	"context"
	"errors"
	"testing"
)

type widgetBook map[string]*Widget

func (b widgetBook) FindByPublicKey(_ context.Context, key string) (*Widget, error) {
	for _, w := range b {
		if w.PublicKey == key {
			return w, nil
		}
	}
	return nil, ErrWidgetNotFound
}

func (b widgetBook) FindByID(_ context.Context, workspaceID, id string) (*Widget, error) {
	if w, ok := b[id]; ok && w.WorkspaceID == workspaceID {
		return w, nil
	}
	return nil, ErrWidgetNotFound
}

func TestFindSiteWidgetTakesTheKeyOrTheIDButOnlyFromTheSameWorkspace(t *testing.T) {
	ours := &Widget{ID: "6f1c3c1e-8d7a-4f39-9a52-2a8d6c1f0b11", WorkspaceID: "ws-1", PublicKey: "pk_ours"}
	theirs := &Widget{ID: "0b7e9a2c-4b8f-4b0e-8f1a-9d2e3c4b5a6f", WorkspaceID: "ws-2", PublicKey: "pk_theirs"}
	book := widgetBook{ours.ID: ours, theirs.ID: theirs}
	for _, ref := range []string{"pk_ours", " " + ours.ID + " "} {
		if w, err := FindSiteWidget(context.Background(), book, "ws-1", ref); err != nil || w != ours {
			t.Fatalf("%q: got %v %v", ref, w, err)
		}
	}
	for _, ref := range []string{"pk_theirs", theirs.ID, "pk_missing", ""} {
		if _, err := FindSiteWidget(context.Background(), book, "ws-1", ref); !errors.Is(err, ErrWidgetNotFound) {
			t.Fatalf("%q: got %v", ref, err)
		}
	}
}
