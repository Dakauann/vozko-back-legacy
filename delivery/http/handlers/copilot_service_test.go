package handlers

import (
	"context"
	"time"

	"vozko/domain/aichat"
	"vozko/domain/copilot"
	"vozko/domain/media"
	"vozko/domain/readiness"
	"vozko/domain/workspace"
	copilot_usecase "vozko/usecases/copilot"
)

type grantedAccess struct{}

func (grantedAccess) Execute(string, string, workspace.Resource, workspace.Action) error { return nil }

type fundedCopilot struct{}

func (fundedCopilot) Check(string) error { return nil }

type noFiles struct{}

func (noFiles) GetMedia(string, string) (*media.Media, error) { return nil, media.ErrMediaNotFound }

type noState struct{}

func (noState) Snapshot(context.Context, readiness.Person) (*readiness.Snapshot, error) {
	return nil, nil
}

type silentScreens struct{}

func (silentScreens) Expect(string) (func(context.Context) (copilot.ScreenReply, error), func()) {
	return func(ctx context.Context) (copilot.ScreenReply, error) {
		<-ctx.Done()
		return copilot.ScreenReply{}, ctx.Err()
	}, func() {}
}

func (silentScreens) Deliver(string, copilot.ScreenReply) error { return copilot.ErrNoScreen }

type emptyQueue struct{}

func (emptyQueue) Enqueue(*aichat.QueuedMessage) error             { return nil }
func (emptyQueue) Pending(string) ([]*aichat.QueuedMessage, error) { return nil, nil }
func (emptyQueue) Remove(string, string) error                     { return aichat.ErrQueuedMessageGone }
func (emptyQueue) Deliver(string) ([]*aichat.QueuedMessage, error) { return nil, nil }

type keptProjects struct{}

func (keptProjects) AdoptProject(string, string) error { return nil }

func copilotService(d copilot_usecase.Deps) *copilot_usecase.Service {
	if d.Registry == nil {
		d.Registry = copilot_usecase.NewRegistry()
	}
	if d.Access == nil {
		d.Access = grantedAccess{}
	}
	if d.Funds == nil {
		d.Funds = fundedCopilot{}
	}
	if d.Messages == nil {
		d.Messages = turnMessages{}
	}
	if d.Files == nil {
		d.Files = noFiles{}
	}
	if d.State == nil {
		d.State = noState{}
	}
	if d.NewID == nil {
		d.NewID = func() string { return time.Now().Format(time.RFC3339Nano) }
	}
	if d.Screens == nil {
		d.Screens = silentScreens{}
	}
	if d.Queue == nil {
		d.Queue = emptyQueue{}
	}
	if d.Adopter == nil {
		d.Adopter = keptProjects{}
	}
	svc, err := copilot_usecase.NewService(d)
	if err != nil {
		panic(err)
	}
	return svc
}
