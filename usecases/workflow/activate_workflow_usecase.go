package workflow_usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"

	"vozko/domain/agent"
	domainmcp "vozko/domain/agent/mcp"
	"vozko/domain/automationneeds"
	"vozko/domain/callrouting"
	"vozko/domain/conversation"
	"vozko/domain/customfield"
	label_domain "vozko/domain/label"
	"vozko/domain/lead"
	media_domain "vozko/domain/media"
	rag_domain "vozko/domain/rag"
	"vozko/domain/shared/deps"
	stage_domain "vozko/domain/stage"
	businessphone_domain "vozko/domain/whatsapp/business_phone"
	template_domain "vozko/domain/whatsapp/template"
	"vozko/domain/workflow"
	workspace_domain "vozko/domain/workspace"
	dept_domain "vozko/domain/workspace/workspace_department"
	automationneeds_usecase "vozko/usecases/automationneeds"
	opportunity_usecase "vozko/usecases/opportunity"
)

type MemberLookup interface {
	GetMember(workspaceID, userID string) (*workspace_domain.Member, error)
}

type BusinessPhoneLookup interface {
	FindByID(id string) (*businessphone_domain.WhatsAppBusinessPhoneNumber, error)
}

type KnowledgeBaseLookup interface {
	FindByIDs(ctx context.Context, ids []string) ([]*rag_domain.KnowledgeBase, error)
}

type CustomFieldLister interface {
	ListByObject(workspaceID string, objectType customfield.ObjectType) ([]*customfield.Definition, error)
}

type CallQueueLookup interface {
	FindInWorkspace(ctx context.Context, workspaceID, id string) (*callrouting.Queue, error)
}

type InboundTrunkLookup interface {
	ReceivesCalls(workspaceID, trunkID string) (bool, error)
}

type ModelLookup interface {
	IsValidModel(ctx context.Context, modelID string) (bool, error)
}

type StageLookup interface {
	FindByID(id string) (*stage_domain.Stage, error)
}

type WorkflowSwitchingPublisher interface {
	PublishVersionSwitchingOff(in workflow.PublishVersion, workspaceID string, offs []automationneeds.SwitchOff) (*workflow.WorkflowVersion, []string, error)
}

type WorkflowPublishPlanner interface {
	PlanWorkflow(ctx context.Context, draft *workflow.Workflow) (automationneeds_usecase.Plan, error)
	Done(ctx context.Context, plan automationneeds_usecase.Plan, containers []string)
}

type DealFunnels interface {
	PipelineStages(workspaceID, pipelineID string) ([]*stage_domain.Stage, error)
}

type ActivateWorkflowDeps struct {
	Workflows         workflow.WorkflowRepository
	Catalog           func() []workflow.NodeDefinition
	Templates         template_domain.Repository
	Agents            agent.Repository
	Media             media_domain.MediaRepository
	Labels            label_domain.Repository
	Stages            StageLookup
	StageDestinations stage_domain.DestinationReader
	DealFunnels       DealFunnels
	Departments       dept_domain.Repository
	Members           MemberLookup
	BusinessPhones    BusinessPhoneLookup
	MCPCollections    domainmcp.CollectionRepository
	KnowledgeBases    KnowledgeBaseLookup
	CustomFields      CustomFieldLister
	Sheets            GoogleSheetLookup
	Models            ModelLookup
	ModelCatalog      ModelCatalog
	InboundTrunks     InboundTrunkLookup
	CallQueues        CallQueueLookup
	Switching         WorkflowSwitchingPublisher
	Planner           WorkflowPublishPlanner
}

type activateWorkflowUseCase struct {
	repo              workflow.WorkflowRepository
	catalogFn         func() []workflow.NodeDefinition
	templateRepo      template_domain.Repository
	agentRepo         agent.Repository
	mediaRepo         media_domain.MediaRepository
	labelRepo         label_domain.Repository
	stageRepo         StageLookup
	stageDestinations stage_domain.DestinationReader
	dealFunnels       DealFunnels
	departmentRepo    dept_domain.Repository
	workspaceRepo     MemberLookup
	businessPhoneRepo BusinessPhoneLookup
	mcpCollectionRepo domainmcp.CollectionRepository
	knowledgeBaseRepo KnowledgeBaseLookup
	customFields      CustomFieldLister
	googleSheets      GoogleSheetLookup

	modelLookup  ModelLookup
	modelCatalog ModelCatalog

	inboundTrunks InboundTrunkLookup
	callQueues    CallQueueLookup

	switching WorkflowSwitchingPublisher
	planner   WorkflowPublishPlanner
}

