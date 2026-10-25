package tools

import (
	"context"
	"strings"
)

type callIDKey struct{}

func WithCallID(ctx context.Context, callID string) context.Context {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return ctx
	}
	return context.WithValue(ctx, callIDKey{}, callID)
}

func CallIDFrom(ctx context.Context) string {
	callID, _ := ctx.Value(callIDKey{}).(string)
	return callID
}

type instanceKey struct{}

func WithInstance(ctx context.Context, ref *InstanceRef) context.Context {
	if ref == nil {
		return ctx
	}
	return context.WithValue(ctx, instanceKey{}, *ref)
}

func InstanceFrom(ctx context.Context) (InstanceRef, bool) {
	ref, ok := ctx.Value(instanceKey{}).(InstanceRef)
	return ref, ok
}

func Offered(defs []Definition, name string) (Definition, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	for _, def := range defs {
		if strings.ToLower(def.Name) == key {
			return def, true
		}
	}
	return Definition{}, false
}

func (d Definition) Handler() string {
	if d.HandlerName != "" {
		return d.HandlerName
	}
	return d.Name
}

type runIDKey struct{}

func WithRunID(ctx context.Context, runID string) context.Context {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return ctx
	}
	return context.WithValue(ctx, runIDKey{}, runID)
}

func RunIDFrom(ctx context.Context) string {
	runID, _ := ctx.Value(runIDKey{}).(string)
	return runID
}
