package automationtransfer_usecase

import (
	"context"
	"errors"
	"time"

	"vozko/domain/agent"
	"vozko/domain/automationneeds"
	"vozko/domain/automationtransfer"
	"vozko/domain/campaign"
	"vozko/domain/conversation"
	ce "vozko/domain/conversation_event"
	"vozko/domain/conversationvars"
	"vozko/domain/customfield"
	"vozko/domain/shared"
	"vozko/domain/stage"
	"vozko/domain/workflow"
	ia_usecase "vozko/usecases/inbox_assignment"
)

var published = 1

type agentBook map[string]*agent.Agent

func (b agentBook) FindByID(id string) (*agent.Agent, error) {
	if id == "broken" {
		return nil, errors.New("connection reset")
	}
	if found, ok := b[id]; ok {
		return found, nil
	}
	return nil, agent.ErrAgentNotFound
}

type workflowBook map[string]*workflow.Workflow

func (b workflowBook) FindByID(id string) (*workflow.Workflow, error) {
	if id == "broken" {
		return nil, errors.New("connection reset")
	}
	return b[id], nil
}

type governance struct {
	governing conversation.Automation
	governed  bool
	err       error
}

func (g *governance) Governing(string, string, shared.EntryType) (conversation.Automation, bool, error) {
	return g.governing, g.governed, g.err
}

type delegations struct {
	checkErr    error
	delegateErr error
	checked     []conversation.Automation
	delegated   []ia_usecase.SystemDelegation
	order       *[]string
}

func (d *delegations) CheckDelegation(_ context.Context, in ia_usecase.SystemDelegation) error {
	d.checked = append(d.checked, in.Automation)
	return d.checkErr
}

func (d *delegations) DelegateAsSystem(_ context.Context, in ia_usecase.SystemDelegation) error {
	*d.order = append(*d.order, "delegate")
	if d.delegateErr != nil {
		return d.delegateErr
	}
	d.delegated = append(d.delegated, in)
	return nil
}

type stages struct {
	byID     map[string]stage.FunnelStage
	current  stage.FunnelStage
	moveErr  error
	moves    []stage.AssignEntryStageInput
	removed  []stage.RemoveEntryStageInput
	readErr  error
	order    *[]string
	failBack bool
}

func (s *stages) Destination(_ string, stageID string) (stage.FunnelStage, error) {
	if s.readErr != nil {
		return stage.FunnelStage{}, s.readErr
	}
	found, ok := s.byID[stageID]
	if !ok {
		return stage.FunnelStage{}, stage.ErrTagNotFound
	}
	return found, nil
}

func (s *stages) Move(_ string, in stage.AssignEntryStageInput) (stage.Move, error) {
	*s.order = append(*s.order, "move:"+in.StageID)
	if s.moveErr != nil || (s.failBack && len(s.moves) > 0) {
		return stage.Move{}, errors.Join(s.moveErr, errors.New("move failed"))
	}
	s.moves = append(s.moves, in)
	from := s.current
	s.current = s.byID[in.StageID]
	return stage.Move{From: from, To: s.current}, nil
}

func (s *stages) GetEntryStage(string, string, string) (*stage.EntryStage, error) {
	if s.current.StageID == "" {
		return nil, stage.ErrEntryTagNotFound
	}
	return &stage.EntryStage{StageID: s.current.StageID}, nil
}

func (s *stages) Execute(_ string, in stage.RemoveEntryStageInput) error {
	*s.order = append(*s.order, "remove:"+in.StageID)
	s.removed = append(s.removed, in)
	s.current = stage.FunnelStage{}
	return nil
}

type broadcasts struct{ stages []string }

func (b *broadcasts) BroadcastStageUpdate(_, entryID, _ string) {
	b.stages = append(b.stages, entryID)
}

type timeline struct {
	events  []*ce.ConversationEvent
	listErr error
	saveErr error
	asked   []ce.ListFilter
}

func (t *timeline) Create(event *ce.ConversationEvent) error {
	if t.saveErr != nil {
		return t.saveErr
	}
	t.events = append(t.events, event)
	return nil
}

