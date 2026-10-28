package sitetools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"vozko/domain/copilot"
	"vozko/usecases/siteinspect"
)

type memoryInspections struct {
	values map[string]string
}

func (m *memoryInspections) GetString(key string) (string, error) { return m.values[key], nil }

func (m *memoryInspections) SetString(key, value string, _ time.Duration) error {
	m.values[key] = value
	return nil
}

func gapsOf(t *testing.T, data interface{}) (interface{}, interface{}) {
	t.Helper()
	body := decoded(t, data)
	return body["open_gaps"], body["open_gaps_unread"]
}

func TestGapsAreReadFromTheDocumentsTheOpenEditorRenders(t *testing.T) {
	r := newRig(t)
	r.editor.pages = allRoutes(gappedHome)
	res := NewSiteReadTool(r.deps).Execute(context.Background(), r.cc, map[string]interface{}{})
	gaps, unread := gapsOf(t, res.Data)
	if res.Status != copilot.StatusOK || unread != nil || len(gaps.([]interface{})) != 2 {
		t.Fatalf("got %+v", res.Data)
	}
	sent := r.editor.sent(copilot.ScreenInspect)
	if len(sent) != 1 {
		t.Fatalf("sent %+v", r.editor.commands)
	}
	order := sent[0].Args.(inspectOrder)
	if order.PreviewURL != previewURL || strings.Join(order.Routes, ",") != "/,/obrigado,/privacidade" {
		t.Fatalf("order %+v", order)
	}
}

func TestGapsStayUnreadWhenTheEditorAnswersWithSomethingElse(t *testing.T) {
	cases := map[string]copilot.ScreenReply{
		"stale stamp":   {OK: true, Data: mustJSON(t, inspectReply{Documents: map[string]string{"/": cleanHome, "/obrigado": cleanHome, "/privacidade": cleanHome}})},
		"extra field":   {OK: true, Data: []byte(`{"documents":{},"images":[]}`)},
		"missing route": {OK: true, Data: []byte(`{"documents":{"/":"<html></html>"}}`)},
		"refused":       {OK: false, Error: &copilot.ScreenError{Code: "too_large", Message: "grande demais"}},
	}
	for name, reply := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			r.editor.replies = map[copilot.ScreenCommandName]copilot.ScreenReply{copilot.ScreenInspect: reply}
			res := NewSiteReadTool(r.deps).Execute(context.Background(), r.cc, map[string]interface{}{})
			gaps, unread := gapsOf(t, res.Data)
			if res.Status != copilot.StatusOK || gaps != nil || unread == nil {
				t.Fatalf("got %+v", res.Data)
			}
		})
	}
	r := newRig(t)
	r.editor.replies = map[copilot.ScreenCommandName]copilot.ScreenReply{copilot.ScreenInspect: cases["stale stamp"]}
	_, unread := gapsOf(t, NewSiteReadTool(r.deps).Execute(context.Background(), r.cc, map[string]interface{}{}).Data)
	if unread != siteinspect.ReasonStale {
		t.Fatalf("a document built from older files says so: %v", unread)
	}
}

func TestOneBuildIsCapturedOnceAcrossTools(t *testing.T) {
	r := newRig(t)
	r.deps.Inspections = &memoryInspections{values: map[string]string{}}
	r.storage.build = "build-1"
	NewSiteReadTool(r.deps).Execute(context.Background(), r.cc, map[string]interface{}{})
	NewSiteReadTool(r.deps).Execute(context.Background(), r.cc, map[string]interface{}{})
	if got := len(r.editor.sent(copilot.ScreenInspect)); got != 1 {
		t.Fatalf("the second read reuses the advisory inspection of the same build: %d", got)
	}
}

func mustJSON(t *testing.T, value interface{}) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