func NewActivateWorkflowUseCase(d ActivateWorkflowDeps) (workflow.ActivateWorkflowUseCase, error) {
	if err := deps.Require("workflow activation", map[string]any{
		"workflows":          d.Workflows,
		"catalog":            d.Catalog,
		"templates":          d.Templates,
		"agents":             d.Agents,
		"media":              d.Media,
		"labels":             d.Labels,
		"stages":             d.Stages,
		"stage destinations": d.StageDestinations,
		"deal funnels":       d.DealFunnels,
		"departments":        d.Departments,
		"members":            d.Members,
		"business phones":    d.BusinessPhones,
		"mcp collections":    d.MCPCollections,
		"knowledge bases":    d.KnowledgeBases,
		"custom fields":      d.CustomFields,
		"google sheets":      d.Sheets,
		"models":             d.Models,
		"model catalog":      d.ModelCatalog,
		"inbound trunks":     d.InboundTrunks,
		"call queues":        d.CallQueues,
		"switching":          d.Switching,
		"publish planner":    d.Planner,
	}); err != nil {
		return nil, err
	}
	return &activateWorkflowUseCase{
		repo:              d.Workflows,
		catalogFn:         d.Catalog,
		templateRepo:      d.Templates,
		agentRepo:         d.Agents,
		mediaRepo:         d.Media,
		labelRepo:         d.Labels,
		stageRepo:         d.Stages,
		stageDestinations: d.StageDestinations,
		dealFunnels:       d.DealFunnels,
		departmentRepo:    d.Departments,
		workspaceRepo:     d.Members,
		businessPhoneRepo: d.BusinessPhones,
		mcpCollectionRepo: d.MCPCollections,
		knowledgeBaseRepo: d.KnowledgeBases,
		customFields:      d.CustomFields,
		googleSheets:      d.Sheets,
		modelLookup:       d.Models,
		modelCatalog:      d.ModelCatalog,
		inboundTrunks:     d.InboundTrunks,
		callQueues:        d.CallQueues,
		switching:         d.Switching,
		planner:           d.Planner,
	}, nil
}

func (uc *activateWorkflowUseCase) Execute(ctx context.Context, workflowID string) (*workflow.Workflow, error) {
	w, err := uc.load(workflowID)
	if err != nil {
		return nil, err
	}
	if w.Status != workflow.WorkflowStatusDraft && w.Status != workflow.WorkflowStatusPaused {
		return nil, workflow.ErrCannotActivate
	}
	if w.PublishedVersion == nil {
		return uc.publish(ctx, w)
	}

	running, err := workflow.PublishedOf(uc.repo, w)
	if err != nil {
		return nil, err
	}
	running.Normalize()
	if err := uc.validateRunnable(ctx, running); err != nil {
		return nil, err
	}
	if err := uc.repo.UpdateStatus(w.ID, workflow.WorkflowStatusActive); err != nil {
		return nil, err
	}
	slog.Info("workflow.resumed", slog.String("workspace_id", w.WorkspaceID), slog.String("workflow_id", w.ID), slog.Int("version", *running.GraphVersion))
	return findWithPublication(uc.repo, w.ID)
}

func (uc *activateWorkflowUseCase) Publish(ctx context.Context, workflowID string) (*workflow.Workflow, error) {
	w, err := uc.load(workflowID)
	if err != nil {
		return nil, err
	}
	if w.Status == workflow.WorkflowStatusArchived {
		return nil, workflow.ErrCannotActivate
	}
	return uc.publish(ctx, w)
}

func (uc *activateWorkflowUseCase) load(workflowID string) (*workflow.Workflow, error) {
	if workflowID == "" {
		return nil, workflow.ErrWorkflowNotFound
	}
	w, err := uc.repo.FindByID(workflowID)
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, workflow.ErrWorkflowNotFound
	}
	return w, nil
}