func (t *timeline) ListByEntryFiltered(_, entryID, _ string, filter ce.ListFilter) ([]*ce.ConversationEvent, int64, error) {
	t.asked = append(t.asked, filter)
	if t.listErr != nil {
		return nil, 0, t.listErr
	}
	var matched []*ce.ConversationEvent
	for i := len(t.events) - 1; i >= 0; i-- {
		e := t.events[i]
		if e.EntryID == entryID && e.EventType == filter.EventType && (filter.Since == nil || !e.CreatedAt.Before(*filter.Since)) {
			matched = append(matched, e)
		}
	}
	total := int64(len(matched))
	if filter.Limit > 0 && len(matched) > filter.Limit {
		matched = matched[:filter.Limit]
	}
	return matched, total, nil
}

type alerts struct {
	refusals []*automationneeds.Refusal
	from     []string
}

func (a *alerts) TransferUnmet(_ context.Context, _ string, refusal *automationneeds.Refusal, fromName string) error {
	a.refusals = append(a.refusals, refusal)
	a.from = append(a.from, fromName)
	return nil
}

type needsBook map[string]automationneeds.Needs

func (b needsBook) Needs(_ string, a conversation.Automation) (automationneeds.Needs, error) {
	if a.ID == "needs-broken" {
		return nil, errors.New("connection reset")
	}
	return b[a.ID], nil
}

type resolver struct {
	lead    map[string]string
	current map[string]string
	asked   [][]conversationvars.Binding
}

func (r *resolver) ResolveValues(_ context.Context, _ conversationvars.Ref, bindings []conversationvars.Binding) (map[string]string, error) {
	r.asked = append(r.asked, bindings)
	out := map[string]string{}
	var empty []string
	for _, b := range bindings {
		var value string
		switch {
		case b.Source == campaign.BindLiteral:
			value = b.Value
		case b.Source == conversationvars.SourceCurrent:
			if r.current[b.Name] == "" {
				empty = append(empty, b.Name)
			}
			continue
		default:
			value = r.lead[string(b.Source)]
		}
		if value == "" {
			empty = append(empty, b.Name)
			continue
		}
		out[b.Name] = value
	}
	if len(empty) > 0 {
		return nil, &conversationvars.VariablesError{Cause: conversationvars.ErrValueEmpty, Names: empty}
	}
	return out, nil
}

func (r *resolver) LeadFields(string) ([]*customfield.Definition, error) {
	return []*customfield.Definition{{Key: "curso", Type: customfield.TypeText}}, nil
}

type variableStore struct {
	values  map[string]string
	saveErr error
	saves   []map[string]string
	by      []string
	forgot  [][]string
	order   *[]string
}

func (v *variableStore) Values(context.Context, string, shared.EntryType) (map[string]string, error) {
	out := map[string]string{}
	for name, value := range v.values {
		out[name] = value
	}
	return out, nil
}

func (v *variableStore) Save(_ context.Context, _ conversationvars.Ref, values map[string]string, by string) error {
	*v.order = append(*v.order, "store")
	if v.saveErr != nil {
		return v.saveErr
	}
	v.saves = append(v.saves, values)
	v.by = append(v.by, by)
	for name, value := range values {
		v.values[name] = value
	}
	return nil
}

func (v *variableStore) Forget(_ context.Context, _ conversationvars.Ref, names []string) error {
	*v.order = append(*v.order, "forget")
	v.forgot = append(v.forgot, names)
	for _, name := range names {
		delete(v.values, name)
	}
	return nil
}

type harness struct {
	needs       needsBook
	resolver    *resolver
	store       *variableStore
	service     *Service
	governance  *governance
	delegations *delegations
	stages      *stages
	broadcasts  *broadcasts
	timeline    *timeline
	alerts      *alerts
	order       []string
	now         time.Time
}

func agentAt(id string) conversation.Automation {
	return conversation.Automation{Kind: conversation.AutomationAgent, ID: id}
}

func workflowAt(id string) conversation.Automation {
	return conversation.Automation{Kind: conversation.AutomationWorkflow, ID: id}
}

