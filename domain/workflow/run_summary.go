package workflow

import (
	"sort"
	"strings"
)

type RunningState string

const (
	RunningDraft  RunningState = "draft"
	RunningPaused RunningState = "paused"
	RunningActive RunningState = "active"
)

func RunningStateOf(w *Workflow) RunningState {
	switch WorkflowBlockReason(w) {
	case EntryBlockedNotPublished:
		return RunningDraft
	case EntryBlockedPaused:
		return RunningPaused
	}
	return RunningActive
}

type RunTargetKind string

const (
	RunTargetForm           RunTargetKind = "form"
	RunTargetAgent          RunTargetKind = "agent"
	RunTargetStage          RunTargetKind = "stage"
	RunTargetSIPTrunk       RunTargetKind = "sip_trunk"
	RunTargetCampaignButton RunTargetKind = "campaign_button"
	RunTargetTemplateButton RunTargetKind = "template_button"
)

type RunTarget struct {
	Kind     RunTargetKind `json:"kind"`
	ID       string        `json:"id,omitempty"`
	Name     string        `json:"name,omitempty"`
	Off      bool          `json:"off,omitempty"`
	Missing  bool          `json:"missing,omitempty"`
	Button   string        `json:"button,omitempty"`
	Language string        `json:"language,omitempty"`
}

type RunCondition struct {
	Kind        EntryPointKind   `json:"kind"`
	Scope       TriggerScope     `json:"scope,omitempty"`
	NodeID      string           `json:"nodeId,omitempty"`
	Reason      EntryBlockReason `json:"reason,omitempty"`
	Targets     []RunTarget      `json:"targets,omitempty"`
	MoreTargets int              `json:"moreTargets,omitempty"`
	Stage       *RunTarget       `json:"stage,omitempty"`
	Hour        *int             `json:"hour,omitempty"`
}

type WorkflowRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type RunOverlap struct {
	Kind      EntryPointKind `json:"kind"`
	Target    *RunTarget     `json:"target,omitempty"`
	Workflows []WorkflowRef  `json:"workflows"`
}

type RunSummary struct {
	State      RunningState   `json:"state"`
	Conditions []RunCondition `json:"conditions"`
	Overlaps   []RunOverlap   `json:"overlaps,omitempty"`
}

type EntryTrunk struct {
	Name    string
	Enabled bool
}

type ActiveTrigger struct {
	WorkflowID   string
	WorkflowName string
	Node         Node
	LegacyConfig map[string]interface{}
}

const TrunkIDKey = "trunk_id"

var overlappingTriggers = []TriggerType{TriggerFormSubmitted, TriggerBirthday, TriggerCallReceived}

func OverlappingTriggerTypes() []TriggerType {
	return append([]TriggerType(nil), overlappingTriggers...)
}

func overlaps(t TriggerType) bool {
	for _, candidate := range overlappingTriggers {
		if candidate == t {
			return true
		}
	}
	return false
}

func HasOverlappingTrigger(triggers []Node) bool {
	for _, node := range triggers {
		if overlaps(TriggerType(node.Type)) {
			return true
		}
	}
	return false
}

func EffectiveTriggerConfig(nodeConfig, legacy map[string]interface{}) map[string]interface{} {
	if len(nodeConfig) == 0 && len(legacy) > 0 {
		return legacy
	}
	if nodeConfig == nil {
		return map[string]interface{}{}
	}
	return nodeConfig
}

func TrunkIDOf(config map[string]interface{}) string {
	trunkID, _ := config[TrunkIDKey].(string)
	return strings.TrimSpace(trunkID)
}

func RunSummaryOf(w *Workflow, triggers []Node, facts EntryPointFacts, active []ActiveTrigger) RunSummary {
	summary := RunSummary{State: RunningStateOf(w), Conditions: RunConditionsOf(triggers, facts)}
	if summary.State == RunningActive && w != nil {
		summary.Overlaps = RunOverlapsOf(w.ID, triggers, active, facts)
	}
	return summary
}

func PublishPreviewOf(w *Workflow, facts EntryPointFacts, active []ActiveTrigger) RunSummary {
	triggers := DraftTriggers(w)
	summary := RunSummary{State: RunningActive, Conditions: RunConditionsOf(triggers, facts)}
	if w != nil {
		summary.Overlaps = RunOverlapsOf(w.ID, triggers, active, facts)
	}
	return summary
}

func DraftTriggers(w *Workflow) []Node {
	if w == nil {
		return nil
	}
	var out []Node
	for _, n := range w.Graph.TriggerNodes() {
		out = append(out, *n)
	}
	return out
}

func RunConditionsOf(triggers []Node, facts EntryPointFacts) []RunCondition {
	out := []RunCondition{}
	for _, node := range triggers {
		kind, ok := EntryKindOfTrigger(TriggerType(node.Type))
		if !ok {
			continue
		}
		scope, _ := TriggerType(node.Type).Scope()
		details, reason := kindReadiness(node, kind, facts)
		condition := RunCondition{Kind: kind, Scope: scope, NodeID: node.ID, Reason: reason, Targets: detailTargets(details)}
		if scope == TriggerScopeConversation {
			condition.MoreTargets = distinctChannels(facts.Channels) - len(details.Channels)
		}
		switch TriggerType(node.Type) {
		case TriggerStageAdded:
			condition.Stage = stageTarget(node.Config, facts.Stages)
		case TriggerBirthday:
			if hour, valid := birthdayHourOf(node.Config); valid {
				condition.Hour = &hour
			}
		case TriggerCallReceived:
			condition.Targets, condition.Reason = trunkCondition(node.Config, facts.Trunks)
		}
		out = append(out, condition)
	}
	if len(facts.LeadExits) > 0 {
		out = append(out, RunCondition{Kind: EntryLeadExit, Reason: leadExitReadiness(triggers), Targets: leadExitTargets(facts.LeadExits)})
	}
	return out
}