func (uc *activateWorkflowUseCase) publish(ctx context.Context, w *workflow.Workflow) (*workflow.Workflow, error) {
	storedHash, storedType := workflow.GraphHash(&w.Graph), w.Type
	w.Normalize()
	if err := uc.validateRunnable(ctx, w); err != nil {
		return nil, err
	}
	hash := workflow.GraphHash(&w.Graph)
	attrs := []any{slog.String("workspace_id", w.WorkspaceID), slog.String("workflow_id", w.ID)}
	if w.PublishedVersion != nil && hash == w.PublishedGraphHash {
		if w.Status != workflow.WorkflowStatusActive {
			if err := uc.repo.UpdateStatus(w.ID, workflow.WorkflowStatusActive); err != nil {
				return nil, err
			}
		}
		slog.Info("workflow.publish.unchanged", append(attrs, slog.Int("version", *w.PublishedVersion))...)
		return findWithPublication(uc.repo, w.ID)
	}
	if uc.planner == nil || uc.switching == nil {
		return nil, automationneeds.ErrUnreadable
	}
	plan, err := uc.planner.PlanWorkflow(ctx, w)
	if err != nil {
		return nil, err
	}
	v, containers, err := uc.switching.PublishVersionSwitchingOff(workflow.PublishVersionOf(w, workflow.PublisherFrom(ctx), hash != storedHash || storedType != w.Type), plan.WorkspaceID, plan.Offs)
	if err != nil {
		return nil, err
	}
	uc.planner.Done(ctx, plan, containers)
	slog.Info("workflow.published", append(attrs, slog.Int("version", v.Version))...)
	return findWithPublication(uc.repo, w.ID)
}

func (uc *activateWorkflowUseCase) graphProblems(ctx context.Context, w *workflow.Workflow) (workflow.LintReport, []error) {
	catalog := uc.catalogFn()
	report := LintWorkflowGraph(catalog, w)
	report.Issues = append(report.Issues, workflow.HTTPSIssues(&w.Graph, uc.runningGraph(w))...)
	report.Issues = append(report.Issues, uc.stageDestinationNotes(w)...)

	var problems []error
	if err := checkGraphReasoning(ctx, uc.modelCatalog, &w.Graph); err != nil {
		problems = append(problems, err)
	}
	if len(catalog) > 0 {
		problems = append(problems, workflow.ReferenceProblems(&w.Graph, catalog, uc.referenceValidators(w)...)...)
	}
	if w.Type == workflow.WorkflowTypeVoice {
		problems = append(problems, uc.callQueueProblems(w)...)
	}
	return report, problems
}

func validationOnly(problems []error) error {
	for _, p := range problems {
		if !workflow.IsValidationError(p) {
			return p
		}
	}
	return nil
}

func (uc *activateWorkflowUseCase) Lint(ctx context.Context, w *workflow.Workflow) (workflow.LintReport, error) {
	report, problems := uc.graphProblems(ctx, w)
	if err := validationOnly(problems); err != nil {
		return workflow.LintReport{}, err
	}
	return report.WithProblems(problems), nil
}

func (uc *activateWorkflowUseCase) validateRunnable(ctx context.Context, w *workflow.Workflow) error {
	report, problems := uc.graphProblems(ctx, w)
	if w.Type == workflow.WorkflowTypeVoice {
		if err := uc.validateVoiceTrunk(w); err != nil {
			problems = append(problems, err)
		}
	}
	if err := validationOnly(problems); err != nil {
		return err
	}

	err := workflow.NotPublishableFrom(report, problems)
	if blocked, ok := workflow.NotPublishableOf(err); ok {
		slog.Info("workflow.publish.blocked", slog.String("workspace_id", w.WorkspaceID), slog.String("workflow_id", w.ID), slog.Int("issues", len(blocked.Issues)))
	}
	return err
}

