package automationneedshttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"vozko/domain/auth"
	domain "vozko/domain/automationneeds"
	"vozko/domain/campaign"
	"vozko/domain/campaignaudience"
	"vozko/domain/conversation"
	"vozko/domain/conversationvars"
	workspace_domain "vozko/domain/workspace"
	"vozko/infra/http/middleware"
	automationneeds_usecase "vozko/usecases/automationneeds"
)

type fakeQueries struct {
	asked  []conversation.Automation
	form   automationneeds_usecase.VariablesForm
	formOf []*conversation.Automation
	open   *automationneeds_usecase.OpenSwitchOff
	saved  []automationneeds_usecase.SaveValues
	err    error
}

func (f *fakeQueries) Eligibility(_ string, asked []conversation.Automation) ([]automationneeds_usecase.Eligibility, error) {
	f.asked = asked
	if len(asked) > 100 {
		return nil, automationneeds_usecase.ErrTooMany
	}
	out := []automationneeds_usecase.Eligibility{}
	for _, a := range asked {
		out = append(out, automationneeds_usecase.Eligibility{Automation: domain.Link{Automation: a, Name: "X"}, Needs: domain.Needs{{Name: "turma"}}, Readable: true})
	}
	return out, nil
}

func (f *fakeQueries) PublishImpact(context.Context, string, conversation.Automation) ([]domain.SwitchOff, error) {
	return []domain.SwitchOff{{Slot: domain.Slot{Kind: domain.SlotInstagram, ID: "ig", Name: "@loja"}}}, nil
}

func (f *fakeQueries) OpenSwitchOff(context.Context, string, domain.SlotKind, string) (*automationneeds_usecase.OpenSwitchOff, error) {
	return f.open, nil
}

func (f *fakeQueries) SaveConversationValues(_ context.Context, in automationneeds_usecase.SaveValues) error {
	f.saved = append(f.saved, in)
	return f.err
}

func (f *fakeQueries) VariablesForm(_ context.Context, _ automationneeds_usecase.ConversationRef, target *conversation.Automation) (automationneeds_usecase.VariablesForm, error) {
	f.formOf = append(f.formOf, target)
	return f.form, f.err
}

func allow(_ workspace_domain.Resource, _ workspace_domain.Action, next http.HandlerFunc) http.HandlerFunc {
	return next
}

