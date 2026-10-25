package workflow

import (
	"errors"
	"fmt"

	"vozko/domain/testinput"
	"vozko/domain/tracepayload"
)

type TestInput = testinput.TestInput

var (
	ErrTestInputInvalid    = errors.New("workflow: invalid saved test input")
	ErrTestInputNotAllowed = fmt.Errorf("%w: only trigger nodes keep test inputs", ErrTestInputInvalid)
	ErrTestInputTooLarge   = fmt.Errorf("%w: input is larger than %d bytes", ErrTestInputInvalid, tracepayload.MaxBytes)
)

func cloneTestInput(in TestInput) TestInput {
	cp := in
	cp.Input = cloneConfigMap(in.Input)
	return cp
}

func cloneTestInputs(inputs []TestInput) []TestInput {
	if inputs == nil {
		return nil
	}
	out := make([]TestInput, len(inputs))
	for i, in := range inputs {
		out[i] = cloneTestInput(in)
	}
	return out
}

func ValidateTestInputs(g *Graph) error {
	for _, n := range g.Nodes {
		if err := validateNodeTestInputs(n); err != nil {
			return err
		}
	}
	return nil
}

func validateNodeTestInputs(n Node) error {
	if len(n.TestInputs) == 0 {
		return nil
	}
	if !n.Type.IsTrigger() {
		return fmt.Errorf("%w: node %s (%s)", ErrTestInputNotAllowed, n.ID, n.Type)
	}
	err := testinput.Validate(n.TestInputs)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, testinput.ErrTooLarge):
		return fmt.Errorf("%w: node %s: %v", ErrTestInputTooLarge, n.ID, err)
	default:
		return fmt.Errorf("%w: node %s: %v", ErrTestInputInvalid, n.ID, err)
	}
}

func (g *Graph) HasDraftOnly() bool {
	for _, n := range g.Nodes {
		if n.Pinned != nil || len(n.TestInputs) > 0 {
			return true
		}
	}
	return false
}

func StripDraftOnly(g Graph) Graph {
	if !g.HasDraftOnly() {
		return g
	}
	out := Graph{Nodes: make([]Node, len(g.Nodes)), Edges: g.Edges}
	for i, n := range g.Nodes {
		n.Pinned = nil
		n.TestInputs = nil
		out.Nodes[i] = n
	}
	return out
}