func (uc *activateWorkflowUseCase) referenceValidators(w *workflow.Workflow) []workflow.ConfigValidator {
	workflowWorkspaceID := strings.TrimSpace(w.WorkspaceID)
	return []workflow.ConfigValidator{
		&businessPhoneValidator{repo: uc.businessPhoneRepo, workspaceID: workflowWorkspaceID},
		&templateValidator{repo: uc.templateRepo, businessPhoneRepo: uc.businessPhoneRepo, workspaceID: workflowWorkspaceID},
		&agentValidator{repo: uc.agentRepo, workspaceID: workflowWorkspaceID},
		&mediaValidator{repo: uc.mediaRepo, workspaceID: workflowWorkspaceID},
		&labelValidator{repo: uc.labelRepo, workspaceID: workflowWorkspaceID},
		&customFieldValidator{repo: uc.customFields, workspaceID: workflowWorkspaceID},
		&stageValidator{repo: uc.stageRepo, destinations: uc.stageDestinations, workspaceID: workflowWorkspaceID},
		&dealValidator{funnels: uc.dealFunnels, workspaceID: workflowWorkspaceID},
		&departmentValidator{repo: uc.departmentRepo, workspaceID: workflowWorkspaceID},
		&memberValidator{repo: uc.workspaceRepo, workspaceID: workflowWorkspaceID},
		&mcpCollectionValidator{repo: uc.mcpCollectionRepo, workspaceID: workflowWorkspaceID},
		&knowledgeBaseValidator{repo: uc.knowledgeBaseRepo, workspaceID: workflowWorkspaceID},
		&modelValidator{lookup: uc.modelLookup},
		&googleSheetValidator{sheets: uc.googleSheets, workspaceID: workflowWorkspaceID},
		&workflowReferenceValidator{repo: uc.repo, workspaceID: workflowWorkspaceID},
	}
}

func collectExecuteModeLeafNodes(g *workflow.Graph) map[string]bool {

	executeAgents := make(map[string]bool)
	for i := range g.Nodes {
		node := &g.Nodes[i]
		if node.Type == workflow.NodeTypeActionAIAgent && workflow.ToolModeOf(node) == workflow.ToolModeExecute {
			executeAgents[node.ID] = true
		}
	}
	if len(executeAgents) == 0 {
		return nil
	}

	type edgeInfo struct {
		source string
		label  string
	}
	incomingByTarget := make(map[string][]edgeInfo)
	for _, e := range g.Edges {
		incomingByTarget[e.Target] = append(incomingByTarget[e.Target], edgeInfo{source: e.Source, label: e.Label})
	}

	reserved := map[string]bool{"default": true, "erro": true, "": true}

	leafNodes := make(map[string]bool)
	for targetID, edges := range incomingByTarget {
		allFromExecuteAgent := true
		for _, e := range edges {
			if !executeAgents[e.source] || reserved[e.label] {
				allFromExecuteAgent = false
				break
			}
		}
		if allFromExecuteAgent {
			leafNodes[targetID] = true
		}
	}
	return leafNodes
}

type templateValidator struct {
	repo              template_domain.Repository
	businessPhoneRepo BusinessPhoneLookup
	workspaceID       string
}

func (v *templateValidator) Validate(n *workflow.Node) error {
	if n.Type != workflow.NodeTypeActionSendTemplate {
		return nil
	}
	templateID, _ := n.Config["template_id"].(string)
	if strings.TrimSpace(templateID) == "" {
		return nil
	}
	tmpl, err := v.repo.FindByID(templateID)
	if err != nil || tmpl == nil {
		return fmt.Errorf("%w: node %q template_id %q", workflow.ErrNodeInvalidTemplateID, n.ID, templateID)
	}
	if !tmpl.CanBeSent() {
		return fmt.Errorf("%w: node %q template %q is not approved (status: %s)", workflow.ErrNodeInvalidTemplateID, n.ID, tmpl.Name, tmpl.Status)
	}

	businessPhoneID, _ := n.Config["business_phone_id"].(string)
	businessPhoneID = strings.TrimSpace(businessPhoneID)
	if businessPhoneID == "" || v.businessPhoneRepo == nil {
		return nil
	}

	phone, err := v.businessPhoneRepo.FindByID(businessPhoneID)
	if err != nil {
		return err
	}
	if phone == nil || !phone.BelongsToWorkspace(v.workspaceID) {
		return nil
	}
	if strings.TrimSpace(tmpl.WABAId) != "" && strings.TrimSpace(phone.WABAId) != "" && strings.TrimSpace(tmpl.WABAId) != strings.TrimSpace(phone.WABAId) {
		return fmt.Errorf("%w: node %q template_id %q incompatible with business_phone_id %q", workflow.ErrNodeInvalidTemplateID, n.ID, templateID, businessPhoneID)
	}
	return nil
}