func serve(t *testing.T, q *fakeQueries, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	router := mux.NewRouter()
	RegisterRoutes(router, NewHandler(q), allow)
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	ctx := context.WithValue(req.Context(), middleware.WorkspaceIDContextKey, "ws-1")
	ctx = context.WithValue(ctx, middleware.ClaimsContextKey, &auth.Claims{UserID: "u-1"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req.WithContext(ctx))
	return rec
}

func TestEligibilityListsEveryAskedAutomation(t *testing.T) {
	q := &fakeQueries{}
	rec := serve(t, q, http.MethodGet, "/automation-needs?agents=a1,%20a2&workflows=w1", "")
	if rec.Code != http.StatusOK || len(q.asked) != 3 || q.asked[1].ID != "a2" || q.asked[2].Kind != conversation.AutomationWorkflow {
		t.Fatalf("code = %d asked = %+v", rec.Code, q.asked)
	}
	var body EligibilityListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if len(body.Items) != 3 || body.Items[0].ReceptiveEligible || body.Items[0].Variables[0].Name != "turma" {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestTheChannelReadsANullNoticeWhenNothingWasSwitchedOff(t *testing.T) {
	rec := serve(t, &fakeQueries{}, http.MethodGet, "/automation-switch-offs?channel=instagram&id=ig", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"notice":null`) {
		t.Fatalf("code = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestConversationValuesAreSavedOrRefusedWithACode(t *testing.T) {
	q := &fakeQueries{}
	body := `{"automation":{"kind":"agent","id":"a1"},"values":{"turma":"B"}}`
	if rec := serve(t, q, http.MethodPut, "/conversations/instagram/e1/variables", body); rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d body = %s", rec.Code, rec.Body.String())
	}
	if len(q.saved) != 1 || q.saved[0].Values["turma"] != "B" || q.saved[0].Automation.ID != "a1" || q.saved[0].UserID != "u-1" {
		t.Fatalf("saved = %+v", q.saved)
	}
	q.err = conversationvars.ErrNotNeeded
	rec := serve(t, q, http.MethodPut, "/conversations/instagram/e1/variables", body)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "conversation_variable_not_needed") {
		t.Fatalf("code = %d body = %s", rec.Code, rec.Body.String())
	}
	if rec := serve(t, q, http.MethodPut, "/conversations/fax/e1/variables", body); rec.Code != http.StatusBadRequest {
		t.Fatalf("an unknown entry type = %d", rec.Code)
	}
}

func TestConversationBindingsReachTheUseCaseAndRefusalsNameTheRows(t *testing.T) {
	q := &fakeQueries{}
	body := `{"automation":{"kind":"workflow","id":"w1"},"bindings":[{"name":"nome_cliente","source":"lead.first_name"},{"name":"matricula","source":"literal","value":"17"},{"name":"turma","source":"conversation"}]}`
	if rec := serve(t, q, http.MethodPut, "/conversations/whatsapp/e1/variables", body); rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d body = %s", rec.Code, rec.Body.String())
	}
	want := []conversationvars.Binding{
		{Name: "nome_cliente", Source: campaign.BindFirstName},
		{Name: "matricula", Source: campaign.BindLiteral, Value: "17"},
		{Name: "turma", Source: conversationvars.SourceCurrent},
	}
	if len(q.saved) != 1 || !reflect.DeepEqual(q.saved[0].Bindings, want) || q.saved[0].Automation.Kind != conversation.AutomationWorkflow {
		t.Fatalf("saved = %+v", q.saved)
	}
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{&conversationvars.VariablesError{Cause: conversationvars.ErrValueEmpty, Names: []string{"matricula"}}, http.StatusUnprocessableEntity, "conversation_variable_empty"},
		{&conversationvars.VariablesError{Cause: conversationvars.ErrSourceInvalid, Names: []string{"matricula"}}, http.StatusBadRequest, "conversation_variable_source_invalid"},
		{&conversationvars.VariablesError{Cause: conversationvars.ErrNotNeeded, Names: []string{"matricula"}}, http.StatusBadRequest, "conversation_variable_not_needed"},
	}
	for _, tc := range cases {
		q.err = tc.err
		rec := serve(t, q, http.MethodPut, "/conversations/whatsapp/e1/variables", body)
		var got struct {
			Code    string `json:"code"`
			Details struct {
				Variables []string `json:"variables"`
			} `json:"details"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		if rec.Code != tc.status || got.Code != tc.code || !reflect.DeepEqual(got.Details.Variables, []string{"matricula"}) {
			t.Fatalf("code = %d body = %s", rec.Code, rec.Body.String())
		}
	}
}

func TestTheFormOfAConversationListsRowsAndLeadFields(t *testing.T) {
	q := &fakeQueries{form: automationneeds_usecase.VariablesForm{
		Automation: &domain.Link{Automation: conversation.Automation{Kind: conversation.AutomationWorkflow, ID: "w1"}, Name: "Altiva: cobrança"},
		Rows: []automationneeds_usecase.VariableRow{{
			Need:      domain.Need{Name: "matricula", Via: []domain.Link{{Automation: conversation.Automation{Kind: conversation.AutomationWorkflow, ID: "w1"}, Name: "Altiva: cobrança"}}},
			Current:   "",
			Suggested: campaign.CustomBinding("matricula"),
		}},
		Fields: []automationneeds_usecase.FieldValue{{Field: campaignaudience.Field{Source: campaign.BindFirstName, Kind: campaignaudience.FieldLead, Column: "primeiro_nome"}, Value: "Dakauann"}},
	}}
	rec := serve(t, q, http.MethodGet, "/conversations/whatsapp/e1/variables?kind=workflow&id=w1", "")
	if rec.Code != http.StatusOK || len(q.formOf) != 1 || q.formOf[0] == nil || q.formOf[0].ID != "w1" {
		t.Fatalf("code = %d body = %s asked = %+v", rec.Code, rec.Body.String(), q.formOf)
	}
	var body ConversationVariablesResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Automation == nil || body.Automation.Name != "Altiva: cobrança" || body.Variables[0].Name != "matricula" || body.Variables[0].Suggested != "lead.custom:matricula" || body.Variables[0].Via[0].Name != "Altiva: cobrança" {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if body.Fields[0].Source != "lead.first_name" || body.Fields[0].Value != "Dakauann" {
		t.Fatalf("fields = %s", rec.Body.String())
	}

	q.form = automationneeds_usecase.VariablesForm{}
	rec = serve(t, q, http.MethodGet, "/conversations/whatsapp/e1/variables", "")
	if rec.Code != http.StatusOK || q.formOf[1] != nil || !strings.Contains(rec.Body.String(), `"automation":null`) || !strings.Contains(rec.Body.String(), `"variables":[]`) {
		t.Fatalf("without a target the backend picks the automation switching back on hands to: %d %s", rec.Code, rec.Body.String())
	}
	if rec := serve(t, q, http.MethodGet, "/conversations/whatsapp/e1/variables?kind=workflow", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("a kind without an id = %d", rec.Code)
	}
	q.err = automationneeds_usecase.ErrForbidden
	if rec := serve(t, q, http.MethodGet, "/conversations/whatsapp/e1/variables?kind=workflow&id=w1", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("no access = %d", rec.Code)
	}
}
