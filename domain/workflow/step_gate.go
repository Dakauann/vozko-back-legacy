package workflow

type GatedStep struct {
	Result *NodeResult
	Pinned bool
}

type StepGate interface {
	BeforeStep(run *WorkflowRun, node *Node) (*GatedStep, error)
}

type PassThroughStepGate struct{}

func (PassThroughStepGate) BeforeStep(*WorkflowRun, *Node) (*GatedStep, error) { return nil, nil }