type agentValidator struct {
	repo        agent.Repository
	workspaceID string
}

func (v *agentValidator) Validate(n *workflow.Node) error {
	if n.Type != workflow.NodeTypeActionAIAgent {
		return nil
	}

	source, _ := n.Config["source"].(string)
	if source != "prompt" {
		if agentID, _ := n.Config["agent_id"].(string); strings.TrimSpace(agentID) != "" {
			ag, err := v.repo.FindByID(agentID)
			if err != nil || ag == nil || !sameWorkspaceID(v.workspaceID, ag.WorkspaceID) {
				return fmt.Errorf("%w: node %q agent_id %q", workflow.ErrNodeInvalidAgentID, n.ID, agentID)
			}
		}
	}

	if n.Type == workflow.NodeTypeActionAIAgent {
		toolMode, _ := n.Config[workflow.ToolModeKey].(string)
		if !workflow.ValidToolMode(toolMode) {
			return fmt.Errorf("%w: node %q tool_mode %q", workflow.ErrNodeInvalidToolMode, n.ID, toolMode)
		}
		responseMode, _ := n.Config["response_mode"].(string)
		if responseMode != "" && responseMode != "default" && responseMode != "segmented" {
			return fmt.Errorf("%w: node %q response_mode %q", workflow.ErrNodeInvalidResponseMode, n.ID, responseMode)
		}
	}
	return nil
}

type mcpCollectionValidator struct {
	repo        domainmcp.CollectionRepository
	workspaceID string
}

func (v *mcpCollectionValidator) Validate(n *workflow.Node) error {
	if n.Type != workflow.NodeTypeActionAIAgent {
		return nil
	}
	raw, ok := n.Config["mcp_collection_ids"]
	if !ok || raw == nil {
		return nil
	}
	ids := extractStringSlice(raw)
	if len(ids) == 0 {
		return nil
	}
	found, err := v.repo.ListByIDs(context.Background(), v.workspaceID, ids)
	if err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(found))
	for _, c := range found {
		seen[c.ID] = struct{}{}
	}
	for _, id := range ids {
		if _, ok := seen[id]; !ok {
			return fmt.Errorf("%w: node %q mcp_collection_id %q", workflow.ErrNodeInvalidMCPCollectionID, n.ID, id)
		}
	}
	return nil
}

type knowledgeBaseValidator struct {
	repo        KnowledgeBaseLookup
	workspaceID string
}

func (v *knowledgeBaseValidator) Validate(n *workflow.Node) error {
	if n.Type != workflow.NodeTypeActionAIAgent {
		return nil
	}
	ids := extractStringSlice(n.Config["knowledge_base_ids"])
	if len(ids) == 0 {
		return nil
	}
	found, err := v.repo.FindByIDs(context.Background(), ids)
	if err != nil {
		return err
	}
	inWorkspace := make(map[string]struct{}, len(found))
	for _, kb := range found {
		if kb != nil && sameWorkspaceID(v.workspaceID, kb.WorkspaceID) {
			inWorkspace[kb.ID] = struct{}{}
		}
	}
	for _, id := range ids {
		if _, ok := inWorkspace[id]; !ok {
			return fmt.Errorf("%w: node %q knowledge_base_id %q", workflow.ErrNodeInvalidKnowledgeBaseID, n.ID, id)
		}
	}
	return nil
}

func extractStringSlice(raw any) []string {
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

type modelValidator struct {
	lookup ModelLookup
}

func (v *modelValidator) Validate(n *workflow.Node) error {
	if n.Type != workflow.NodeTypeActionAIAgent {
		return nil
	}
	if source, _ := n.Config["source"].(string); strings.TrimSpace(source) == "agent" {
		return nil
	}
	model, _ := n.Config["model"].(string)
	model = strings.TrimSpace(model)
	if model == "" || strings.Contains(model, "{{") {
		return nil
	}
	valid, err := v.lookup.IsValidModel(context.Background(), model)
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("%w: node %q model %q", workflow.ErrNodeInvalidModelID, n.ID, model)
	}
	return nil
}

