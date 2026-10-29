package copilot_usecase

import (
	"vozko/domain/ai"
	"vozko/domain/aichat"
	"vozko/usecases/agentloop"
)

func replayRows(msgs []*aichat.Message, model string, sees bool) ([]ai.Message, error) {
	out := make([]ai.Message, 0, len(msgs))
	var asked []*aichat.Message
	for _, m := range msgs {
		switch m.Role {
		case aichat.RoleUser:
			asked = append(asked, m)
			continue
		case aichat.RoleAssistant:
			turn, err := aichat.DecodeTranscript(m.Transcript)
			if err != nil {
				return nil, err
			}
			cut := len(asked) - min(interjections(turn), len(asked))
			out = append(out, requests(asked[:cut], sees)...)
			if len(turn) > 0 {
				out = append(out, withDeliveredRows(aichat.ReplayFor(turn, m.Model, model), asked[cut:], sees)...)
			} else if content := assistantHistoryContent(m); content != "" {
				out = append(out, ai.Message{Role: ai.RoleAssistant, Content: content})
			}
		case aichat.RoleSystem:
			out = append(out, requests(asked, sees)...)
			out = append(out, ai.Message{Role: ai.RoleSystem, Content: m.Content})
		default:
			out = append(out, requests(asked, sees)...)
		}
		asked = nil
	}
	return append(out, requests(asked, sees)...), nil
}

func requests(asked []*aichat.Message, sees bool) []ai.Message {
	out := make([]ai.Message, len(asked))
	for i, m := range asked {
		out[i] = userMessage(m, agentloop.UserRequest, sees)
	}
	return out
}

func interjections(turn []ai.Message) int {
	count := 0
	for _, m := range turn {
		if agentloop.Interjected(m) {
			count++
		}
	}
	return count
}

func withDeliveredRows(turn []ai.Message, delivered []*aichat.Message, sees bool) []ai.Message {
	out := make([]ai.Message, len(turn))
	unmatched := interjections(turn) - len(delivered)
	next := 0
	for i, m := range turn {
		if agentloop.Interjected(m) {
			if unmatched > 0 {
				unmatched--
			} else {
				m = userMessage(delivered[next], agentloop.Interjection, sees)
				next++
			}
		}
		out[i] = m
	}
	return out
}
