package automationneeds

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"vozko/domain/agent"
	"vozko/domain/conversation"
	"vozko/domain/workflow"
)

var (
	ErrNotFound   = errors.New("automation needs: the agent or workflow does not exist in this workspace")
	ErrUnreadable = errors.New("automation needs: the agent or workflow could not be read")
)

const (
	KeyLeadName     = "lead_name"
	KeyLeadNumber   = "lead_number"
	KeyCampaignName = "campaign_name"
)

var systemKeys = []string{KeyLeadName, KeyLeadNumber, KeyCampaignName}

func SystemKeys() []string {
	return append([]string(nil), systemKeys...)
}

func IsSystemKey(name string) bool {
	for _, key := range systemKeys {
		if key == name {
			return true
		}
	}
	return false
}

type Source interface {
	Agent(id string) (*agent.Agent, error)
	Workflow(id string) (*workflow.Workflow, error)
}

type Link struct {
	conversation.Automation
	Name string
}

type Need struct {
	Name        string
	Description string
	Default     string
	Via         []Link
}

func (n Need) Owner() Link {
	if len(n.Via) == 0 {
		return Link{}
	}
	return n.Via[len(n.Via)-1]
}

func (n Need) Path() []conversation.Automation {
	out := make([]conversation.Automation, 0, len(n.Via))
	for _, link := range n.Via {
		out = append(out, link.Automation)
	}
	return out
}

type Needs []Need

func (n Needs) Names() []string {
	if len(n) == 0 {
		return nil
	}
	out := make([]string, 0, len(n))
	for _, need := range n {
		out = append(out, need.Name)
	}
	return out
}

func (n Needs) Find(name string) *Need {
	for i := range n {
		if n[i].Name == name {
			return &n[i]
		}
	}
	return nil
}

func (n Needs) Missing(values map[string]string) Needs {
	return n.missing(func(name string) bool {
		return strings.TrimSpace(values[name]) == ""
	})
}

func (n Needs) MissingMetadata(metadata map[string]any) Needs {
	return n.missing(func(name string) bool {
		return agent.MetadataValueMissing(metadata, name)
	})
}

func (n Needs) missing(absent func(string) bool) Needs {
	var out Needs
	for _, need := range n {
		if absent(need.Name) {
			out = append(out, need)
		}
	}
	return out
}

func Of(src Source, workspaceID string, root conversation.Automation) (Needs, error) {
	vars, err := VariablesOf(src, workspaceID, root)
	if err != nil {
		return nil, err
	}
	return vars.Required, nil
}

type Variables struct {
	Required Needs
	Optional Needs
}

func VariablesOf(src Source, workspaceID string, root conversation.Automation) (Variables, error) {
	w := walker{src: src, workspaceID: workspaceID, seen: map[conversation.Automation]bool{}, found: map[string]Need{}, optional: map[string]Need{}}
	if err := w.visit(root, nil); err != nil {
		return Variables{}, err
	}
	return Variables{Required: w.needs(), Optional: w.optionals()}, nil
}

func (v Variables) Union(other Variables) Variables {
	required := v.Required.Union(other.Required)
	return Variables{Required: required, Optional: v.Optional.Union(other.Optional).without(required)}
}

func (n Needs) without(other Needs) Needs {
	var out Needs
	for _, need := range n {
		if other.Find(need.Name) == nil {
			out = append(out, need)
		}
	}
	return out
}

func DependsOn(src Source, workspaceID string, root, on conversation.Automation) (bool, error) {
	w := walker{src: src, workspaceID: workspaceID, seen: map[conversation.Automation]bool{}, found: map[string]Need{}}
	if err := w.visit(root, nil); err != nil {
		return false, err
	}
	return w.seen[on], nil
}

type walker struct {
	src         Source
	workspaceID string
	seen        map[conversation.Automation]bool
	found       map[string]Need
	optional    map[string]Need
}

func (w *walker) visit(a conversation.Automation, via []Link) error {
	a.ID = strings.TrimSpace(a.ID)
	if w.seen[a] {
		return nil
	}
	w.seen[a] = true
	switch a.Kind {
	case conversation.AutomationAgent:
		return w.agent(a, via)
	case conversation.AutomationWorkflow:
		return w.workflow(a, via)
	}
	return fmt.Errorf("%w: %q has no kind", ErrNotFound, a.ID)
}

