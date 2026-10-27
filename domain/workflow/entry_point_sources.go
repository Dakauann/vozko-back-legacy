package workflow

import (
	"strings"
	"time"
)

type EntryPointSourceQuery struct {
	WorkspaceID string
	WorkflowIDs []string
	FormIDs     []string
	StageIDs    []string
	TrunkIDs    []string
}

type WorkspaceChannel struct {
	EntryChannel
	WorkflowID string
}

type EntryPointSources struct {
	Webhooks       map[string]WebhookEntry
	Forms          map[string]EntryForm
	Agents         map[string][]EntryAgent
	LinkedChannels []WorkspaceChannel
	LeadExits      map[string][]LeadExitEntry
	LastStarts     map[string]map[EntryPointKind]time.Time
	LastDeliveries map[string]EntryDelivery
	Stages         map[string]string
	Trunks         map[string]EntryTrunk
}

type EntryPointReader interface {
	PublishedTriggers(workflowIDs []string) (map[string][]Node, error)
	EntryPointSources(q EntryPointSourceQuery) (*EntryPointSources, error)
	ActiveTriggers(workspaceID string, types []TriggerType) ([]ActiveTrigger, error)
}

func RunningTriggers(w *Workflow, published map[string][]Node) []Node {
	if w == nil {
		return nil
	}
	if w.PublishedVersion != nil {
		return published[w.ID]
	}
	return DraftTriggers(w)
}

func EntryPointQueryOf(workspaceID string, workflows []*Workflow, published map[string][]Node) EntryPointSourceQuery {
	running := map[string][]Node{}
	var ids []string
	for _, w := range workflows {
		if w == nil {
			continue
		}
		ids = append(ids, w.ID)
		running[w.ID] = RunningTriggers(w, published)
	}
	q := EntryPointQueryFor(workspaceID, running)
	q.WorkflowIDs = ids
	return q
}

func EntryPointQueryFor(workspaceID string, triggers map[string][]Node) EntryPointSourceQuery {
	q := EntryPointSourceQuery{WorkspaceID: workspaceID}
	forms, stages, trunks := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for workflowID, nodes := range triggers {
		q.WorkflowIDs = append(q.WorkflowIDs, workflowID)
		for _, node := range nodes {
			switch TriggerType(node.Type) {
			case TriggerFormSubmitted:
				ids, _ := FormTriggerFormIDs(node.Config)
				for _, id := range ids {
					forms[id] = true
				}
			case TriggerStageAdded:
				if stage := stageTarget(node.Config, nil); stage != nil {
					stages[stage.ID] = true
				}
			case TriggerCallReceived:
				if trunkID := TrunkIDOf(node.Config); trunkID != "" {
					trunks[trunkID] = true
				}
			}
		}
	}
	q.FormIDs, q.StageIDs, q.TrunkIDs = keysOf(forms), keysOf(stages), keysOf(trunks)
	return q
}

func keysOf(set map[string]bool) []string {
	var out []string
	for k := range set {
		if strings.TrimSpace(k) != "" {
			out = append(out, k)
		}
	}
	return out
}

func (s *EntryPointSources) FactsOf(workflowID string) EntryPointFacts {
	if s == nil {
		return EntryPointFacts{}
	}
	facts := EntryPointFacts{
		Forms:      s.Forms,
		Agents:     s.Agents[workflowID],
		LeadExits:  s.LeadExits[workflowID],
		LastStarts: s.LastStarts[workflowID],
		Stages:     s.Stages,
		Trunks:     s.Trunks,
	}
	if webhook, ok := s.Webhooks[workflowID]; ok {
		facts.Webhook = &webhook
	}
	if delivery, ok := s.LastDeliveries[workflowID]; ok {
		facts.LastDelivery = &delivery
	}
	for _, c := range s.LinkedChannels {
		if c.WorkflowID == workflowID {
			facts.Channels = append(facts.Channels, c.EntryChannel)
		}
	}
	return facts
}
