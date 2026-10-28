package workflow

import (
	"fmt"

	"vozko/domain/shared"
)

const CodeChannelUnsupported = "channel_unsupported"

func OffChannelStep(g *Graph, node *Node, entryType string) (*NodeResult, bool) {
	if g == nil || node == nil || !IsChannel(entryType) {
		return nil, false
	}
	channel := shared.EntryType(entryType)
	for _, need := range NodeChannelNeeds(node, nil) {
		if !need.Required || ChannelProvides(channel, need.Fact) != ChannelNever {
			continue
		}
		cause := fmt.Sprintf("%s: %s não oferece %s", CodeChannelUnsupported, ChannelBranchLabel(channel), need.Fact.Noun())
		output := map[string]interface{}{"sent": false, "error": cause, "error_code": CodeChannelUnsupported, "channel": entryType}
		for _, e := range g.OutgoingEdges(node.ID) {
			if need.Fallback != "" && e.Label == need.Fallback {
				return &NodeResult{NextNodeID: e.Target, Branch: e.Label, Output: output, ErrorKind: ErrorKindHandled}, true
			}
		}
		return &NodeResult{Output: output, Error: cause, ErrorKind: ErrorKindConfig}, true
	}
	return nil, false
}
