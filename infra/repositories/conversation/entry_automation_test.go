package conversation_repository

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"vozko/domain/conversation"
	"vozko/domain/shared"
)

func aiProfileColumns() []string {
	return []string{"lead_id", "business_phone_id", "campaign_id", "campaign_name",
		"agent_id", "workflow_id", "agent_responses_enabled", "workflow_enabled",
		"automation_enabled", "conversation_status"}
}

func TestEntryAIProfile_ReadsEveryChannelThroughItsEntryInfoQuery(t *testing.T) {
	for _, ch := range channelQueries {
		t.Run(string(ch.EntryType), func(t *testing.T) {
			db, mock, sqlDB := newStatusDB(t)
			defer sqlDB.Close()

			mock.ExpectQuery(`AS automation_enabled`).
				WithArgs("entry-1", "entry-1", sqlmock.AnyArg()).
				WillReturnRows(sqlmock.NewRows(aiProfileColumns()).
					AddRow("", "", "", "", "agent-1", "", true, false, nil, ""))

			got, err := NewEntryAutomationReader(db).EntryAutomation("entry-1", string(ch.EntryType))
			if err != nil {
				t.Fatal(err)
			}
			want := conversation.AutomationProfile{AgentID: "agent-1", AgentResponsesEnabled: true}
			if got.AgentID != want.AgentID || got.AgentResponsesEnabled != want.AgentResponsesEnabled ||
				got.WorkflowEnabled || got.AutomationEnabled != nil {
				t.Fatalf("profile = %+v, want %+v", got, want)
			}
		})
	}
}

func TestEntryAIProfile_CarriesThePauseSwitch(t *testing.T) {
	db, mock, sqlDB := newStatusDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`AS automation_enabled`).
		WillReturnRows(sqlmock.NewRows(aiProfileColumns()).
			AddRow("", "", "", "", "agent-1", "wf-1", true, true, false, ""))

	got, err := NewEntryAutomationReader(db).EntryAutomation("entry-1", string(shared.EntryTypeTelegram))
	if err != nil {
		t.Fatal(err)
	}
	if got.AutomationEnabled == nil || *got.AutomationEnabled {
		t.Fatalf("AutomationEnabled = %v, want an explicit false", got.AutomationEnabled)
	}
	if got.WorkflowID != "wf-1" || !got.WorkflowEnabled {
		t.Fatalf("workflow fields lost: %+v", got)
	}
}

func TestEntryAIProfile_FailsInsteadOfReturningAZeroProfile(t *testing.T) {
	t.Run("query error", func(t *testing.T) {
		db, mock, sqlDB := newStatusDB(t)
		defer sqlDB.Close()
		mock.ExpectQuery(`AS automation_enabled`).WillReturnError(errors.New("connection reset"))

		if _, err := NewEntryAutomationReader(db).EntryAutomation("entry-1", "whatsapp"); err == nil {
			t.Fatal("a failed query must be an error")
		}
	})

	t.Run("no such conversation", func(t *testing.T) {
		db, mock, sqlDB := newStatusDB(t)
		defer sqlDB.Close()
		mock.ExpectQuery(`AS automation_enabled`).WillReturnRows(sqlmock.NewRows(aiProfileColumns()))

		_, err := NewEntryAutomationReader(db).EntryAutomation("entry-1", "whatsapp")
		if !errors.Is(err, conversation.ErrConversationNotFound) {
			t.Fatalf("err = %v, want ErrConversationNotFound", err)
		}
	})

	t.Run("unknown channel", func(t *testing.T) {
		db, _, sqlDB := newStatusDB(t)
		defer sqlDB.Close()

		_, err := NewEntryAutomationReader(db).EntryAutomation("entry-1", "fax")
		if !errors.Is(err, conversation.ErrEntryTypeInvalid) {
			t.Fatalf("err = %v, want ErrEntryTypeInvalid", err)
		}
	})
}

