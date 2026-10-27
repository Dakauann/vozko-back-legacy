package automationneeds

import (
	"context"
	"errors"
	"reflect"
	"testing"

	domain "vozko/domain/automationneeds"
	"vozko/domain/campaign"
	"vozko/domain/conversation"
	"vozko/domain/conversationvars"
	"vozko/domain/customfield"
	"vozko/domain/lead"
	"vozko/domain/shared"
)

type fakeSnapshots struct {
	snapshot conversationvars.Snapshot
	err      error
}

func (f *fakeSnapshots) Snapshot(context.Context, string, shared.EntryType) (conversationvars.Snapshot, error) {
	return f.snapshot, f.err
}

type fakeDefinitions struct {
	defs []*customfield.Definition
	err  error
}

func (f fakeDefinitions) ListByObject(string, customfield.ObjectType) ([]*customfield.Definition, error) {
	return f.defs, f.err
}

type fakeSubjects struct {
	leads map[string]*lead.Lead
	asked [][]string
}

func (f *fakeSubjects) Identified(_ context.Context, _ string, ids []string, _ campaign.BindingPlan) (map[string]campaign.BindingSubject, error) {
	f.asked = append(f.asked, ids)
	out := map[string]campaign.BindingSubject{}
	for _, id := range ids {
		if l, ok := f.leads[id]; ok {
			out[id] = campaign.BindingSubject{Lead: l, OwnerName: "Rui"}
		}
	}
	return out, nil
}

type fakeProfiles struct {
	profile conversation.AutomationProfile
	err     error
}

func (f *fakeProfiles) EntryAutomation(string, string) (conversation.AutomationProfile, error) {
	return f.profile, f.err
}

type formFakes struct {
	snapshots *fakeSnapshots
	defs      fakeDefinitions
	subjects  *fakeSubjects
	profiles  *fakeProfiles
	store     *memValueStore
}

func newFormFakes() *formFakes {
	return &formFakes{
		snapshots: &fakeSnapshots{snapshot: conversationvars.Snapshot{LeadID: "lead-1", Values: map[string]string{"turma": "A", "lead_name": "Dakauann Souza"}}},
		defs:      fakeDefinitions{defs: []*customfield.Definition{{Key: "turma", Label: "Turma", ObjectType: customfield.ObjectLead, Type: customfield.TypeText}}},
		subjects:  &fakeSubjects{leads: map[string]*lead.Lead{"lead-1": {ID: "lead-1", Name: "Dakauann Souza", CustomFields: map[string]any{"turma": "C"}}}},
		profiles:  &fakeProfiles{},
		store:     &memValueStore{},
	}
}

func (f *formFakes) queries(t *testing.T, allowed bool) *Queries {
	t.Helper()
	return newQueriesWith(t, &memRecords{}, f.store, allowed, func(d *QueriesDeps) {
		d.Conversations, d.Definitions, d.Subjects, d.Profiles = f.snapshots, f.defs, f.subjects, f.profiles
	})
}

var conversationRef = ConversationRef{WorkspaceID: "ws", UserID: "u", EntryID: "e1", EntryType: shared.EntryTypeWhatsApp}

func agentRef(id string) *conversation.Automation {
	a := ref(conversation.AutomationAgent, id)
	return &a
}

func TestTheFormShowsEveryNeedWithItsCurrentValueTheSuggestionAndTheLeadFields(t *testing.T) {
	f := newFormFakes()
	form, err := f.queries(t, true).VariablesForm(context.Background(), conversationRef, agentRef("escola"))
	if err != nil {
		t.Fatal(err)
	}
	if form.Automation == nil || form.Automation.Name != "Escola" || form.Automation.ID != "escola" {
		t.Fatalf("automation = %+v", form.Automation)
	}
	if len(form.Rows) != 1 || form.Rows[0].Need.Name != "turma" || form.Rows[0].Current != "A" || form.Rows[0].Suggested != campaign.CustomBinding("turma") {
		t.Fatalf("rows = %+v", form.Rows)
	}
	if form.Fields[0].Field.Source != campaign.BindFirstName || form.Fields[0].Value != "Dakauann" {
		t.Fatalf("first field = %+v", form.Fields[0])
	}
	last := form.Fields[len(form.Fields)-1]
	if last.Field.Source != campaign.CustomBinding("turma") || last.Value != "C" {
		t.Fatalf("custom field = %+v", last)
	}
}

func TestTheFormOfAnAutomationWithoutNeedsReadsNoLead(t *testing.T) {
	f := newFormFakes()
	form, err := f.queries(t, true).VariablesForm(context.Background(), conversationRef, agentRef("livre"))
	if err != nil || form.Automation == nil || len(form.Rows) != 0 || len(f.subjects.asked) != 0 {
		t.Fatalf("form = %+v, %v, asked = %v", form, err, f.subjects.asked)
	}
}

func TestWithoutATargetTheFormReadsTheAutomationSwitchingBackOnHandsTo(t *testing.T) {
	f := newFormFakes()
	paused := false
	f.profiles.profile = conversation.AutomationProfile{AgentID: "escola", AgentResponsesEnabled: true, AutomationEnabled: &paused}
	form, err := f.queries(t, true).VariablesForm(context.Background(), conversationRef, nil)
	if err != nil || form.Automation == nil || form.Automation.ID != "escola" || len(form.Rows) != 1 {
		t.Fatalf("form = %+v, %v", form, err)
	}

	f.profiles.profile = conversation.AutomationProfile{}
	form, err = f.queries(t, true).VariablesForm(context.Background(), conversationRef, nil)
	if err != nil || form.Automation != nil || len(form.Rows) != 0 {
		t.Fatalf("nothing to hand back to = %+v, %v", form, err)
	}

	f.profiles.err = errors.New("db down")
	if _, err := f.queries(t, true).VariablesForm(context.Background(), conversationRef, nil); !errors.Is(err, domain.ErrUnreadable) {
		t.Fatalf("err = %v", err)
	}
}

