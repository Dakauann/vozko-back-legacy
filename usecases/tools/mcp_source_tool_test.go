package tools_usecase

import (
	"context"
	"errors"
	"testing"

	domainmcp "vozko/domain/agent/mcp"
	"vozko/domain/agenttool"
	"vozko/domain/tools"
)

type fakeSource struct {
	id      string
	tools   []domainmcp.Tool
	listErr error
	called  string
}

func (f *fakeSource) ID() string           { return f.id }
func (f *fakeSource) Kind() domainmcp.Kind { return domainmcp.KindRemote }
func (f *fakeSource) DisplayName() string  { return "CRM" }
func (f *fakeSource) ListTools(context.Context, domainmcp.WorkspaceID) ([]domainmcp.Tool, error) {
	return f.tools, f.listErr
}
func (f *fakeSource) CallTool(_ context.Context, _ domainmcp.WorkspaceID, name string, _ map[string]any) (domainmcp.ToolResult, error) {
	f.called = name
	return domainmcp.TextResult("ok"), nil
}

type sourceCatalog struct {
	sources []domainmcp.ToolSource
	err     error
}

func (c sourceCatalog) Sources(context.Context, domainmcp.WorkspaceID) ([]domainmcp.ToolSource, error) {
	return c.sources, c.err
}

func crmSource() *fakeSource {
	return &fakeSource{id: "remote:crm", tools: []domainmcp.Tool{{Name: "buscar_cliente"}, {Name: "apagar_cliente"}}}
}

func TestAnMCPSourceIsOneInstancePerSource(t *testing.T) {
	def := NewMCPSourceTool(sourceCatalog{}).Definition()
	if def.Instances != tools.PerTarget("source_id") || def.Name != MCPSourceToolName {
		t.Fatalf("definition %+v", def)
	}
}

func TestAnMCPSourceIsNeverExecutedByType(t *testing.T) {
	if _, err := NewMCPSourceTool(sourceCatalog{}).ExecuteWithConfig(context.Background(), nil, nil); err == nil {
		t.Fatal("the source type must refuse a direct call")
	}
}

func TestTheMCPSelectionMustPointAtAConnectedSourceAndExistingTools(t *testing.T) {
	v := NewMCPSourceTool(sourceCatalog{sources: []domainmcp.ToolSource{crmSource()}}).(tools.BindingValidator)
	ok := []map[string]interface{}{
		{"source_id": "remote:crm", "all_tools": true},
		{"source_id": "remote:crm", "tools": []interface{}{"buscar_cliente"}},
	}
	for _, config := range ok {
		if err := v.ValidateBinding(context.Background(), "ws-1", config); err != nil {
			t.Fatalf("%v: %v", config, err)
		}
	}
	bad := []map[string]interface{}{
		{},
		{"source_id": "remote:crm"},
		{"source_id": "remote:outro", "all_tools": true},
		{"source_id": "remote:crm", "tools": []interface{}{"sumiu"}},
	}
	for _, config := range bad {
		problem, isProblem := tools.ProblemOf(v.ValidateBinding(context.Background(), "ws-1", config))
		if !isProblem || problem.Code != agenttool.CodeMCPSourceInvalid {
			t.Fatalf("%v: %+v", config, problem)
		}
	}
}

func TestAnUnreachableMCPCatalogIsAnOutageNotAProblem(t *testing.T) {
	for _, catalog := range []sourceCatalog{{err: errors.New("db down")}, {sources: []domainmcp.ToolSource{&fakeSource{id: "remote:crm", listErr: errors.New("timeout")}}}} {
		err := NewMCPSourceTool(catalog).(tools.BindingValidator).ValidateBinding(context.Background(), "ws-1", map[string]interface{}{"source_id": "remote:crm", "tools": []interface{}{"x"}})
		if _, isProblem := tools.ProblemOf(err); err == nil || isProblem {
			t.Fatalf("err %v", err)
		}
	}
}

func TestTheSelectionAllowsOnlyTheChosenTools(t *testing.T) {
	selection := MCPSelectionOf(map[string]interface{}{"source_id": " remote:crm ", "tools": []interface{}{"buscar_cliente", "", 3}})
	if selection.SourceID != "remote:crm" || !selection.Allows("buscar_cliente") || selection.Allows("apagar_cliente") {
		t.Fatalf("selection %+v", selection)
	}
	if !MCPSelectionOf(map[string]interface{}{"all_tools": true}).Allows("qualquer") {
		t.Fatal("all tools allows every tool")
	}
}

func TestListingMCPSourcesShowsToolsAndMarksASourceThatDoesNotAnswer(t *testing.T) {
	views, err := ListMCPSources(context.Background(), sourceCatalog{sources: []domainmcp.ToolSource{crmSource(), &fakeSource{id: "remote:off", listErr: errors.New("timeout")}}}, "ws-1")
	if err != nil || len(views) != 2 {
		t.Fatalf("views %+v err %v", views, err)
	}
	if views[0].ID != "remote:crm" || len(views[0].Tools) != 2 || views[0].Unavailable || !views[1].Unavailable || views[1].Tools == nil {
		t.Fatalf("views %+v", views)
	}
	if _, err := ListMCPSources(context.Background(), sourceCatalog{err: errors.New("db down")}, "ws-1"); err == nil {
		t.Fatal("a catalog outage is an error")
	}
}