func detailTargets(details EntryPointDetails) []RunTarget {
	var targets []RunTarget
	for _, c := range details.Channels {
		targets = append(targets, RunTarget{Kind: RunTargetKind(c.Kind), ID: c.ID, Name: c.Name, Off: !c.WorkflowEnabled})
	}
	for _, f := range details.Forms {
		targets = append(targets, RunTarget{Kind: RunTargetForm, ID: f.ID, Name: f.Name, Off: !f.Missing && !f.Active, Missing: f.Missing})
	}
	for _, a := range details.Agents {
		targets = append(targets, RunTarget{Kind: RunTargetAgent, ID: a.ID, Name: a.Name, Off: !a.Active})
	}
	return targets
}

func distinctChannels(channels []EntryChannel) int {
	seen := map[EntryChannel]bool{}
	for _, c := range channels {
		seen[EntryChannel{Kind: c.Kind, ID: c.ID}] = true
	}
	return len(seen)
}

func stageTarget(config map[string]interface{}, names map[string]string) *RunTarget {
	stageID, _ := config[DataKeyStageID].(string)
	stageID = strings.TrimSpace(stageID)
	if stageID == "" {
		return nil
	}
	name, known := names[stageID]
	return &RunTarget{Kind: RunTargetStage, ID: stageID, Name: name, Missing: !known}
}

func trunkTarget(trunkID string, trunks map[string]EntryTrunk) (RunTarget, bool) {
	trunk, known := trunks[trunkID]
	if !known {
		return RunTarget{Kind: RunTargetSIPTrunk, ID: trunkID, Missing: true}, false
	}
	return RunTarget{Kind: RunTargetSIPTrunk, ID: trunkID, Name: trunk.Name, Off: !trunk.Enabled}, true
}

func trunkCondition(config map[string]interface{}, trunks map[string]EntryTrunk) ([]RunTarget, EntryBlockReason) {
	trunkID := TrunkIDOf(config)
	if trunkID == "" {
		return nil, EntryBlockedNoTrunk
	}
	target, known := trunkTarget(trunkID, trunks)
	if !known {
		return []RunTarget{target}, EntryBlockedNoTrunk
	}
	return []RunTarget{target}, ""
}

func leadExitTargets(exits []LeadExitEntry) []RunTarget {
	targets := make([]RunTarget, 0, len(exits))
	for _, exit := range exits {
		kind := RunTargetTemplateButton
		if exit.Source == LeadExitFromCampaign {
			kind = RunTargetCampaignButton
		}
		targets = append(targets, RunTarget{Kind: kind, ID: exit.ID, Name: exit.Name, Button: exit.Button, Language: exit.Language})
	}
	return targets
}

func RunOverlapsOf(workflowID string, triggers []Node, active []ActiveTrigger, facts EntryPointFacts) []RunOverlap {
	var out []RunOverlap
	for _, node := range triggers {
		switch TriggerType(node.Type) {
		case TriggerFormSubmitted:
			ids, _ := FormTriggerFormIDs(node.Config)
			for _, formID := range ids {
				others := othersWhere(workflowID, active, TriggerFormSubmitted, func(other ActiveTrigger) bool {
					return FormTriggerAccepts(EffectiveTriggerConfig(other.Node.Config, other.LegacyConfig), formID)
				})
				if len(others) > 0 {
					out = append(out, RunOverlap{Kind: EntryFormSubmitted, Target: formTarget(formID, facts.Forms), Workflows: others})
				}
			}
		case TriggerBirthday:
			if others := othersWhere(workflowID, active, TriggerBirthday, func(ActiveTrigger) bool { return true }); len(others) > 0 {
				out = append(out, RunOverlap{Kind: EntryBirthday, Workflows: others})
			}
		case TriggerCallReceived:
			trunkID := TrunkIDOf(node.Config)
			if trunkID == "" {
				continue
			}
			others := othersWhere(workflowID, active, TriggerCallReceived, func(other ActiveTrigger) bool {
				return TrunkIDOf(EffectiveTriggerConfig(other.Node.Config, other.LegacyConfig)) == trunkID
			})
			if len(others) > 0 {
				target, _ := trunkTarget(trunkID, facts.Trunks)
				out = append(out, RunOverlap{Kind: EntryCallReceived, Target: &target, Workflows: others})
			}
		}
	}
	return out
}

func formTarget(formID string, forms map[string]EntryForm) *RunTarget {
	form, known := forms[formID]
	return &RunTarget{Kind: RunTargetForm, ID: formID, Name: form.Name, Off: known && !form.Active, Missing: !known}
}

func othersWhere(workflowID string, active []ActiveTrigger, t TriggerType, matches func(ActiveTrigger) bool) []WorkflowRef {
	seen := map[string]bool{}
	var out []WorkflowRef
	for _, other := range active {
		if other.WorkflowID == workflowID || seen[other.WorkflowID] || TriggerType(other.Node.Type) != t || !matches(other) {
			continue
		}
		seen[other.WorkflowID] = true
		out = append(out, WorkflowRef{ID: other.WorkflowID, Name: other.WorkflowName})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out
}
