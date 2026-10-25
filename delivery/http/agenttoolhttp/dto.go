package agenttoolhttp

import (
	"time"

	"vozko/domain/agent"
	"vozko/domain/agenttool"
	"vozko/domain/calltrace"
	"vozko/domain/testinput"
)

type ToolResponse struct {
	ID          string                 `json:"id" example:"5b0c1d4e-9a7f-4f4a-8a52-3b7e1e2f9c10"`
	Type        string                 `json:"type" example:"http_request"`
	Name        string                 `json:"name" example:"consultar_status_do_pedido"`
	Description string                 `json:"description" example:"Consulta o status de um pedido pelo número que o cliente informou."`
	Config      map[string]interface{} `json:"config"`
	Visibility  []string               `json:"visibility"`
	Position    int                    `json:"position" example:"0"`
	Revision    int                    `json:"revision" example:"3"`
	InputSchema map[string]interface{} `json:"inputSchema"`
	TestInputs  []testinput.TestInput  `json:"testInputs"`
	TestMode    agenttool.TestMode     `json:"testMode" enums:"run,preview,scenario" example:"run"`
}

type ToolListResponse struct {
	Tools  []ToolResponse    `json:"tools"`
	Green  bool              `json:"green"`
	Issues []agenttool.Issue `json:"issues"`
}

type CreateToolRequest struct {
	Type        string                 `json:"type" example:"http_request"`
	Name        string                 `json:"name" example:"consultar_status_do_pedido"`
	Description string                 `json:"description" example:"Consulta o status de um pedido pelo número que o cliente informou."`
	Config      map[string]interface{} `json:"config"`
	Visibility  []string               `json:"visibility"`
}

type UpdateToolRequest struct {
	Name        string                 `json:"name" example:"consultar_status_do_pedido"`
	Description string                 `json:"description" example:"Consulta o status de um pedido pelo número que o cliente informou."`
	Config      map[string]interface{} `json:"config"`
	Visibility  []string               `json:"visibility"`
	Revision    int                    `json:"revision" example:"3"`
}

type OrderRequest struct {
	IDs []string `json:"ids"`
}

type RevisionResponse struct {
	ToolID      string                 `json:"toolId"`
	Revision    int                    `json:"revision" example:"2"`
	Type        string                 `json:"type" example:"http_request"`
	Name        string                 `json:"name" example:"consultar_status_do_pedido"`
	Description string                 `json:"description"`
	Config      map[string]interface{} `json:"config"`
	Visibility  []string               `json:"visibility"`
	SavedBy     string                 `json:"savedBy"`
	SavedAt     time.Time              `json:"savedAt"`
}

type RevisionListResponse struct {
	Revisions []RevisionResponse `json:"revisions"`
}

func toolResponse(b agent.ToolBinding, position int, inputSchema map[string]interface{}, testMode agenttool.TestMode) ToolResponse {
	config := b.Config
	if config == nil {
		config = map[string]interface{}{}
	}
	if inputSchema == nil {
		inputSchema = map[string]interface{}{"type": "object"}
	}
	testInputs := b.TestInputs
	if testInputs == nil {
		testInputs = []testinput.TestInput{}
	}
	return ToolResponse{
		ID: b.ID, Type: b.Name, Name: b.Function, Description: b.Description, Config: config,
		Visibility: visibilityStrings(b.Visibility), Position: position, Revision: b.Revision,
		InputSchema: inputSchema, TestInputs: testInputs, TestMode: testMode,
	}
}

func revisionResponse(r agent.ToolRevision) RevisionResponse {
	config := r.Config
	if config == nil {
		config = map[string]interface{}{}
	}
	return RevisionResponse{
		ToolID: r.ToolID, Revision: r.Revision, Type: r.Type, Name: r.Name, Description: r.Description,
		Config: config, Visibility: visibilityStrings(r.Visibility), SavedBy: r.SavedBy, SavedAt: r.SavedAt,
	}
}

func visibilityStrings(in []agent.ToolVisibility) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		out = append(out, string(v))
	}
	return out
}

func visibilityOf(in []string) []agent.ToolVisibility {
	if len(in) == 0 {
		return nil
	}
	out := make([]agent.ToolVisibility, 0, len(in))
	for _, v := range in {
		out = append(out, agent.ToolVisibility(v))
	}
	return out
}

type DraftTool struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type" example:"http_request"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Config      map[string]interface{} `json:"config"`
	Visibility  []string               `json:"visibility"`
}

type ValidateRequest struct {
	Tools []DraftTool `json:"tools"`
}

type ValidationResponse struct {
	Green  bool              `json:"green"`
	Issues []agenttool.Issue `json:"issues"`
}

func validationResponse(report agenttool.Report) ValidationResponse {
	issues := report.Issues
	if issues == nil {
		issues = []agenttool.Issue{}
	}
	return ValidationResponse{Green: report.IsGreen(), Issues: issues}
}

func draftBindings(tools []DraftTool) []agent.ToolBinding {
	if tools == nil {
		return nil
	}
	out := make([]agent.ToolBinding, 0, len(tools))
	for _, t := range tools {
		out = append(out, agent.ToolBinding{ID: t.ID, Name: t.Type, Function: t.Name, Description: t.Description, Config: t.Config, Visibility: visibilityOf(t.Visibility)})
	}
	return out
}

type TestContext struct {
	EntryID   string            `json:"entryId"`
	EntryType string            `json:"entryType"`
	LeadID    string            `json:"leadId"`
	Vars      map[string]string `json:"vars"`
}

type TestRequest struct {
	Arguments       map[string]interface{} `json:"arguments"`
	TestInputID     string                 `json:"testInputId"`
	ConfirmRealCall bool                   `json:"confirmRealCall"`
	Context         TestContext            `json:"context"`
}

type DraftTestRequest struct {
	Tool            DraftTool              `json:"tool"`
	Arguments       map[string]interface{} `json:"arguments"`
	ConfirmRealCall bool                   `json:"confirmRealCall"`
	Context         TestContext            `json:"context"`
}

type TestResponse struct {
	Exchanges    []calltrace.HTTPExchange `json:"exchanges"`
	MappedOutput interface{}              `json:"mappedOutput"`
	ModelResult  string                   `json:"modelResult"`
	IsError      bool                     `json:"isError"`
	ErrorKind    string                   `json:"errorKind,omitempty"`
	ErrorChain   []string                 `json:"errorChain,omitempty"`
	DurationMs   int64                    `json:"durationMs"`
	Issues       []agenttool.Issue        `json:"issues"`
	Preview      bool                     `json:"preview"`
}

type TestInputsRequest struct {
	TestInputs []testinput.TestInput `json:"testInputs"`
}

type TestInputsResponse struct {
	TestInputs []testinput.TestInput `json:"testInputs"`
}
