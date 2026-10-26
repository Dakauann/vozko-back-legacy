package workflow_infra

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"vozko/domain/conversation"
	ia "vozko/domain/inbox_assignment"
	"vozko/domain/shared"
	"vozko/domain/workflow"
	conversation_repository "vozko/infra/repositories/conversation"
	conversation_usecase "vozko/usecases/conversation"
)

func gateDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	return db, mock, sqlDB
}

func entryInfoRow(automation interface{}) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"lead_id", "business_phone_id", "campaign_id", "campaign_name",
		"agent_id", "workflow_id", "agent_responses_enabled", "workflow_enabled",
		"automation_enabled", "conversation_status"}).
		AddRow("", "", "", "", "", "", false, false, automation, "")
}

var everyChannel = []shared.EntryType{
	shared.EntryTypeWhatsApp,
	shared.EntryTypeInstagram,
	shared.EntryTypeTelegram,
	shared.EntryTypeUnofficialWhatsApp,
	shared.EntryTypeFacebook,
	shared.EntryTypeWebchat,
	shared.EntryTypeMercadoLivre,
	shared.EntryTypeTikTok,
}

func TestGateReadsThePauseSwitchOfEveryChannelNotOnlyOfficialWhatsApp(t *testing.T) {
	cases := []struct {
		name       string
		automation interface{}
		want       bool
	}{
		{"paused", false, false},
		{"explicitly on", true, true},
		{"never set", nil, true},
	}
	for _, channel := range everyChannel {
		for _, c := range cases {
			t.Run(string(channel)+"/"+c.name, func(t *testing.T) {
				db, mock, sqlDB := gateDB(t)
				defer sqlDB.Close()
				mock.ExpectQuery(`AS automation_enabled`).
					WithArgs("entry-1", "entry-1", string(channel)).
					WillReturnRows(entryInfoRow(c.automation))

				gate := NewAutomationGate(conversation_repository.NewEntryAutomationReader(db), ownerGate(""))
				got, err := gate.AutomationEnabled("entry-1", string(channel))
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != c.want {
					t.Fatalf("enabled = %v, want %v", got, c.want)
				}
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestGateRefusesWhatItCannotReadInsteadOfLettingTheRunContinue(t *testing.T) {
	cases := []struct {
		name      string
		entryType string
		expect    func(sqlmock.Sqlmock)
		wantErr   error
	}{
		{"unknown entry type", "messenger", func(sqlmock.Sqlmock) {}, conversation.ErrEntryTypeInvalid},
		{"empty entry type", "", func(sqlmock.Sqlmock) {}, conversation.ErrEntryTypeInvalid},
		{"call entry without a pause switch", string(workflow.EntryTypeSIPCall), func(sqlmock.Sqlmock) {}, conversation.ErrEntryTypeInvalid},
		{"database error", "instagram", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(`AS automation_enabled`).WillReturnError(errors.New("connection reset"))
		}, nil},
		{"conversation gone", "telegram", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(`AS automation_enabled`).WillReturnRows(sqlmock.NewRows([]string{"automation_enabled"}))
		}, conversation.ErrConversationNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db, mock, sqlDB := gateDB(t)
			defer sqlDB.Close()
			c.expect(mock)

			got, err := NewAutomationGate(conversation_repository.NewEntryAutomationReader(db), ownerGate("")).AutomationEnabled("entry-1", c.entryType)
			if err == nil {
				t.Fatal("an unreadable automation state must be an error")
			}
			if c.wantErr != nil && !errors.Is(err, c.wantErr) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
			if got {
				t.Fatal("an unreadable automation state must never report automation as on")
			}
		})
	}
}

func TestGateWithoutAReaderIsAnErrorNotAPass(t *testing.T) {
	got, err := NewAutomationGate(nil, nil).AutomationEnabled("entry-1", "whatsapp")
	if err == nil || got {
		t.Fatalf("got %v, %v; want false and an error", got, err)
	}
}

type fixedOwner string

func (o fixedOwner) FindByEntry(workspaceID, entryID, entryType string) (*ia.InboxAssignment, error) {
	if o == "" {
		return nil, nil
	}
	return &ia.InboxAssignment{WorkspaceID: workspaceID, EntryID: entryID, EntryType: entryType, AssignedUserID: string(o)}, nil
}

type noDelegation struct {
	conversation.DelegationRepository
}

func (noDelegation) Find(context.Context, string, shared.EntryType) (*conversation.Delegation, error) {
	return nil, nil
}

type agentProfile struct{}

func (agentProfile) EntryAutomation(string, string) (conversation.AutomationProfile, error) {
	return conversation.AutomationProfile{AgentID: "agent-1", AgentResponsesEnabled: true}, nil
}

func ownerGate(owner fixedOwner) *conversation_usecase.AutomationGate {
	return conversation_usecase.NewAutomationGate(conversation_usecase.AutomationGateDeps{Owners: owner, Delegations: noDelegation{}, Profiles: agentProfile{}})
}

func TestGateAsksTheConversationRuleWhetherAnAnsweringRunMayAct(t *testing.T) {
	for owner, want := range map[fixedOwner]bool{"": true, "ai:agent-1": true, "user-1": false} {
		got, err := NewAutomationGate(agentProfile{}, ownerGate(owner)).AutomationMayAct("ws-1", "entry-1", "instagram")
		if err != nil || got != want {
			t.Errorf("owner %q: may act = %v, %v, want %v", owner, got, err, want)
		}
	}
}

func TestGateWithoutTheConversationRuleIsAnErrorNotAPass(t *testing.T) {
	got, err := NewAutomationGate(agentProfile{}, nil).AutomationMayAct("ws-1", "entry-1", "instagram")
	if err == nil || got {
		t.Fatalf("got %v, %v; want false and an error", got, err)
	}
}
