package aichat_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/ai"
	"vozko/domain/aichat"
	"vozko/domain/shared/deps"
)

type quietAI struct{}

func (quietAI) Generate(context.Context, ai.GenerateInput) (*ai.GenerateOutput, error) {
	return nil, errors.New("no model in this test")
}

func (quietAI) GenerateStream(context.Context, ai.GenerateInput) (<-chan ai.StreamEvent, error) {
	return nil, errors.New("no model in this test")
}

func (quietAI) GetAvaibleModels(context.Context) ([]string, error) { return nil, nil }

func (quietAI) GetModelsWithPricing(context.Context) ([]ai.ModelInfo, error) { return nil, nil }

type quietMessages struct{}

func (quietMessages) Create(*aichat.Message) error { return nil }
func (quietMessages) ListByThread(aichat.ListMessagesInput) ([]*aichat.Message, int64, error) {
	return nil, 0, nil
}
func (quietMessages) DeleteByThread(string) error { return nil }
func (quietMessages) ClaimProposal(string, string, aichat.ProposalStatus) (*aichat.Message, error) {
	return nil, nil
}
func (quietMessages) ExpireProposals(string) error            { return nil }
func (quietMessages) HasPendingProposal(string) (bool, error) { return false, nil }

func completeDeps() Deps {
	return Deps{
		Threads:  &costThreads{threads: map[string]*aichat.Thread{}},
		Messages: quietMessages{},
		AI:       quietAI{},
		Funds:    NewFundsGate(&fakeLedger{balance: 1 << 40}, active),
		Charges:  &ledger{},
		Usage:    &usageLedger{},
		Projects: &projectBinder{bound: map[string]string{}},
	}
}

func testService(d Deps) *Service {
	full := completeDeps()
	if d.Threads != nil {
		full.Threads = d.Threads
	}
	if d.Messages != nil {
		full.Messages = d.Messages
	}
	if d.AI != nil {
		full.AI = d.AI
	}
	if d.Funds != nil {
		full.Funds = d.Funds
	}
	if d.Charges != nil {
		full.Charges = d.Charges
	}
	if d.Usage != nil {
		full.Usage = d.Usage
	}
	if d.Projects != nil {
		full.Projects = d.Projects
	}
	svc, err := NewService(full)
	if err != nil {
		panic(err)
	}
	return svc
}

func TestNewServiceRefusesEveryMissingDependency(t *testing.T) {
	var typedNilFunds *FundsGate
	cases := map[string]func(*Deps){
		"threads":         func(d *Deps) { d.Threads = nil },
		"messages":        func(d *Deps) { d.Messages = nil },
		"ai service":      func(d *Deps) { d.AI = nil },
		"funds gate":      func(d *Deps) { d.Funds = typedNilFunds },
		"charge ledger":   func(d *Deps) { d.Charges = nil },
		"usage ledger":    func(d *Deps) { d.Usage = nil },
		"thread projects": func(d *Deps) { d.Projects = nil },
	}
	for name, drop := range cases {
		t.Run(name, func(t *testing.T) {
			d := completeDeps()
			drop(&d)
			svc, err := NewService(d)
			var missing *deps.MissingError
			if svc != nil || !errors.As(err, &missing) || len(missing.Missing) != 1 || missing.Missing[0] != name {
				t.Fatalf("svc = %v err = %v, want %s reported missing", svc, err, name)
			}
		})
	}
}
