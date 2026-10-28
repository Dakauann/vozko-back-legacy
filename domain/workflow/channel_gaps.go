package workflow

import (
	"fmt"
	"strings"

	"vozko/domain/shared"
)

const (
	LintChannelStepUnsupported LintIssueCode = "CHANNEL_STEP_UNSUPPORTED"
	LintChannelStepFallsBack   LintIssueCode = "CHANNEL_STEP_FALLS_BACK"
	LintChannelDataMayFail     LintIssueCode = "CHANNEL_DATA_MAY_FAIL"
	LintChannelDataMayBeEmpty  LintIssueCode = "CHANNEL_DATA_MAY_BE_EMPTY"
)

type ChannelReach map[string][]shared.EntryType

func ChannelReachOf(g *Graph, starts []shared.EntryType) ChannelReach {
	return channelReach(g, starts, startsOnChannel)
}

func CalledChannelReachOf(g *Graph, starts []shared.EntryType) ChannelReach {
	return channelReach(g, starts, func(n *Node) bool { return n.Type != NodeTypeTriggerCallReceived && !TriggerNodeStartsOnLead(*n) })
}

func startsOnChannel(n *Node) bool {
	t := TriggerType(n.Type)
	return t.FollowsResponsibleWorkflow() || t == TriggerAgentCall
}

func channelReach(g *Graph, starts []shared.EntryType, carries func(*Node) bool) ChannelReach {
	reached := map[string]map[shared.EntryType]bool{}
	if g == nil || len(starts) == 0 {
		return ChannelReach{}
	}
	var queue []string
	grow := func(nodeID string, channels map[shared.EntryType]bool) {
		set := reached[nodeID]
		if set == nil {
			set = map[shared.EntryType]bool{}
			reached[nodeID] = set
		}
		grew := false
		for channel := range channels {
			if !set[channel] {
				set[channel] = true
				grew = true
			}
		}
		if grew {
			queue = append(queue, nodeID)
		}
	}
	initial := map[shared.EntryType]bool{}
	for _, channel := range starts {
		if IsChannel(string(channel)) {
			initial[channel] = true
		}
	}
	for _, trigger := range g.TriggerNodes() {
		if carries(trigger) {
			grow(trigger.ID, initial)
		}
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		source := g.FindNode(current)
		edges := g.OutgoingEdges(current)
		for _, e := range edges {
			grow(e.Target, passing(source, edges, e, reached[current]))
		}
	}
	out := ChannelReach{}
	for nodeID, set := range reached {
		if len(set) > 0 {
			out[nodeID] = SortedChannels(set)
		}
	}
	return out
}

func passing(source *Node, edges []Edge, e Edge, channels map[shared.EntryType]bool) map[shared.EntryType]bool {
	if source == nil || source.Type != NodeTypeConditionChannel {
		return channels
	}
	label := strings.ToLower(strings.TrimSpace(e.Label))
	out := map[shared.EntryType]bool{}
	if label != ChannelBranchDefault {
		if channels[shared.EntryType(label)] {
			out[shared.EntryType(label)] = true
		}
		return out
	}
	own := map[shared.EntryType]bool{}
	for _, other := range edges {
		own[shared.EntryType(strings.ToLower(strings.TrimSpace(other.Label)))] = true
	}
	for channel := range channels {
		if !own[channel] {
			out[channel] = true
		}
	}
	return out
}

type ChannelGap struct {
	NodeID    string
	Need      ChannelNeed
	Level     ChannelLevel
	Channels  []shared.EntryType
	FallsBack bool
}

func (gap ChannelGap) Code() LintIssueCode {
	switch {
	case !gap.Need.Required:
		return LintChannelDataMayBeEmpty
	case gap.Level == ChannelNever && gap.FallsBack:
		return LintChannelStepFallsBack
	case gap.Level == ChannelNever:
		return LintChannelStepUnsupported
	}
	return LintChannelDataMayFail
}

func (gap ChannelGap) Severity() LintSeverity {
	switch gap.Code() {
	case LintChannelStepUnsupported:
		return SeverityBlocking
	case LintChannelDataMayBeEmpty:
		return SeverityInfo
	}
	return SeverityAdvisory
}

func (gap ChannelGap) Blocks() bool {
	return gap.Severity() == SeverityBlocking
}