func (uc *activateWorkflowUseCase) callQueueProblems(w *workflow.Workflow) []error {
	var problems []error
	for _, n := range w.Graph.Nodes {
		if n.Type != workflow.NodeTypeTransferToQueue {
			continue
		}
		queueID, _ := n.Config["queue_id"].(string)
		queueID = strings.TrimSpace(queueID)
		if queueID == "" {
			problems = append(problems, fmt.Errorf("%w: node %q", workflow.ErrNodeInvalidQueueID, n.ID))
			continue
		}
		if _, err := uc.callQueues.FindInWorkspace(context.Background(), w.WorkspaceID, queueID); err != nil {
			problems = append(problems, fmt.Errorf("%w: node %q queue_id %q", workflow.ErrNodeInvalidQueueID, n.ID, queueID))
		}
	}
	return problems
}

func (uc *activateWorkflowUseCase) validateVoiceTrunk(w *workflow.Workflow) error {
	trunkID := w.VoiceTrunkID()
	if trunkID == "" {
		return workflow.ErrVoiceTrunkRequired
	}
	receives, err := uc.inboundTrunks.ReceivesCalls(w.WorkspaceID, trunkID)
	if err != nil {
		return err
	}
	if !receives {
		return workflow.ErrVoiceTrunkInvalid
	}
	active, err := uc.repo.FindActiveByTrigger(w.WorkspaceID, workflow.TriggerCallReceived)
	if err != nil {
		return err
	}
	for _, other := range active {
		if other.ID != w.ID && other.VoiceTrunkID() == trunkID {
			return workflow.ErrVoiceTrunkTaken
		}
	}
	return nil
}

type mediaValidator struct {
	repo        media_domain.MediaRepository
	workspaceID string
}

func (v *mediaValidator) Validate(n *workflow.Node) error {
	if n.Type == workflow.NodeTypeActionPlayAudio {
		mediaID, _ := n.Config["media_id"].(string)
		m, err := v.repo.GetMediaByID(strings.TrimSpace(mediaID))
		if err != nil || !m.PlayableOnCallFor(v.workspaceID) {
			return fmt.Errorf("%w: node %q media_id %q", workflow.ErrNodeInvalidMediaID, n.ID, mediaID)
		}
		return nil
	}
	if n.Type == workflow.NodeTypeActionSendMedia {
		mediaURL, _ := n.Config["media_url"].(string)
		if strings.TrimSpace(mediaURL) != "" {
			return nil
		}
		mediaID, _ := n.Config["media_id"].(string)
		if strings.TrimSpace(mediaID) == "" {
			return nil
		}
		if _, err := conversation.SendableLibraryMedia(v.repo, v.workspaceID, mediaID); err != nil {
			return fmt.Errorf("%w: node %q media_id %q", workflow.ErrNodeInvalidMediaID, n.ID, mediaID)
		}
		return nil
	}
	return nil
}

type labelValidator struct {
	repo        label_domain.Repository
	workspaceID string
}

func (v *labelValidator) Validate(n *workflow.Node) error {
	switch n.Type {
	case workflow.NodeTypeActionAssignLabel, workflow.NodeTypeConditionCheckLabel:
		labelID, _ := n.Config["label_id"].(string)
		if strings.TrimSpace(labelID) == "" {
			return nil
		}
		l, err := v.repo.FindByID(labelID)
		if err != nil || l == nil || !sameWorkspaceID(v.workspaceID, l.WorkspaceID) {
			return fmt.Errorf("%w: node %q label_id %q", workflow.ErrNodeInvalidLabelID, n.ID, labelID)
		}
	}
	return nil
}

type customFieldValidator struct {
	repo        CustomFieldLister
	workspaceID string
}

