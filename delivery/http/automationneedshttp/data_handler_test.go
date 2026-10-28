package automationneedshttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"vozko/domain/conversationvars"
	workspace_domain "vozko/domain/workspace"
	automationneeds_usecase "vozko/usecases/automationneeds"
)

func (f *fakeQueries) ConversationData(context.Context, automationneeds_usecase.ConversationRef) ([]automationneeds_usecase.DataRow, error) {
	return f.data, f.err
}

func (f *fakeQueries) SetConversationData(_ context.Context, _ automationneeds_usecase.ConversationRef, name, value string) (automationneeds_usecase.DataRow, error) {
	f.set = append(f.set, name+"="+value)
	return automationneeds_usecase.DataRow{Name: name, Label: "Turno", Value: value, Origin: automationneeds_usecase.OriginStored, Fits: true}, f.err
}

func TestTheConversationDataRoutesAnswerAndEditWithTheConversationPermission(t *testing.T) {
	q := &fakeQueries{data: []automationneeds_usecase.DataRow{{Name: "lead_name", Label: "Nome do contato", LabelKey: "conversation_lead_name", Value: "Ana", Origin: automationneeds_usecase.OriginSystem, Fits: true}}}
	rec := serve(t, q, http.MethodGet, "/conversations/whatsapp/e1/variable-values", "")
	var got DataResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got.Values) != 1 || got.Values[0].LabelKey != "conversation_lead_name" || got.Values[0].Origin != "system" {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	rec = serve(t, q, http.MethodPut, "/conversations/whatsapp/e1/variable-values/turno", `{"value":"Tarde"}`)
	if rec.Code != http.StatusOK || len(q.set) != 1 || q.set[0] != "turno=Tarde" {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	asked := map[string]bool{}
	RegisterRoutes(mux.NewRouter(), NewHandler(q), func(r workspace_domain.Resource, a workspace_domain.Action, next http.HandlerFunc) http.HandlerFunc {
		asked[string(r)+":"+string(a)] = true
		return next
	})
	if !asked["conversations:update"] || !asked["workflows:read"] {
		t.Fatalf("asked = %v", asked)
	}
	q.err = &conversationvars.VariablesError{Cause: conversationvars.ErrNotEditable, Names: []string{"x"}}
	if rec := serve(t, q, http.MethodPut, "/conversations/whatsapp/e1/variable-values/x", `{"value":"1"}`); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "conversation_variable_not_editable") {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}
