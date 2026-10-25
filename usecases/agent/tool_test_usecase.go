package agent_usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"vozko/domain/agent"
	"vozko/domain/agenttool"
	"vozko/domain/calltrace"
	"vozko/domain/httpcall"
	"vozko/domain/lint"
	"vozko/domain/tools"
	"vozko/domain/tracepayload"
)

var (
	ErrRealCallNotConfirmed = errors.New("agent tool test: confirm the real call before testing a method that changes data")
	ErrTestInputTooLarge    = fmt.Errorf("agent tool test: the arguments are larger than %d bytes", tracepayload.MaxBytes)
	ErrToolTestUnsupported  = errors.New("agent tool test: this tool type is tested through a scenario")
	ErrTestInputMissing     = errors.New("agent tool test: saved test input not found")
)

type ToolTestContext struct {
	EntryID   string
	EntryType string
	LeadID    string
	Vars      map[string]string
}

type ToolTestRequest struct {
	Arguments       map[string]interface{}
	TestInputID     string
	ConfirmRealCall bool
	Context         ToolTestContext
}

type ToolTestResult struct {
	Exchanges    []calltrace.HTTPExchange `json:"exchanges"`
	MappedOutput interface{}              `json:"mappedOutput"`
	ModelResult  string                   `json:"modelResult"`
	IsError      bool                     `json:"isError"`
	ErrorKind    calltrace.ErrorKind      `json:"errorKind,omitempty"`
	ErrorChain   []string                 `json:"errorChain,omitempty"`
	DurationMs   int64                    `json:"durationMs"`
	Issues       []agenttool.Issue        `json:"issues"`
	Preview      bool                     `json:"preview"`
}

type ToolTester interface {
	Test(ctx context.Context, agentID, toolID string, req ToolTestRequest) (ToolTestResult, error)
	TestDraft(ctx context.Context, agentID string, draft agent.ToolBinding, req ToolTestRequest) (ToolTestResult, error)
}

type toolTester struct {
	agents   agent.Repository
	registry tools.Service
}

func NewToolTester(agents agent.Repository, registry tools.Service) ToolTester {
	return &toolTester{agents: agents, registry: registry}
}

func (t *toolTester) Test(ctx context.Context, agentID, toolID string, req ToolTestRequest) (ToolTestResult, error) {
	a, err := t.load(agentID)
	if err != nil {
		return ToolTestResult{}, err
	}
	i := indexOf(a.InternalTools, toolID)
	if i < 0 {
		return ToolTestResult{}, fmt.Errorf("%w: %s", agenttool.ErrToolNotFound, toolID)
	}
	return t.run(ctx, a, a.InternalTools[i], req)
}

func (t *toolTester) TestDraft(ctx context.Context, agentID string, draft agent.ToolBinding, req ToolTestRequest) (ToolTestResult, error) {
	a, err := t.load(agentID)
	if err != nil {
		return ToolTestResult{}, err
	}
	defs := definitionsByName(t.registry)
	def, ok := defs[strings.ToLower(strings.TrimSpace(draft.Name))]
	if !ok {
		return ToolTestResult{}, fmt.Errorf("%w: %s", agenttool.ErrTypeUnknown, draft.Name)
	}
	draft.Name = def.Name
	if len(draft.Config) > 0 {
		normalized, err := setRulesFor(t.registry, defs).Normalize(def, draft.Config)
		if err != nil {
			return ToolTestResult{}, err
		}
		draft.Config = normalized
	}
	if draft.Function == "" {
		draft.Function = def.LegacyName(draft.Config)
	}
	if draft.ID == "" {
		draft.ID = "draft"
	}
	return t.run(ctx, a, draft, req)
}

