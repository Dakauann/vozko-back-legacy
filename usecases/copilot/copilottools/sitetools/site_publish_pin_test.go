package sitetools

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"vozko/domain/copilot"
	sitepublish_usecase "vozko/usecases/sitepublish"
)

type fakePins struct {
	values map[string]string
	err    error
}

func (f *fakePins) SetNX(key, value string, _ time.Duration) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	if _, ok := f.values[key]; ok {
		return false, nil
	}
	f.values[key] = value
	return true, nil
}

func (f *fakePins) GetString(key string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.values[key], nil
}

func proposed(r *rig, id string) copilot.Context {
	cc := r.cc
	cc.ProposalID = id
	return cc
}

func TestThePublishCardPinsTheVersionItShowed(t *testing.T) {
	r := newRig(t)
	tool := NewSitePublishTool(r.deps)
	cc := proposed(r, "proposal-1")
	tool.(copilot.Describer).Describe(context.Background(), cc, map[string]interface{}{})
	r.storage.view.Project.Version = 9
	tool.(copilot.Describer).Describe(context.Background(), cc, map[string]interface{}{})
	if res := tool.Execute(context.Background(), cc, map[string]interface{}{}); res.Status != copilot.StatusOK {
		t.Fatalf("got %+v", res)
	}
	if r.publisher.pinned[0] != 3 {
		t.Fatalf("the version on the card is the one published against: %v", r.publisher.pinned)
	}
}

func TestAPublishWithoutItsPinOrOnAChangedPageRefuses(t *testing.T) {
	r := newRig(t)
	tool := NewSitePublishTool(r.deps)
	res := tool.Execute(context.Background(), proposed(r, "never-shown"), map[string]interface{}{})
	if res.Status == copilot.StatusOK || !strings.Contains(res.Message, publishStale) || len(r.publisher.pinned) != 0 {
		t.Fatalf("got %+v", res)
	}
	r.publisher.err = sitepublish_usecase.ErrPublishStale
	cc := proposed(r, "proposal-2")
	tool.(copilot.Describer).Describe(context.Background(), cc, map[string]interface{}{})
	if res := tool.Execute(context.Background(), cc, map[string]interface{}{}); res.Status == copilot.StatusOK || !strings.Contains(res.Message, publishStale) {
		t.Fatalf("got %+v", res)
	}
	r.publisher.err = errors.Join(sitepublish_usecase.ErrPublishInProgress)
	if res := tool.Execute(context.Background(), cc, map[string]interface{}{}); !strings.Contains(res.Message, "já está sendo publicado") {
		t.Fatalf("got %+v", res)
	}
}
