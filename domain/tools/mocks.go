package tools

import "context"

type Mock struct {
	Result  interface{}
	IsError bool
}

type mocksKey struct{}

func WithMocks(ctx context.Context, mocks map[string]Mock) context.Context {
	if len(mocks) == 0 {
		return ctx
	}
	return context.WithValue(ctx, mocksKey{}, mocks)
}

func MockFrom(ctx context.Context) (Mock, bool) {
	mocks, _ := ctx.Value(mocksKey{}).(map[string]Mock)
	ref, ok := InstanceFrom(ctx)
	if !ok || ref.ID == "" || mocks == nil {
		return Mock{}, false
	}
	mock, found := mocks[ref.ID]
	return mock, found
}
