package copilot

import (
	"errors"
	"fmt"
	"strings"
)

type Mode string

const (
	ModeAsk  Mode = "ask"
	ModeEdit Mode = "edit"
	ModeFull Mode = "full"
)

var ErrInvalidMode = errors.New("copilot: unknown execution mode")

func ParseMode(raw string) (Mode, error) {
	switch mode := Mode(strings.TrimSpace(raw)); mode {
	case "":
		return ModeAsk, nil
	case ModeAsk, ModeEdit, ModeFull:
		return mode, nil
	}
	return "", fmt.Errorf("%w: %q", ErrInvalidMode, raw)
}

type Graded interface {
	NeedsApproval(mode Mode) bool
}

func NeedsApproval(tool Tool, mode Mode) bool {
	if graded, ok := tool.(Graded); ok {
		return graded.NeedsApproval(mode)
	}
	return tool.Meta().Mutating
}

type ArgumentGraded interface {
	NeedsApprovalFor(mode Mode, args map[string]interface{}) bool
}

func NeedsApprovalFor(tool Tool, mode Mode, args map[string]interface{}) bool {
	if graded, ok := tool.(ArgumentGraded); ok && graded.NeedsApprovalFor(mode, args) {
		return true
	}
	return NeedsApproval(tool, mode)
}
