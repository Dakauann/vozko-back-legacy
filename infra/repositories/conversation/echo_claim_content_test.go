package conversation_repository

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"vozko/domain/conversation"
	"vozko/domain/shared"
)

const claimedMenuMetadata = `{"interactive":{"kind":"list","body":"MENU INICIAL","button_label":"Ver opções","sections":[{"rows":[{"id":"filiado","title":"Já sou filiado"}]}]}}`

func menuSendClaimingItsEcho() *conversation.Message {
	send := senderTestMessage(conversation.SentByWorkflow("wf-1"))
	send.MessageType, send.Text = conversation.MessageTypeAIResponse, "MENU INICIAL"
	send.Metadata = []byte(claimedMenuMetadata)
	send.DeliveryStatus = conversation.DeliveryStatusSent
	return send
}

func TestOurSendCarriesItsOptionsOntoTheEchoItClaims(t *testing.T) {
	db, mock, sqlDB := newStatusDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "conversation_messages"`).
		WillReturnError(errors.New(`ERROR: duplicate key value violates unique constraint "ux_cm_entry_external_msgid" (SQLSTATE 23505)`))
	mock.ExpectRollback()
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "conversation_messages" SET "delivery_status"=COALESCE\(NULLIF\(delivery_status, ''\), \$1\),"metadata"=CASE WHEN jsonb_typeof\(metadata\) = 'object' THEN metadata \|\| \$2::jsonb ELSE \$3::jsonb END,"sender_id"=\$4,"sender_kind"=\$5,"updated_at"=\$6 WHERE .*entry_type = \$7 AND entry_id = \$8 AND external_message_id = \$9 AND sender_kind = \$10`).
		WithArgs("sent", claimedMenuMetadata, claimedMenuMetadata, "workflow:wf-1", "workflow", sqlmock.AnyArg(), "unofficial_whatsapp", "entry-1", "3EB0ECHO", "external").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO conversation_origins`).WithArgs("entry-1", "unofficial_whatsapp").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := NewRepository(db).Create(menuSendClaimingItsEcho()); err != nil {
		t.Fatalf("Create() = %v, want the echo claimed with the options", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAClaimWithNothingToAddLeavesTheEchoContentAlone(t *testing.T) {
	db, mock, sqlDB := newStatusDB(t)
	defer sqlDB.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "conversation_messages" SET "sender_id"=\$1,"sender_kind"=\$2,"updated_at"=\$3 WHERE`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO conversation_origins`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	claimed, err := NewRepository(db).ClaimExternalEcho(senderTestMessage(conversation.SentByCampaign("camp-1")))
	if err != nil || !claimed {
		t.Fatalf("ClaimExternalEcho() = %v, %v, want claimed", claimed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestABackfillAddsTheMenuOnlyToTheWorkflowsOwnMessageWithoutOne(t *testing.T) {
	db, mock, sqlDB := newStatusDB(t)
	defer sqlDB.Close()
	menu := conversation.InteractiveContent{
		Kind: conversation.InteractiveKindList, Body: "MENU INICIAL", ButtonLabel: "Ver opções",
		Sections: []conversation.InteractiveSection{{Rows: []conversation.InteractiveRow{{ID: "filiado", Title: "Já sou filiado"}}}},
	}

	mock.ExpectExec(`UPDATE "conversation_messages" SET "metadata"=CASE WHEN jsonb_typeof\(metadata\) = 'object' THEN metadata \|\| \$1::jsonb ELSE \$2::jsonb END,"updated_at"=\$3 WHERE \(entry_type = \$4 AND entry_id = \$5 AND external_message_id = \$6 AND sender_kind = \$7 AND sender_id = \$8\) AND \(metadata IS NULL OR \(jsonb_typeof\(metadata\) = 'object' AND metadata->'interactive' IS NULL\)\)`).
		WithArgs(claimedMenuMetadata, claimedMenuMetadata, sqlmock.AnyArg(), "unofficial_whatsapp", "entry-1", "3EB0ECHO", "workflow", "workflow:wf-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	repaired, err := NewInteractiveBackfill(db).AttachMissingInteractive(context.Background(), conversation.SentMessageRef{
		EntryType: shared.EntryTypeUnofficialWhatsApp, EntryID: "entry-1", ProviderMessageID: "3EB0ECHO", SentBy: conversation.SentByWorkflow("wf-1"),
	}, menu)
	if err != nil || !repaired {
		t.Fatalf("AttachMissingInteractive() = %v, %v, want repaired", repaired, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestABackfillNeverStoresOptionsNobodyCouldRender(t *testing.T) {
	db, mock, sqlDB := newStatusDB(t)
	defer sqlDB.Close()

	repaired, err := NewInteractiveBackfill(db).AttachMissingInteractive(context.Background(), conversation.SentMessageRef{
		EntryType: shared.EntryTypeUnofficialWhatsApp, EntryID: "entry-1", ProviderMessageID: "3EB0ECHO", SentBy: conversation.SentByWorkflow("wf-1"),
	}, conversation.InteractiveContent{Kind: conversation.InteractiveKindList, Body: "MENU"})
	if err != nil || repaired {
		t.Fatalf("AttachMissingInteractive() = %v, %v, want nothing written", repaired, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