func newHarness() *harness {
	h := &harness{now: time.Date(2026, 10, 27, 15, 0, 0, 0, time.UTC)}
	h.governance = &governance{governing: agentAt("ag-prospeccao"), governed: true}
	h.delegations = &delegations{order: &h.order}
	h.stages = &stages{order: &h.order, byID: map[string]stage.FunnelStage{
		"st-centro":  {StageID: "st-centro", StageName: "Atendimento", PipelineID: "p-centro", PipelineName: "Polo Centro"},
		"st-norte":   {StageID: "st-norte", StageName: "Atendimento", PipelineID: "p-norte", PipelineName: "Polo Norte"},
		"st-entrada": {StageID: "st-entrada", StageName: "Entrada", PipelineID: "p-redes", PipelineName: "Redes sociais"},
	}}
	h.broadcasts = &broadcasts{}
	h.timeline = &timeline{}
	h.alerts = &alerts{}
	h.needs = needsBook{}
	h.resolver = &resolver{lead: map[string]string{}, current: map[string]string{}}
	h.store = &variableStore{values: map[string]string{}, order: &h.order}
	service, err := New(Deps{
		Needs:     h.needs,
		Values:    h.resolver,
		Variables: h.store,
		Agents: agentBook{
			"ag-prospeccao": {ID: "ag-prospeccao", WorkspaceID: "ws-1", Name: "Agente Prospecção", IsActive: true},
			"ag-academico":  {ID: "ag-academico", WorkspaceID: "ws-1", Name: "Agente Acadêmico", Description: "Atende alunos matriculados.", IsActive: true},
			"ag-off":        {ID: "ag-off", WorkspaceID: "ws-1", Name: "Desligado"},
			"ag-other":      {ID: "ag-other", WorkspaceID: "ws-2", Name: "De outro workspace", IsActive: true},
		},
		Workflows: workflowBook{
			"wf-matricula": {ID: "wf-matricula", WorkspaceID: "ws-1", Name: "Matrícula", Description: "Faz a matrícula de novos alunos.", Status: workflow.WorkflowStatusActive, PublishedVersion: &published},
			"wf-draft":     {ID: "wf-draft", WorkspaceID: "ws-1", Name: "Rascunho", Status: workflow.WorkflowStatusDraft},
			"wf-paused":    {ID: "wf-paused", WorkspaceID: "ws-1", Name: "Pausado", Status: workflow.WorkflowStatusPaused, PublishedVersion: &published},
			"wf-ura":       {ID: "wf-ura", WorkspaceID: "ws-1", Name: "URA", Type: workflow.WorkflowTypeVoice, Status: workflow.WorkflowStatusActive, PublishedVersion: &published},
		},
		Governance:  h.governance,
		Delegations: h.delegations,
		Stages:      h.stages,
		Placement:   h.stages,
		Remover:     h.stages,
		Broadcaster: h.broadcasts,
		Timeline:    h.timeline,
		Alerts:      h.alerts,
		Now:         func() time.Time { return h.now },
	})
	if err != nil {
		panic(err)
	}
	h.service = service
	return h
}

func destinationsOf(items ...map[string]interface{}) map[string]interface{} {
	list := make([]interface{}, 0, len(items))
	for _, item := range items {
		list = append(list, item)
	}
	return map[string]interface{}{automationtransfer.DestinationsKey: list}
}

func academicConfig() map[string]interface{} {
	return destinationsOf(
		map[string]interface{}{"id": "centro", "kind": "agent", "automation_id": "ag-academico", "stage_id": "st-centro"},
		map[string]interface{}{"id": "norte", "kind": "agent", "automation_id": "ag-academico", "stage_id": "st-norte", "description": "Alunos do polo Norte."},
		map[string]interface{}{"id": "matricula", "kind": "workflow", "automation_id": "wf-matricula"},
	)
}

func (h *harness) request(destinationID, note string) Request {
	return Request{
		WorkspaceID: "ws-1", EntryID: "entry-1", EntryType: shared.EntryTypeWhatsApp,
		AgentID: "ag-prospeccao", AgentName: "Agente Prospecção",
		Config: academicConfig(), DestinationID: destinationID, Note: note,
	}
}

func (h *harness) pastTransfers(n int, ago time.Duration) {
	for i := 0; i < n; i++ {
		h.timeline.events = append(h.timeline.events, &ce.ConversationEvent{
			EntryID: "entry-1", EventType: ce.EventAutomationTransferred, CreatedAt: h.now.Add(-ago),
		})
	}
}
