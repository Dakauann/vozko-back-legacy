package automationneeds

import (
	"fmt"
	"strings"

	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/domain/workflow"
)

type ChannelBlock struct {
	Automation Link
	Via        []Link
	NodeID     string
	Step       string
	Fact       workflow.ChannelFact
	Channels   []shared.EntryType
}

type ChannelWalk struct {
	Blocks  []ChannelBlock
	Reached map[conversation.Automation][]shared.EntryType
}

func (w ChannelWalk) Channels() []shared.EntryType {
	set := map[shared.EntryType]bool{}
	for _, block := range w.Blocks {
		for _, channel := range block.Channels {
			set[channel] = true
		}
	}
	return workflow.SortedChannels(set)
}

func (b ChannelBlock) Through(a conversation.Automation) bool {
	for _, link := range b.Via {
		if link.Automation == a {
			return true
		}
	}
	return false
}

func ChannelWalkOf(src Source, workspaceID string, root conversation.Automation, channels []shared.EntryType) (ChannelWalk, error) {
	w := channelWalker{src: src, workspaceID: workspaceID, reached: map[conversation.Automation]map[shared.EntryType]bool{}, blocked: map[string]int{}}
	if err := w.visit(root, nil, channels, false); err != nil {
		return ChannelWalk{}, err
	}
	out := ChannelWalk{Blocks: w.blocks, Reached: map[conversation.Automation][]shared.EntryType{}}
	for a, set := range w.reached {
		if len(set) > 0 {
			out.Reached[a] = workflow.SortedChannels(set)
		}
	}
	return out, nil
}

type channelWalker struct {
	src         Source
	workspaceID string
	reached     map[conversation.Automation]map[shared.EntryType]bool
	blocks      []ChannelBlock
	blocked     map[string]int
}

func (w *channelWalker) fresh(a conversation.Automation, channels []shared.EntryType) []shared.EntryType {
	set := w.reached[a]
	if set == nil {
		set = map[shared.EntryType]bool{}
		w.reached[a] = set
	}
	var added []shared.EntryType
	for _, channel := range channels {
		if !set[channel] {
			set[channel] = true
			added = append(added, channel)
		}
	}
	return added
}

func (w *channelWalker) visit(a conversation.Automation, via []Link, channels []shared.EntryType, called bool) error {
	a.ID = strings.TrimSpace(a.ID)
	added := w.fresh(a, channels)
	if len(added) == 0 {
		return nil
	}
	switch a.Kind {
	case conversation.AutomationAgent:
		return w.agent(a, via, added)
	case conversation.AutomationWorkflow:
		return w.workflow(a, via, added, called)
	}
	return fmt.Errorf("%w: %q has no kind", ErrNotFound, a.ID)
}

func (w *channelWalker) agent(a conversation.Automation, via []Link, channels []shared.EntryType) error {
	found, err := w.src.Agent(a.ID)
	if err != nil {
		return fmt.Errorf("%w: agent %s: %w", ErrUnreadable, a.ID, err)
	}
	if found == nil || found.WorkspaceID != w.workspaceID {
		return nil
	}
	path := extend(via, Link{Automation: a, Name: found.Name})
	for _, tool := range found.InternalTools {
		if tool.Name != workflow.AgentRunWorkflowTool {
			continue
		}
		if id, ok := followable(tool.Config, "workflow_id"); ok {
			if err := w.visit(conversation.Automation{Kind: conversation.AutomationWorkflow, ID: id}, path, channels, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *channelWalker) workflow(a conversation.Automation, via []Link, channels []shared.EntryType, called bool) error {
	found, err := w.src.Workflow(a.ID)
	if err != nil {
		return fmt.Errorf("%w: workflow %s: %w", ErrUnreadable, a.ID, err)
	}
	if found == nil || found.WorkspaceID != w.workspaceID {
		return nil
	}
	path := extend(via, Link{Automation: a, Name: found.Name})
	reach := workflow.ChannelReachOf(&found.Graph, channels)
	if called {
		reach = workflow.CalledChannelReachOf(&found.Graph, channels)
	}
	for _, gap := range workflow.ChannelGapsOf(&found.Graph, reach, nil) {
		if gap.Blocks() {
			w.block(path, &found.Graph, gap)
		}
	}
	for _, node := range found.Graph.Nodes {
		next, runs := runsAutomation(node)
		if !runs || len(reach[node.ID]) == 0 {
			continue
		}
		if err := w.visit(next, path, reach[node.ID], true); err != nil {
			return err
		}
	}
	return nil
}

func (w *channelWalker) block(path []Link, g *workflow.Graph, gap workflow.ChannelGap) {
	link := path[len(path)-1]
	key := link.ID + "\x00" + gap.NodeID + "\x00" + string(gap.Need.Fact)
	if at, seen := w.blocked[key]; seen {
		merged := map[shared.EntryType]bool{}
		for _, channel := range append(w.blocks[at].Channels, gap.Channels...) {
			merged[channel] = true
		}
		w.blocks[at].Channels = workflow.SortedChannels(merged)
		return
	}
	w.blocked[key] = len(w.blocks)
	w.blocks = append(w.blocks, ChannelBlock{Automation: link, Via: append([]Link(nil), path...), NodeID: gap.NodeID, Step: workflow.StepName(g.FindNode(gap.NodeID), nil), Fact: gap.Need.Fact, Channels: gap.Channels})
}

func ChannelRefusal(root conversation.Automation, walk ChannelWalk) error {
	if len(walk.Blocks) == 0 {
		return nil
	}
	return &Refusal{Cause: ErrChannelUnsupported, Automation: rootLinkOf(root, walk.Blocks), Channels: walk.Blocks}
}

func rootLinkOf(root conversation.Automation, blocks []ChannelBlock) Link {
	for _, block := range blocks {
		if block.Automation.Automation == root {
			return block.Automation
		}
	}
	return Link{Automation: root}
}

func channelSentence(blocks []ChannelBlock) string {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		who := AutomationNoun(block.Automation.Automation)
		if block.Automation.Name != "" {
			who += " " + block.Automation.Name
		}
		parts = append(parts, fmt.Sprintf("o passo %q d%s usa %s, que %s não oferece", block.Step, who, block.Fact.Noun(), workflow.ChannelLabels(block.Channels)))
	}
	return capitalize(strings.Join(parts, "; ")) + ". Coloque o passo atrás de um nó Canal ou escolha outra automação para esse canal."
}
