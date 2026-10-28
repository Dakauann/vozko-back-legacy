package automationneeds

import (
	"context"
	"errors"
	"testing"
	"time"

	ce "vozko/domain/conversation_event"
	"vozko/domain/conversationvars"
	"vozko/domain/shared"
	"vozko/domain/variabledecl"
)

type fakeStoredDetails struct {
	rows []conversationvars.Stored
	err  error
}

func (f *fakeStoredDetails) Detailed(context.Context, string, shared.EntryType) ([]conversationvars.Stored, error) {
	return f.rows, f.err
}

type fakeTimeline struct {
	events []*ce.ConversationEvent
}

func (f *fakeTimeline) Create(event *ce.ConversationEvent) error {
	f.events = append(f.events, event)
	return nil
}

type dataFakes struct {
	*formFakes
	stored   *fakeStoredDetails
	timeline *fakeTimeline
}

func newDataFakes() *dataFakes {
	f := &dataFakes{formFakes: newFormFakes(), stored: &fakeStoredDetails{}, timeline: &fakeTimeline{}}
	f.snapshots.snapshot.Values = map[string]string{"lead_name": "Ana", "lead_number": "5511", "campaign_name": "", "turno": "Noite", "matricula": "123", "1": "Ana"}
	f.stored.rows = []conversationvars.Stored{{Name: "matricula", Value: "123", SetBy: "u-9", SetAt: time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)}}
	return f
}

func (f *dataFakes) queries(t *testing.T, allowed bool) *Queries {
	t.Helper()
	q := newQueriesWith(t, &memRecords{}, f.store, allowed, func(d *QueriesDeps) {
		d.Conversations, d.Definitions, d.Subjects, d.Profiles = f.snapshots, f.defs, f.subjects, f.profiles
		d.StoredDetails, d.Timeline = f.stored, f.timeline
	})
	q.d.Service.declarations = fakeDeclarations{rows: []variabledecl.Declaration{
		{Name: "turno", Label: "Turno", Type: variabledecl.Type{Kind: variabledecl.KindChoice, Options: []string{"Manhã", "Tarde"}}},
		{Name: "matricula", Label: "Matrícula", Type: variabledecl.Type{Kind: variabledecl.KindNumber}},
		{Name: "valor", Label: "Valor", Type: variabledecl.Type{Kind: variabledecl.KindNumber}},
	}}
	return q
}

func TestTheConversationDataListsSystemDeclaredAndStoredValuesWithOrigins(t *testing.T) {
	f := newDataFakes()
	rows, err := f.queries(t, true).ConversationData(context.Background(), conversationRef)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]DataRow{}
	for _, row := range rows {
		byName[row.Name] = row
	}
	if _, ok := byName["1"]; ok {
		t.Fatal("template positions are not conversation data")
	}
	if lead := byName["lead_name"]; lead.Origin != OriginSystem || lead.Label != "Nome do contato" || lead.LabelKey != "conversation_lead_name" || lead.Value != "Ana" {
		t.Fatalf("lead_name = %+v", lead)
	}
	if m := byName["matricula"]; m.Origin != OriginStored || m.SetBy != "u-9" || m.Label != "Matrícula" || !m.Fits {
		t.Fatalf("matricula = %+v", m)
	}
	if turno := byName["turno"]; turno.Origin != OriginCampaign || turno.Fits || turno.Shape != variabledecl.ShapeChoice {
		t.Fatalf("turno = %+v, a campaign value outside the options is flagged", turno)
	}
	if valor := byName["valor"]; valor.Value != "" || valor.Origin != OriginNone || !valor.Fits {
		t.Fatalf("valor = %+v", valor)
	}
}

func TestEditingConversationDataChecksTheTypeStoresAndAudits(t *testing.T) {
	f := newDataFakes()
	q := f.queries(t, true)
	if _, err := q.SetConversationData(context.Background(), conversationRef, "turno", "Noite"); !errors.Is(err, conversationvars.ErrTypeInvalid) {
		t.Fatalf("err = %v", err)
	}
	if len(f.store.saved) != 0 || len(f.timeline.events) != 0 {
		t.Fatalf("stored = %v events = %v", f.store.saved, f.timeline.events)
	}
	row, err := q.SetConversationData(context.Background(), conversationRef, "turno", " Tarde ")
	if err != nil {
		t.Fatal(err)
	}
	if f.store.saved["turno"] != "Tarde" || row.Value != "Tarde" || row.Origin != OriginStored {
		t.Fatalf("saved = %v row = %+v", f.store.saved, row)
	}
	if len(f.timeline.events) != 1 || f.timeline.events[0].EventType != ce.EventConversationVariableChanged || f.timeline.events[0].ActorID != "u" {
		t.Fatalf("events = %+v", f.timeline.events)
	}
}

func TestEditingConversationDataRefusesWhatItMustNotTouch(t *testing.T) {
	f := newDataFakes()
	cases := map[string]struct {
		name, value string
		want        error
	}{
		"system key": {"lead_name", "X", conversationvars.ErrNameReserved},
		"unknown":    {"sumiu", "X", conversationvars.ErrNotEditable},
		"empty":      {"valor", " ", conversationvars.ErrValueEmpty},
	}
	for name, c := range cases {
		if _, err := f.queries(t, true).SetConversationData(context.Background(), conversationRef, c.name, c.value); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if _, err := f.queries(t, false).SetConversationData(context.Background(), conversationRef, "valor", "1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("forbidden = %v", err)
	}
	if _, err := f.queries(t, false).ConversationData(context.Background(), conversationRef); !errors.Is(err, ErrForbidden) {
		t.Fatalf("forbidden read = %v", err)
	}
}