func (v *customFieldValidator) Validate(n *workflow.Node) error {
	if n.Type != workflow.NodeTypeActionUpdateLead {
		return nil
	}
	keys := slices.Sorted(maps.Keys(workflow.UpdateLeadCustomFieldsOf(n.Config)))
	if len(keys) == 0 {
		return nil
	}
	defs, err := v.repo.ListByObject(v.workspaceID, customfield.ObjectLead)
	if err != nil {
		return fmt.Errorf("lead custom fields of workspace %s: %w", v.workspaceID, err)
	}
	err = lead.ProfileFieldsWritable(keys, defs)
	var refused *customfield.ValueError
	if !errors.As(err, &refused) {
		return err
	}
	cause := workflow.ErrNodeUnknownCustomField
	if errors.Is(refused.Err, customfield.ErrValueForbidden) {
		cause = workflow.ErrNodeSensitiveCustomField
	}
	return &workflow.CustomFieldRefError{NodeID: n.ID, Key: refused.Key, Err: cause}
}

var errStageDestinationsMissing = errors.New("workflow: the stage destinations cannot be checked")

type stageValidator struct {
	repo         StageLookup
	destinations stage_domain.DestinationReader
	workspaceID  string
}

func (v *stageValidator) Validate(n *workflow.Node) error {
	if n.Type != workflow.NodeTypeActionMoveStage && n.Type != workflow.NodeTypeConditionCheckStage {
		return nil
	}
	stageID := moveStageTarget(n)
	if stageID == "" {
		return nil
	}
	if n.Type == workflow.NodeTypeActionMoveStage {
		return v.validateDestination(n.ID, stageID)
	}
	s, err := v.repo.FindByID(stageID)
	if err != nil || s == nil || !sameWorkspaceID(v.workspaceID, s.WorkspaceID) {
		return fmt.Errorf("%w: node %q stage_id %q", workflow.ErrNodeInvalidStageID, n.ID, stageID)
	}
	return nil
}

func (v *stageValidator) validateDestination(nodeID, stageID string) error {
	if v.destinations == nil {
		return errStageDestinationsMissing
	}
	_, err := v.destinations.Destination(v.workspaceID, stageID)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, stage_domain.ErrStageNotConversationFunnel):
		return fmt.Errorf("%w: node %q stage_id %q", workflow.ErrNodeStageNotConversationFunnel, nodeID, stageID)
	case errors.Is(err, stage_domain.ErrTagNotFound), errors.Is(err, stage_domain.ErrUnauthorized):
		return fmt.Errorf("%w: node %q stage_id %q", workflow.ErrNodeInvalidStageID, nodeID, stageID)
	}
	return err
}

func moveStageTarget(n *workflow.Node) string {
	stageID, _ := n.Config["stage_id"].(string)
	return strings.TrimSpace(stageID)
}

func (uc *activateWorkflowUseCase) stageDestinationNotes(w *workflow.Workflow) []workflow.LintIssue {
	if uc.stageDestinations == nil {
		return nil
	}
	var notes []workflow.LintIssue
	for i := range w.Graph.Nodes {
		n := &w.Graph.Nodes[i]
		if n.Type != workflow.NodeTypeActionMoveStage || moveStageTarget(n) == "" {
			continue
		}
		to, err := uc.stageDestinations.Destination(strings.TrimSpace(w.WorkspaceID), moveStageTarget(n))
		if err != nil || to.PipelineName == "" {
			continue
		}
		notes = append(notes, workflow.StageDestinationIssue(n.ID, to.StageID, to.PipelineName, to.StageName))
	}
	return notes
}

type dealValidator struct {
	funnels     DealFunnels
	workspaceID string
}

func (v *dealValidator) Validate(n *workflow.Node) error {
	if n.Type != workflow.NodeTypeActionManageOpportunity && n.Type != workflow.NodeTypeConditionCheckOpportunity {
		return nil
	}
	pipelineID, _ := n.Config["pipeline_id"].(string)
	if pipelineID = strings.TrimSpace(pipelineID); pipelineID == "" {
		return nil
	}
	stages, err := v.funnels.PipelineStages(v.workspaceID, pipelineID)
	if refusal, refused := opportunity_usecase.RefusalOf(err); refused && refusal == opportunity_usecase.RefusalPipelineInvalid {
		return fmt.Errorf("%w: node %q pipeline_id %q", workflow.ErrNodeInvalidOpportunityPipeline, n.ID, pipelineID)
	}
	if err != nil {
		return err
	}
	stageID, _ := n.Config["stage_id"].(string)
	if stageID = strings.TrimSpace(stageID); stageID == "" {
		return nil
	}
	for _, st := range stages {
		if st.ID == stageID {
			return nil
		}
	}
	return fmt.Errorf("%w: node %q stage_id %q", workflow.ErrNodeInvalidStageID, n.ID, stageID)
}

