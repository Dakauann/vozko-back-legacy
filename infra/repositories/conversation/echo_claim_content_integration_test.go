package conversation_repository

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"vozko/domain/conversation"
	"vozko/domain/shared"
)

func claimMenu() conversation.InteractiveContent {
	return conversation.InteractiveContent{
		Kind:        conversation.InteractiveKindList,
		Header:      &conversation.InteractiveHeader{Type: "text", Text: "JURIDICO SINDCOP"},
		Body:        "MENU INICIAL",
		Footer:      "Atendimento",
		ButtonLabel: "Ver opções",
		Sections: []conversation.InteractiveSection{{Title: "Assuntos", Rows: []conversation.InteractiveRow{
			{ID: "filiado", Title: "Já sou filiado"},
			{ID: "processo", Title: "Processo em andamento", Description: "Ação judicial"},
		}}},
	}
}

func echoAndMenuSend(t *testing.T, entryID string) (*conversation.Message, *conversation.Message) {
	t.Helper()
	external := "3EB0MENU" + entryID[:8]
	echo := originMessage(entryID, conversation.SentExternally(), originClock)
	echo.ExternalMessageID, echo.Text = &external, "MENU INICIAL"
	echo.Metadata = json.RawMessage(`{"providerEvent":"messages","trackId":"` + entryID + `"}`)
	send := originMessage(entryID, conversation.SentByWorkflow("wf-1"), originClock)
	send.ExternalMessageID, send.Text = &external, "MENU INICIAL"
	send.DeliveryStatus = conversation.DeliveryStatusSent
	menu := claimMenu()
	if err := send.AttachInteractive(&menu); err != nil {
		t.Fatal(err)
	}
	return echo, send
}

func storedByProvider(t *testing.T, repo conversation.MessageRepository, entryID string, external *string) *conversation.Message {
	t.Helper()
	stored, err := repo.GetByEntryAndExternalMessageID(shared.EntryTypeWhatsApp, entryID, *external)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	return stored
}

func TestAWorkflowMenuClaimingItsEchoIsStoredWithItsOptionsAgainstPostgres(t *testing.T) {
	db := originDB(t)
	repo := NewRepository(db)
	entryID := uuid.NewString()
	echo, send := echoAndMenuSend(t, entryID)
	createAll(t, repo, echo, send)

	stored := storedByProvider(t, repo, entryID, echo.ExternalMessageID)
	if stored.SentBy != conversation.SentByWorkflow("wf-1") {
		t.Fatalf("sender = %+v, want the workflow", stored.SentBy)
	}
	got, ok := stored.Interactive()
	if !ok || !reflect.DeepEqual(got, claimMenu()) {
		t.Fatalf("interactive = %+v (%v), want the list the workflow sent", got, ok)
	}
	if stored.DeliveryStatus != conversation.DeliveryStatusSent {
		t.Fatalf("delivery status = %q, want sent", stored.DeliveryStatus)
	}
	var kept map[string]any
	if err := json.Unmarshal(stored.Metadata, &kept); err != nil || kept["providerEvent"] != "messages" {
		t.Fatalf("metadata = %s, want the echo's provider fields kept", stored.Metadata)
	}
}

func TestAClaimNeverWalksBackAStatusTheEchoAlreadyReachedAgainstPostgres(t *testing.T) {
	db := originDB(t)
	repo := NewRepository(db)
	entryID := uuid.NewString()
	echo, send := echoAndMenuSend(t, entryID)
	echo.DeliveryStatus = conversation.DeliveryStatusRead
	createAll(t, repo, echo, send)

	if got := storedByProvider(t, repo, entryID, echo.ExternalMessageID).DeliveryStatus; got != conversation.DeliveryStatusRead {
		t.Fatalf("delivery status = %q, want read kept", got)
	}
}

func TestAnEchoArrivingAfterTheMenuLeavesItsOptionsAgainstPostgres(t *testing.T) {
	db := originDB(t)
	repo := NewRepository(db)
	entryID := uuid.NewString()
	echo, send := echoAndMenuSend(t, entryID)
	createAll(t, repo, send)
	if err := repo.Create(echo); err == nil {
		t.Fatal("a late echo must stay a duplicate, not a second row")
	}

	got, ok := storedByProvider(t, repo, entryID, echo.ExternalMessageID).Interactive()
	if !ok || !reflect.DeepEqual(got, claimMenu()) {
		t.Fatalf("interactive = %+v (%v), want the list kept", got, ok)
	}
}

func TestTheBackfillRestoresAMenuOnceAndOnlyOnTheWorkflowsRowAgainstPostgres(t *testing.T) {
	db := originDB(t)
	repo := NewRepository(db)
	backfill := NewInteractiveBackfill(db)
	entryID := uuid.NewString()
	echo, _ := echoAndMenuSend(t, entryID)
	plain := originMessage(entryID, conversation.SentByWorkflow("wf-1"), originClock)
	plain.ExternalMessageID, plain.Text = echo.ExternalMessageID, "MENU INICIAL"
	plain.Metadata = json.RawMessage(`{"providerEvent":"messages"}`)
	createAll(t, repo, plain)
	ref := conversation.SentMessageRef{EntryType: shared.EntryTypeWhatsApp, EntryID: entryID, ProviderMessageID: *echo.ExternalMessageID, SentBy: conversation.SentByWorkflow("wf-1")}

	other := ref
	other.SentBy = conversation.SentByWorkflow("wf-2")
	if repaired, err := backfill.AttachMissingInteractive(context.Background(), other, claimMenu()); err != nil || repaired {
		t.Fatalf("another workflow's ref repaired = %v, %v; want untouched", repaired, err)
	}
	if repaired, err := backfill.AttachMissingInteractive(context.Background(), ref, claimMenu()); err != nil || !repaired {
		t.Fatalf("first run repaired = %v, %v; want repaired", repaired, err)
	}
	if repaired, err := backfill.AttachMissingInteractive(context.Background(), ref, claimMenu()); err != nil || repaired {
		t.Fatalf("second run repaired = %v, %v; want idempotent", repaired, err)
	}

	stored := storedByProvider(t, repo, entryID, echo.ExternalMessageID)
	if got, ok := stored.Interactive(); !ok || !reflect.DeepEqual(got, claimMenu()) {
		t.Fatalf("interactive = %+v (%v), want the restored list", got, ok)
	}
	var kept map[string]any
	if err := json.Unmarshal(stored.Metadata, &kept); err != nil || kept["providerEvent"] != "messages" {
		t.Fatalf("metadata = %s, want the provider fields kept", stored.Metadata)
	}
}
