package database

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"vozko/infra/database/schema"
	"vozko/infra/repositories/repotest"
)

func agentRow(id, ws, variables string) *schema.Agent {
	return &schema.Agent{ID: id, WorkspaceID: ws, Name: "Agente " + id[:4], MessagingPrompt: "Oi {{lead_name}}", Tags: "[]", Metadata: "{}", MediaIDs: "[]", Variables: variables, ToolBindings: `{"bindings":[]}`}
}

func storedVariables(t *testing.T, raw string) []map[string]string {
	t.Helper()
	var out []map[string]string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("variables %q: %v", raw, err)
	}
	return out
}

func TestPostgresPlatformNamesLeaveEveryAgentListAndStoredVersionOnce(t *testing.T) {
	db := repotest.IsolatedDB(t, "agvars", &schema.Agent{}, &schema.AgentVersion{})
	ws := uuid.NewString()
	legacy, clean, empty, nullVars := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.Create(agentRow(legacy, ws, `[{"name":"turno","defaultValue":"Manhã"},{"name":"lead_name","defaultValue":"cliente"},{"name":" campaign_name "}]`)).Error)
	must(db.Create(agentRow(clean, ws, `[{"name":"matricula","description":"Número"}]`)).Error)
	must(db.Create(agentRow(empty, ws, `[]`)).Error)
	must(db.Create(agentRow(nullVars, ws, `null`)).Error)
	oldVersion := uuid.NewString()
	newVersion := uuid.NewString()
	noVariables := uuid.NewString()
	must(db.Create(&schema.AgentVersion{ID: oldVersion, AgentID: legacy, Version: 1, WorkspaceID: ws, PublishedAt: time.Now(),
		Snapshot: `{"name":"Agente","messagingPrompt":"Oi {{lead_name}}","tools":[{"id":"t1","revision":2}],"variables":[{"name":"lead_number","defaultValue":"0"},{"name":"turno"}],"futureField":{"kept":true}}`}).Error)
	must(db.Create(&schema.AgentVersion{ID: newVersion, AgentID: clean, Version: 1, WorkspaceID: ws, PublishedAt: time.Now(),
		Snapshot: `{"name":"Limpo","variables":[{"name":"matricula"}]}`}).Error)
	must(db.Create(&schema.AgentVersion{ID: noVariables, AgentID: empty, Version: 1, WorkspaceID: ws, PublishedAt: time.Now(),
		Snapshot: `{"name":"Antigo"}`}).Error)

	for run := 0; run < 2; run++ {
		if err := removePlatformAgentVariables(db); err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
	}

	var agents []schema.Agent
	must(db.Order("id").Find(&agents).Error)
	got := map[string]string{}
	for _, a := range agents {
		got[a.ID] = a.Variables
	}
	if vars := storedVariables(t, got[legacy]); !reflect.DeepEqual(vars, []map[string]string{{"name": "turno", "defaultValue": "Manhã"}}) {
		t.Fatalf("legacy = %v", vars)
	}
	if vars := storedVariables(t, got[clean]); !reflect.DeepEqual(vars, []map[string]string{{"name": "matricula", "description": "Número"}}) {
		t.Fatalf("clean = %v", vars)
	}
	if strings.TrimSpace(got[empty]) != "[]" || strings.TrimSpace(got[nullVars]) != "null" {
		t.Fatalf("untouched = %q %q", got[empty], got[nullVars])
	}

	var versions []schema.AgentVersion
	must(db.Find(&versions).Error)
	snapshots := map[string]map[string]interface{}{}
	for _, v := range versions {
		var snapshot map[string]interface{}
		must(json.Unmarshal([]byte(v.Snapshot), &snapshot))
		snapshots[v.ID] = snapshot
	}
	old := snapshots[oldVersion]
	if vars, _ := json.Marshal(old["variables"]); string(vars) != `[{"name":"turno"}]` {
		t.Fatalf("old version variables = %s", vars)
	}
	if old["messagingPrompt"] != "Oi {{lead_name}}" || old["futureField"] == nil || old["tools"] == nil {
		t.Fatalf("the rest of the snapshot must stay: %v", old)
	}
	if vars, _ := json.Marshal(snapshots[newVersion]["variables"]); string(vars) != `[{"name":"matricula"}]` {
		t.Fatalf("clean version = %s", vars)
	}
	if _, has := snapshots[noVariables]["variables"]; has {
		t.Fatalf("a snapshot without variables gains none: %v", snapshots[noVariables])
	}
}

func TestPostgresPlatformNameRepairSkipsWhenAgentTablesAreMissing(t *testing.T) {
	db := repotest.IsolatedDB(t, "agvarsnone")
	if err := removePlatformAgentVariables(db); err != nil {
		t.Fatal(err)
	}
}