type departmentValidator struct {
	repo        dept_domain.Repository
	workspaceID string
}

func (v *departmentValidator) Validate(n *workflow.Node) error {
	if n.Type != workflow.NodeTypeActionTransferDepartment {
		return nil
	}
	departmentID, _ := n.Config["department_id"].(string)
	if strings.TrimSpace(departmentID) == "" {
		return nil
	}
	d, err := v.repo.GetDepartmentByID(departmentID)
	if err != nil || d == nil || !sameWorkspaceID(v.workspaceID, d.WorkspaceID) {
		return fmt.Errorf("%w: node %q department_id %q", workflow.ErrNodeInvalidDepartmentID, n.ID, departmentID)
	}
	return nil
}

type memberValidator struct {
	repo        MemberLookup
	workspaceID string
}

func (v *memberValidator) Validate(n *workflow.Node) error {
	if n.Type != workflow.NodeTypeActionAssignMember {
		return nil
	}
	const memberKey = "member_id"

	memberID, _ := n.Config[memberKey].(string)
	memberID = strings.TrimSpace(memberID)
	if memberID == "" || strings.Contains(memberID, "{{") {
		return nil
	}
	m, err := v.repo.GetMember(v.workspaceID, memberID)
	if err != nil || m == nil || !sameWorkspaceID(v.workspaceID, m.WorkspaceID) {
		return fmt.Errorf("%w: node %q %s %q", workflow.ErrNodeInvalidMemberID, n.ID, memberKey, memberID)
	}
	return nil
}

type businessPhoneValidator struct {
	repo        BusinessPhoneLookup
	workspaceID string
}

func (v *businessPhoneValidator) Validate(n *workflow.Node) error {
	if n.Type != workflow.NodeTypeActionSendTemplate {
		return nil
	}
	businessPhoneID, _ := n.Config["business_phone_id"].(string)
	if strings.TrimSpace(businessPhoneID) == "" {
		return nil
	}
	phone, err := v.repo.FindByID(businessPhoneID)
	if err != nil || phone == nil || !phone.BelongsToWorkspace(v.workspaceID) {
		return fmt.Errorf("%w: node %q business_phone_id %q", workflow.ErrNodeInvalidBusinessPhoneID, n.ID, businessPhoneID)
	}
	return nil
}

type workflowReferenceValidator struct {
	repo        workflow.WorkflowRepository
	workspaceID string
}

func (v *workflowReferenceValidator) Validate(n *workflow.Node) error {
	if n.Type != workflow.NodeTypeActionRunWorkflow {
		return nil
	}
	workflowID, _ := n.Config["workflow_id"].(string)
	if strings.TrimSpace(workflowID) == "" {
		return nil
	}
	referencedWorkflow, err := v.repo.FindByID(workflowID)
	if err != nil || referencedWorkflow == nil || !sameWorkspaceID(v.workspaceID, referencedWorkflow.WorkspaceID) {
		return fmt.Errorf("%w: node %q workflow_id %q", workflow.ErrNodeInvalidWorkflowID, n.ID, workflowID)
	}
	return nil
}

func sameWorkspaceID(workflowWorkspaceID, resourceWorkspaceID string) bool {
	return strings.TrimSpace(workflowWorkspaceID) != "" && strings.TrimSpace(workflowWorkspaceID) == strings.TrimSpace(resourceWorkspaceID)
}

func (uc *activateWorkflowUseCase) runningGraph(w *workflow.Workflow) *workflow.Graph {
	if uc.repo == nil || w == nil || w.PublishedVersion == nil {
		return nil
	}
	running, err := workflow.PublishedOf(uc.repo, w)
	if err != nil || running == nil {
		return nil
	}
	return &running.Graph
}