func TestEntryAccountID_ReadsTheChannelAccountFromTheSameQuery(t *testing.T) {
	db, mock, sqlDB := newStatusDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`AS business_phone_id`).
		WithArgs("entry-1", "entry-1", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(aiProfileColumns()).
			AddRow("", "account-7", "", "", "", "", false, false, nil, ""))

	got, err := NewEntryAutomationReader(db).EntryAccountID("entry-1", string(shared.EntryTypeTelegram))
	if err != nil {
		t.Fatal(err)
	}
	if got != "account-7" {
		t.Fatalf("account = %q, want account-7", got)
	}
}

func TestEntryAccountID_FailsForAnUnknownConversation(t *testing.T) {
	db, mock, sqlDB := newStatusDB(t)
	defer sqlDB.Close()
	mock.ExpectQuery(`AS business_phone_id`).WillReturnRows(sqlmock.NewRows(aiProfileColumns()))

	_, err := NewEntryAutomationReader(db).EntryAccountID("entry-1", "whatsapp")
	if !errors.Is(err, conversation.ErrConversationNotFound) {
		t.Fatalf("err = %v, want ErrConversationNotFound", err)
	}
}

func TestEntryAIProfile_CarriesTheDelegateOnEveryChannel(t *testing.T) {
	for _, ch := range channelQueries {
		t.Run(string(ch.EntryType), func(t *testing.T) {
			db, mock, sqlDB := newStatusDB(t)
			defer sqlDB.Close()

			mock.ExpectQuery(`LEFT JOIN conversation_delegations`).
				WithArgs("entry-1", "entry-1", string(ch.EntryType)).
				WillReturnRows(sqlmock.NewRows(append(aiProfileColumns(), "delegate_kind", "delegate_id")).
					AddRow("", "", "", "", "", "", false, false, true, "", "workflow", "wf-9"))

			got, err := NewEntryAutomationReader(db).EntryAutomation("entry-1", string(ch.EntryType))
			if err != nil {
				t.Fatal(err)
			}
			if got.Delegate == nil || *got.Delegate != (conversation.Automation{Kind: conversation.AutomationWorkflow, ID: "wf-9"}) {
				t.Fatalf("delegate = %+v", got.Delegate)
			}
		})
	}
}

func TestEntryAIProfile_WithoutADelegateHasNone(t *testing.T) {
	db, mock, sqlDB := newStatusDB(t)
	defer sqlDB.Close()

	mock.ExpectQuery(`LEFT JOIN conversation_delegations`).
		WillReturnRows(sqlmock.NewRows(append(aiProfileColumns(), "delegate_kind", "delegate_id")).
			AddRow("", "", "", "", "agent-1", "", true, false, nil, "", "", ""))

	got, err := NewEntryAutomationReader(db).EntryAutomation("entry-1", string(shared.EntryTypeTelegram))
	if err != nil {
		t.Fatal(err)
	}
	if got.Delegate != nil {
		t.Fatalf("delegate = %+v, want none", got.Delegate)
	}
}

func TestEntryAutomation_NamesTheResponsibleWorkflowOnEveryKnownChannel(t *testing.T) {
	columns := append(aiProfileColumns(), "delegate_kind", "delegate_id")
	for _, entryType := range shared.KnownEntryTypes() {
		t.Run(string(entryType), func(t *testing.T) {
			for _, tc := range []struct {
				name         string
				delegateKind string
				delegateID   string
				want         string
			}{
				{"container workflow", "", "", "wf-container"},
				{"delegated workflow", "workflow", "wf-delegate", "wf-delegate"},
			} {
				db, mock, sqlDB := newStatusDB(t)
				mock.ExpectQuery(`conversation_delegations`).
					WithArgs("entry-1", "entry-1", string(entryType)).
					WillReturnRows(sqlmock.NewRows(columns).
						AddRow("", "", "", "", "", "wf-container", false, true, nil, "", tc.delegateKind, tc.delegateID))

				profile, err := NewEntryAutomationReader(db).EntryAutomation("entry-1", string(entryType))
				sqlDB.Close()
				if err != nil {
					t.Fatalf("%s: %v", tc.name, err)
				}
				if got, ok := profile.ResponsibleWorkflow(); !ok || got != tc.want {
					t.Fatalf("%s: responsible workflow = %q (%v), want %q", tc.name, got, ok, tc.want)
				}
			}
		})
	}
}