func ChannelGapsOf(g *Graph, reach ChannelReach, columns SheetChannelColumns) []ChannelGap {
	var gaps []ChannelGap
	if g == nil {
		return nil
	}
	for i := range g.Nodes {
		node := &g.Nodes[i]
		channels := reach[node.ID]
		if len(channels) == 0 {
			continue
		}
		for _, need := range NodeChannelNeeds(node, columns) {
			gaps = append(gaps, gapsOfNeed(g, node, need, channels)...)
		}
	}
	return gaps
}

func gapsOfNeed(g *Graph, node *Node, need ChannelNeed, channels []shared.EntryType) []ChannelGap {
	lacking := map[ChannelLevel][]shared.EntryType{}
	for _, channel := range channels {
		if level := ChannelProvides(channel, need.Fact); level != ChannelGuaranteed {
			lacking[level] = append(lacking[level], channel)
		}
	}
	fallsBack := need.Fallback != "" && resolveHandle(g, node.ID, need.Fallback)
	var out []ChannelGap
	for _, level := range []ChannelLevel{ChannelNever, ChannelMaybe} {
		if len(lacking[level]) > 0 {
			out = append(out, ChannelGap{NodeID: node.ID, Need: need, Level: level, Channels: lacking[level], FallsBack: fallsBack})
		}
	}
	return out
}

func resolveHandle(g *Graph, nodeID, handle string) bool {
	for _, e := range g.OutgoingEdges(nodeID) {
		if e.Label == handle {
			return true
		}
	}
	return false
}

func ChannelGapIssues(g *Graph, catalog []NodeDefinition, starts []shared.EntryType, columns SheetChannelColumns) []LintIssue {
	gaps := ChannelGapsOf(g, ChannelReachOf(g, starts), columns)
	if len(gaps) == 0 {
		return nil
	}
	defs := NodeCatalogMap(catalog)
	issues := make([]LintIssue, 0, len(gaps))
	for _, gap := range gaps {
		issues = append(issues, gap.Issue(StepName(g.FindNode(gap.NodeID), defs)))
	}
	return issues
}

func StepName(node *Node, defs map[NodeType]NodeDefinition) string {
	if node == nil {
		return ""
	}
	if name := node.DisplayName(); name != "" {
		return name
	}
	if def, ok := defs[node.Type]; ok && def.Label != "" {
		return def.Label
	}
	return node.ID
}

func (gap ChannelGap) Issue(nodeName string) LintIssue {
	channels := ChannelLabels(gap.Channels)
	need := gap.Need.Fact.Noun()
	subject := gap.Need.Subject
	if subject == "" {
		subject = need
	}
	issue := LintIssue{
		Code: gap.Code(), Severity: gap.Severity(), NodeID: gap.NodeID, Field: gap.Need.Field, Path: gap.Need.Path,
		Values: map[string]string{
			"channels": channels, "supported": ChannelLabels(ChannelsWith(gap.Need.Fact, ChannelGuaranteed)),
			"fact": string(gap.Need.Fact), "need": need, "subject": subject, "handle": gap.Need.Fallback,
		},
	}
	switch issue.Code {
	case LintChannelStepUnsupported:
		issue.Message = fmt.Sprintf("O passo %q usa %s, que %s não oferece; nesse canal o passo nunca funciona", nodeName, gap.Need.Fact.Noun(), channels)
		issue.Hint = fmt.Sprintf("Coloque o passo atrás de um nó Canal, ligado só à saída de %s, ou troque o passo por um que funcione em %s.", ChannelLabels(ChannelsWith(gap.Need.Fact, ChannelGuaranteed)), channels)
	case LintChannelStepFallsBack:
		issue.Message = fmt.Sprintf("O passo %q usa %s, que %s não oferece; nesse canal ele sempre segue pela saída %q", nodeName, gap.Need.Fact.Noun(), channels, gap.Need.Fallback)
		issue.Hint = "Confira se o caminho da saída de falha atende bem esses canais, ou coloque o passo atrás de um nó Canal."
	case LintChannelDataMayFail:
		issue.Message = fmt.Sprintf("O passo %q precisa de %s (%s), que o contato pode não ter em %s; sem esse dado o passo falha", nodeName, gap.Need.Fact.Noun(), subject, channels)
		issue.Hint = "Peça e grave esse dado do contato antes deste passo, ou coloque o passo atrás de um nó Canal para que só rode onde o dado existe."
	default:
		issue.Message = fmt.Sprintf("O passo %q usa %s, que pode ficar vazio ou trazer outro valor em %s", nodeName, subject, channels)
		issue.Hint = "Se o valor for importante, peça o dado ao contato antes deste passo ou coloque o passo atrás de um nó Canal."
	}
	return issue
}