func (t *toolTester) run(ctx context.Context, a *agent.Agent, b agent.ToolBinding, req ToolTestRequest) (ToolTestResult, error) {
	defs := definitionsByName(t.registry)
	def, ok := defs[strings.ToLower(b.Name)]
	if !ok {
		return ToolTestResult{}, fmt.Errorf("%w: %s", agenttool.ErrTypeUnknown, b.Name)
	}
	args, err := testArguments(b, req)
	if err != nil {
		return ToolTestResult{}, err
	}
	report := agenttool.Check(agenttool.Subject{Tools: []agent.ToolBinding{b}, Definitions: defs})
	if blocking := report.Blocking(); len(blocking) > 0 {
		return ToolTestResult{Issues: blocking, IsError: true, Exchanges: []calltrace.HTTPExchange{}}, nil
	}
	handler, ok := t.registry.Handler(strings.ToLower(def.Name))
	if !ok {
		return ToolTestResult{}, fmt.Errorf("%w: %s", agenttool.ErrTypeUnknown, b.Name)
	}
	config := seededConfig(a, b, req.Context)
	callCtx := tools.WithInstance(tools.WithCallID(ctx, "test_"+uuid.NewString()), &tools.InstanceRef{ID: b.ID, Type: def.Name, Revision: b.Revision})
	started := time.Now()
	var res tools.ExecutionResult
	preview := false
	switch agenttool.TestModeOf(def.Name, handler) {
	case agenttool.TestModeRun:
		if !httpcall.ReadOnlyMethod(agenttool.ValueAt(b.Config, "method", "")) && !req.ConfirmRealCall {
			return ToolTestResult{}, ErrRealCallNotConfirmed
		}
		res, err = handler.ExecuteWithConfig(callCtx, config, args)
	case agenttool.TestModePreview:
		previewer, canPreview := handler.(tools.Previewer)
		if !canPreview {
			return ToolTestResult{}, ErrToolTestUnsupported
		}
		preview = true
		res, err = previewer.Preview(callCtx, config, args)
	default:
		return ToolTestResult{}, ErrToolTestUnsupported
	}
	if err != nil {
		res = tools.ExecutionResult{Result: err.Error(), IsError: true, Trace: &calltrace.ToolTrace{ErrorKind: calltrace.ErrorKindInternal, ErrorChain: calltrace.ErrorChain(err)}}
	}
	return resultOf(res, time.Since(started), report, preview), nil
}

func testArguments(b agent.ToolBinding, req ToolTestRequest) (map[string]interface{}, error) {
	args := req.Arguments
	if req.TestInputID != "" {
		found := false
		for _, in := range b.TestInputs {
			if in.ID == req.TestInputID {
				args, found = in.Input, true
			}
		}
		if !found {
			return nil, ErrTestInputMissing
		}
	}
	if args == nil {
		args = map[string]interface{}{}
	}
	raw, err := json.Marshal(args)
	if err != nil || len(raw) > tracepayload.MaxBytes {
		return nil, ErrTestInputTooLarge
	}
	return args, nil
}

func seededConfig(a *agent.Agent, b agent.ToolBinding, c ToolTestContext) map[string]interface{} {
	config := make(map[string]interface{}, len(b.Config)+6)
	for k, v := range b.Config {
		config[k] = v
	}
	config["__workspace_id"] = a.WorkspaceID
	config["__agent_id"] = a.ID
	if c.EntryID != "" {
		config["__entry_id"], config["__entry_type"] = c.EntryID, c.EntryType
	}
	if c.LeadID != "" {
		config["__lead_id"] = c.LeadID
	}
	if len(c.Vars) > 0 {
		config["__vars"] = c.Vars
	}
	return config
}

func resultOf(res tools.ExecutionResult, took time.Duration, report agenttool.Report, preview bool) ToolTestResult {
	out := ToolTestResult{
		ModelResult: res.ModelPayload(),
		IsError:     res.IsError,
		DurationMs:  took.Milliseconds(),
		Issues:      advisoryOf(report),
		Exchanges:   []calltrace.HTTPExchange{},
		Preview:     preview,
	}
	if res.Trace != nil {
		if res.Trace.Exchanges != nil {
			out.Exchanges = res.Trace.Exchanges
		}
		out.MappedOutput = res.Trace.MappedOutput
		out.ErrorKind = res.Trace.ErrorKind
		out.ErrorChain = res.Trace.ErrorChain
	}
	return out
}

func advisoryOf(report agenttool.Report) []agenttool.Issue {
	out := []agenttool.Issue{}
	for _, i := range report.Issues {
		if i.Severity != lint.SeverityBlocking {
			out = append(out, i)
		}
	}
	return out
}

func (t *toolTester) load(agentID string) (*agent.Agent, error) {
	a, err := t.agents.FindByID(agentID)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, agent.ErrAgentNotFound
	}
	return a, nil
}
