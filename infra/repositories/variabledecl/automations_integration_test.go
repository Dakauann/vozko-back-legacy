package variabledecl_repository

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"vozko/domain/automationneeds"
	"vozko/infra/database/schema"
	"vozko/infra/repositories/repotest"
)

func TestPostgresAutomationsReadDraftsPublishedVersionsAndSheetTools(t *testing.T) {
	db := repotest.IsolatedDB(t, "declusers", &schema.Agent{}, &schema.AgentVersion{}, &schema.AgentTool{}, &schema.WorkflowSchema{}, &schema.WorkflowVersionSchema{})
	repotest.DropForeignKeys(t, db)
	ws, other := uuid.NewString(), uuid.NewString()
	cobrador, apagado, flow, outro := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	department := uuid.NewString()
	published := 1
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.Create(&schema.Agent{ID: cobrador, WorkspaceID: ws, DepartmentID: &department, Name: "Cobrador", MessagingPrompt: "Valor {{valor}}", Tags: "[]", Metadata: "{}", MediaIDs: "[]",
		Variables: `[{"name":"turno","defaultValue":"Manhã"}]`, ToolBindings: `{"bindings":[]}`, PublishedVersion: &published}).Error)
	must(db.Create(&schema.AgentVersion{ID: uuid.NewString(), AgentID: cobrador, Version: 1, WorkspaceID: ws, PublishedAt: time.Now(), Snapshot: `{"messagingPrompt":"Antigo {{curso}}","variables":[{"name":"antiga"}]}`}).Error)
	must(db.Create(&schema.AgentTool{ID: uuid.NewString(), WorkspaceID: ws, AgentID: cobrador, Type: "add_sheet_row", Name: "registrar", Config: `{"sheet_id":"tab","sheet_variable":"{{campvars.planilha}}"}`, Visibility: "[]", TestInputs: "[]", Revision: 1}).Error)
	must(db.Create(&schema.Agent{ID: apagado, WorkspaceID: ws, Name: "Apagado", MessagingPrompt: "{{sumiu}}", Tags: "[]", Metadata: "{}", MediaIDs: "[]", Variables: "[]", ToolBindings: `{"bindings":[]}`}).Error)
	must(db.Delete(&schema.Agent{ID: apagado}).Error)
	must(db.Create(&schema.WorkflowSchema{ID: flow, WorkspaceID: ws, Name: "Matrículas", TriggerType: "message_received", TriggerConfig: "{}", Status: "active", PublishedVersion: &published,
		Graph: `{"nodes":[{"id":"n","type":"action_send_text","config":{"text":"Oi {{campvars.matricula}}"}}],"edges":[],"variables":[{"name":"reserva"}]}`}).Error)
	must(db.Create(&schema.WorkflowVersionSchema{ID: uuid.NewString(), WorkflowID: flow, Version: 1, Type: "messages", GraphHash: "h", PublishedAt: time.Now(),
		Graph: `{"nodes":[{"id":"n","type":"action_send_text","config":{"text":"{{campvars.publicada}}"}}],"edges":[]}`}).Error)
	must(db.Create(&schema.WorkflowSchema{ID: outro, WorkspaceID: other, Name: "Outro", TriggerType: "message_received", TriggerConfig: "{}", Graph: `{"nodes":[{"id":"n","type":"action_send_text","config":{"text":"{{campvars.matricula}}"}}],"edges":[]}`}).Error)

	agents, workflows, err := NewAutomations(db).Automations(context.Background(), ws)
	if err != nil {
		t.Fatal(err)
	}
	users := automationneeds.DeclarationUsers(agents, workflows)
	names := make([]string, 0, len(users))
	for name, list := range users {
		owners := make([]string, 0, len(list))
		for _, user := range list {
			owners = append(owners, user.Name)
			if user.Name == "Cobrador" && user.DepartmentID != department {
				t.Fatalf("department = %q", user.DepartmentID)
			}
		}
		names = append(names, name+"="+strings.Join(owners, "+"))
	}
	sort.Strings(names)
	want := "antiga=Cobrador,curso=Cobrador,matricula=Matrículas,planilha=Cobrador,publicada=Matrículas,reserva=Matrículas,turno=Cobrador,valor=Cobrador"
	if strings.Join(names, ",") != want {
		t.Fatalf("users = %s", strings.Join(names, ","))
	}
}

func TestPostgresAutomationsFailClosedOnAnUnreadableGraph(t *testing.T) {
	db := repotest.IsolatedDB(t, "declbad", &schema.Agent{}, &schema.AgentVersion{}, &schema.AgentTool{}, &schema.WorkflowSchema{}, &schema.WorkflowVersionSchema{})
	ws := uuid.NewString()
	if err := db.Create(&schema.WorkflowSchema{ID: uuid.NewString(), WorkspaceID: ws, Name: "Quebrado", TriggerType: "message_received", TriggerConfig: "{}", Graph: `{"nodes":"x"}`}).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewAutomations(db).Automations(context.Background(), ws); err == nil {
		t.Fatal("an unreadable graph was read as no use")
	}
}
