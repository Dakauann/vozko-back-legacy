package site_usecase

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"vozko/domain/site"
)

func insertElementBatch(t *testing.T, element, key string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal([]map[string]any{
		{"op": "insert_element", "at": map[string]string{"page": "pages/index.html"}, "rev": 1, "element": element, "key": key},
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestInsertedPlatformElementsMustNameAWidgetOrBookingOfThisWorkspace(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	v := h.create(t, site.StackHTML, "blank", "Nova")
	stored := &StoredSettings{ProjectID: v.Project.ID, WorkspaceID: wsA, Settings: site.Settings{Bookings: map[string]site.Booking{"agenda": {}}}}
	if err := h.store.SaveSettings(ctx, stored, 1); err != nil {
		t.Fatal(err)
	}
	edit := func(batch json.RawMessage) error {
		_, err := h.svc.Edit(ctx, EditInput{WorkspaceID: wsA, ProjectID: v.Project.ID, ExpectedVersion: 1, Author: PersonAuthor(userA), Batch: batch})
		return err
	}
	refusedWith := func(err error, code string) bool {
		var refusal *EditRefusal
		return errors.As(err, &refusal) && len(refusal.Failures) == 1 && refusal.Failures[0] == (site.Failure{Index: 0, Code: code, Field: "key"})
	}

	if err := edit(insertElementBatch(t, "booking", "outra")); !refusedWith(err, site.CodeBookingUnknown) {
		t.Fatalf("an unknown booking: got %v", err)
	}
	h.refs.refuse["webchat"] = ErrWebchatWidgetUnknown
	if err := edit(insertElementBatch(t, "webchat", "pk_theirs")); !refusedWith(err, site.CodeWebchatWidgetUnknown) {
		t.Fatalf("a foreign widget: got %v", err)
	}
	unavailable := errors.New("widgets down")
	h.refs.refuse["webchat"] = unavailable
	if err := edit(insertElementBatch(t, "webchat", "pk_ours")); !errors.Is(err, unavailable) {
		t.Fatalf("a lookup failure never lets the element in: got %v", err)
	}
	if len(h.executor.calls) != 0 {
		t.Fatal("a refused element never reaches the builder")
	}

	delete(h.refs.refuse, "webchat")
	if err := edit(insertElementBatch(t, "webchat", "pk_ours")); err != nil {
		t.Fatalf("a widget of this workspace: %v", err)
	}
	if got := h.refs.seen[len(h.refs.seen)-1]; got != wsA+"/webchat:pk_ours" {
		t.Fatalf("got %v", h.refs.seen)
	}
	if len(h.executor.calls) != 1 || h.executor.calls[0].Batch.Operations[0].HTML != `<vz-webchat widget="pk_ours"></vz-webchat>` {
		t.Fatalf("got %+v", h.executor.calls)
	}
}

func TestABookingNeedsSavedSettings(t *testing.T) {
	h := newHarness(t)
	v := h.create(t, site.StackHTML, "blank", "Nova")
	_, err := h.svc.Edit(context.Background(), EditInput{WorkspaceID: wsA, ProjectID: v.Project.ID, ExpectedVersion: 1, Author: PersonAuthor(userA), Batch: insertElementBatch(t, "booking", "agenda")})
	var refusal *EditRefusal
	if !errors.As(err, &refusal) || refusal.Failures[0].Code != site.CodeBookingUnknown {
		t.Fatalf("got %v", err)
	}
}