func (w *walker) agent(a conversation.Automation, via []Link) error {
	found, err := w.src.Agent(a.ID)
	if err != nil {
		return fmt.Errorf("%w: agent %s: %w", ErrUnreadable, a.ID, err)
	}
	if found == nil || found.WorkspaceID != w.workspaceID {
		return fmt.Errorf("%w: agent %s", ErrNotFound, a.ID)
	}
	path := extend(via, Link{Automation: a, Name: found.Name})
	for _, need := range OfAgent(found) {
		w.add(need.Name, need.Description, path)
	}
	for _, need := range OptionalOfAgent(found) {
		w.addOptional(need, path)
	}
	for _, tool := range found.InternalTools {
		if tool.Name != workflow.AgentRunWorkflowTool {
			continue
		}
		if id, ok := followable(tool.Config, "workflow_id"); ok {
			if err := w.visit(conversation.Automation{Kind: conversation.AutomationWorkflow, ID: id}, path); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *walker) workflow(a conversation.Automation, via []Link) error {
	found, err := w.src.Workflow(a.ID)
	if err != nil {
		return fmt.Errorf("%w: workflow %s: %w", ErrUnreadable, a.ID, err)
	}
	if found == nil || found.WorkspaceID != w.workspaceID {
		return fmt.Errorf("%w: workflow %s", ErrNotFound, a.ID)
	}
	path := extend(via, Link{Automation: a, Name: found.Name})
	for _, node := range found.Graph.Nodes {
		if err := w.node(node, path); err != nil {
			return err
		}
	}
	return nil
}

func (w *walker) node(node workflow.Node, path []Link) error {
	for _, name := range workflow.ExtractCampaignVars(workflow.Graph{Nodes: []workflow.Node{node}}) {
		w.add(name, "", path)
	}
	next, ok := runsAutomation(node)
	if !ok {
		return nil
	}
	return w.visit(next, path)
}

func OfNode(src Source, workspaceID string, holder *workflow.Workflow, nodeID string) (Needs, error) {
	if holder == nil || holder.WorkspaceID != workspaceID {
		return nil, fmt.Errorf("%w: the workflow that holds node %s", ErrNotFound, nodeID)
	}
	node := holder.Graph.FindNode(nodeID)
	if node == nil {
		return nil, fmt.Errorf("%w: %s", workflow.ErrNodeNotFound, nodeID)
	}
	root := conversation.Automation{Kind: conversation.AutomationWorkflow, ID: holder.ID}
	w := walker{src: src, workspaceID: workspaceID, seen: map[conversation.Automation]bool{root: true}, found: map[string]Need{}}
	if err := w.node(*node, []Link{{Automation: root, Name: holder.Name}}); err != nil {
		return nil, err
	}
	return w.needs(), nil
}

func runsAutomation(node workflow.Node) (conversation.Automation, bool) {
	switch node.Type {
	case workflow.NodeTypeActionAIAgent:
		if source, _ := node.Config["source"].(string); source == "prompt" {
			return conversation.Automation{}, false
		}
		id, ok := followable(node.Config, "agent_id")
		return conversation.Automation{Kind: conversation.AutomationAgent, ID: id}, ok
	case workflow.NodeTypeActionRunWorkflow:
		id, ok := followable(node.Config, "workflow_id")
		return conversation.Automation{Kind: conversation.AutomationWorkflow, ID: id}, ok
	}
	return conversation.Automation{}, false
}

func followable(config map[string]interface{}, key string) (string, bool) {
	id, _ := config[key].(string)
	id = strings.TrimSpace(id)
	if id == "" || strings.Contains(id, "{{") {
		return "", false
	}
	return id, true
}

func extend(via []Link, next Link) []Link {
	return append(append([]Link(nil), via...), next)
}

func (w *walker) add(name, description string, path []Link) {
	name = strings.TrimSpace(name)
	if name == "" || IsSystemKey(name) {
		return
	}
	existing, seen := w.found[name]
	if !seen {
		w.found[name] = Need{Name: name, Description: description, Via: append([]Link(nil), path...)}
		return
	}
	if existing.Description == "" && description != "" {
		existing.Description = description
		w.found[name] = existing
	}
}

func (w *walker) needs() Needs {
	if len(w.found) == 0 {
		return nil
	}
	out := make(Needs, 0, len(w.found))
	for _, need := range w.found {
		out = append(out, need)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

type overlay struct {
	Source
	agent    *agent.Agent
	workflow *workflow.Workflow
}

func OverlayAgent(src Source, candidate *agent.Agent) Source {
	return overlay{Source: src, agent: candidate}
}

func OverlayWorkflow(src Source, candidate *workflow.Workflow) Source {
	return overlay{Source: src, workflow: candidate}
}

func (o overlay) Agent(id string) (*agent.Agent, error) {
	if o.agent != nil && o.agent.ID == id {
		return o.agent, nil
	}
	return o.Source.Agent(id)
}

func (o overlay) Workflow(id string) (*workflow.Workflow, error) {
	if o.workflow != nil && o.workflow.ID == id {
		return o.workflow, nil
	}
	return o.Source.Workflow(id)
}

func (n Needs) Union(other Needs) Needs {
	merged := map[string]Need{}
	for _, list := range []Needs{n, other} {
		for _, need := range list {
			if _, seen := merged[need.Name]; !seen {
				merged[need.Name] = need
			}
		}
	}
	if len(merged) == 0 {
		return nil
	}
	out := make(Needs, 0, len(merged))
	for _, need := range merged {
		out = append(out, need)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func OfAgent(a *agent.Agent) Needs {
	if a == nil {
		return nil
	}
	link := Link{Automation: conversation.Automation{Kind: conversation.AutomationAgent, ID: a.ID}, Name: a.Name}
	var out Needs
	for _, info := range a.RequiredVariableInfos() {
		if !info.Required || IsSystemKey(info.Name) || out.Find(info.Name) != nil {
			continue
		}
		out = append(out, Need{Name: info.Name, Description: info.Description, Via: []Link{link}})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func OptionalOfAgent(a *agent.Agent) Needs {
	if a == nil {
		return nil
	}
	link := Link{Automation: conversation.Automation{Kind: conversation.AutomationAgent, ID: a.ID}, Name: a.Name}
	var out Needs
	for _, info := range a.RequiredVariableInfos() {
		if info.Required || IsSystemKey(info.Name) || out.Find(info.Name) != nil {
			continue
		}
		out = append(out, Need{Name: info.Name, Description: info.Description, Default: info.DefaultValue, Via: []Link{link}})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (w *walker) addOptional(need Need, path []Link) {
	if w.optional == nil {
		w.optional = map[string]Need{}
	}
	if _, seen := w.optional[need.Name]; seen {
		return
	}
	need.Via = append([]Link(nil), path...)
	w.optional[need.Name] = need
}

func (w *walker) optionals() Needs {
	var out Needs
	for name, need := range w.optional {
		if _, required := w.found[name]; !required {
			out = append(out, need)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
