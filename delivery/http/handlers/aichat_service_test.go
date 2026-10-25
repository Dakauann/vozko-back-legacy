package handlers

import (
	"context"
	"errors"

	"vozko/domain/ai"
	aichat_usecase "vozko/usecases/aichat"
)

type idleChatAI struct{}

func (idleChatAI) Generate(context.Context, ai.GenerateInput) (*ai.GenerateOutput, error) {
	return nil, errors.New("no model in this test")
}

func (idleChatAI) GenerateStream(context.Context, ai.GenerateInput) (<-chan ai.StreamEvent, error) {
	return nil, errors.New("no model in this test")
}

func (idleChatAI) GetAvaibleModels(context.Context) ([]string, error) { return nil, nil }

func (idleChatAI) GetModelsWithPricing(context.Context) ([]ai.ModelInfo, error) { return nil, nil }

type openProjects struct{}

func (openProjects) BindProject(string, string) (bool, error) { return true, nil }

func chatService(d aichat_usecase.Deps) *aichat_usecase.Service {
	if d.Messages == nil {
		d.Messages = turnMessages{}
	}
	if d.AI == nil {
		d.AI = idleChatAI{}
	}
	if d.Funds == nil {
		d.Funds = aichat_usecase.NewFundsGate(fundedBalance{}, activePlan{})
	}
	if d.Charges == nil {
		d.Charges = pricedLedger{}
	}
	if d.Usage == nil {
		d.Usage = pricedUsage{}
	}
	if d.Projects == nil {
		d.Projects = openProjects{}
	}
	svc, err := aichat_usecase.NewService(d)
	if err != nil {
		panic(err)
	}
	return svc
}