func TestTheFormFailsClosed(t *testing.T) {
	f := newFormFakes()
	if _, err := f.queries(t, false).VariablesForm(context.Background(), conversationRef, agentRef("escola")); !errors.Is(err, ErrForbidden) {
		t.Fatalf("no access = %v", err)
	}
	if _, err := f.queries(t, true).VariablesForm(context.Background(), conversationRef, agentRef("sumiu")); err == nil {
		t.Fatal("an unknown automation must not give an empty form")
	}
	f.snapshots.err = errors.New("db down")
	if _, err := f.queries(t, true).VariablesForm(context.Background(), conversationRef, agentRef("escola")); !errors.Is(err, domain.ErrUnreadable) {
		t.Fatalf("unreadable values = %v", err)
	}
}

func save(t *testing.T, f *formFakes, allowed bool, values map[string]string, bindings ...conversationvars.Binding) error {
	return f.queries(t, allowed).SaveConversationValues(context.Background(), SaveValues{
		ConversationRef: conversationRef, Automation: ref(conversation.AutomationWorkflow, "cobranca"), Values: values, Bindings: bindings,
	})
}

func TestSavingResolvesEachBindingOnTheServerAndStoresTheValues(t *testing.T) {
	f := newFormFakes()
	f.defs.defs = append(f.defs.defs, &customfield.Definition{Key: "valor", ObjectType: customfield.ObjectLead, Type: customfield.TypeText})
	f.subjects.leads["lead-1"].CustomFields["valor"] = "R$ 90"
	if err := save(t, f, true, nil, conversationvars.Binding{Name: "valor", Source: campaign.CustomBinding("valor")}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.store.saved, map[string]string{"valor": "R$ 90"}) || f.store.ref.EntryID != "e1" {
		t.Fatalf("saved = %v ref = %+v", f.store.saved, f.store.ref)
	}
	if err := save(t, f, true, map[string]string{"valor": " 120 "}); err != nil || f.store.saved["valor"] != "120" {
		t.Fatalf("the values map stays a fixed value: %v, %v", f.store.saved, err)
	}
}

func TestKeepingTheCurrentValueStoresNothing(t *testing.T) {
	f := newFormFakes()
	f.snapshots.snapshot.Values["valor"] = "R$ 50"
	if err := save(t, f, true, nil, conversationvars.Binding{Name: "valor", Source: conversationvars.SourceCurrent}); err != nil {
		t.Fatal(err)
	}
	if f.store.calls != 0 || len(f.subjects.asked) != 0 {
		t.Fatalf("calls = %d asked = %v", f.store.calls, f.subjects.asked)
	}
}

func TestSavingRefusesWhatWouldLeaveTheAutomationBlank(t *testing.T) {
	cases := []struct {
		name     string
		allowed  bool
		values   map[string]string
		bindings []conversationvars.Binding
		prepare  func(*formFakes)
		want     error
		names    []string
	}{
		{name: "no access", values: map[string]string{"valor": "1"}, want: ErrForbidden},
		{name: "a variable the workflow does not need", allowed: true, bindings: []conversationvars.Binding{{Name: "outra", Source: campaign.BindLiteral, Value: "A"}}, want: conversationvars.ErrNotNeeded, names: []string{"outra"}},
		{name: "a reserved key", allowed: true, values: map[string]string{"lead_name": "A"}, want: conversationvars.ErrNameReserved},
		{name: "a lead field the lead does not have", allowed: true, bindings: []conversationvars.Binding{{Name: "valor", Source: campaign.BindNickname}}, want: conversationvars.ErrValueEmpty, names: []string{"valor"}},
		{name: "a conversation without a lead", allowed: true, bindings: []conversationvars.Binding{{Name: "valor", Source: campaign.BindFirstName}}, prepare: func(f *formFakes) { f.snapshots.snapshot.LeadID = "" }, want: conversationvars.ErrValueEmpty, names: []string{"valor"}},
		{name: "keeping a value the conversation does not have", allowed: true, bindings: []conversationvars.Binding{{Name: "valor", Source: conversationvars.SourceCurrent}}, want: conversationvars.ErrValueEmpty, names: []string{"valor"}},
		{name: "an unknown source", allowed: true, bindings: []conversationvars.Binding{{Name: "valor", Source: "lead.signo"}}, want: conversationvars.ErrSourceInvalid, names: []string{"valor"}},
		{name: "unreadable lead fields", allowed: true, values: map[string]string{"valor": "1"}, prepare: func(f *formFakes) { f.defs.err = errors.New("db down") }, want: domain.ErrUnreadable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFormFakes()
			if tc.prepare != nil {
				tc.prepare(f)
			}
			err := save(t, f, tc.allowed, tc.values, tc.bindings...)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			var refusal *conversationvars.VariablesError
			if tc.names != nil && (!errors.As(err, &refusal) || !reflect.DeepEqual(refusal.Names, tc.names)) {
				t.Fatalf("err = %v, want the variables %v", err, tc.names)
			}
			if f.store.calls != 0 {
				t.Fatalf("nothing is stored when the form is refused: %v", f.store.saved)
			}
		})
	}
}
