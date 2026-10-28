package site

import (
	"encoding/json"
	"testing"
)

func TestChangesListWhatTheEditorMustReplace(t *testing.T) {
	before := map[string]File{"a.html": {Content: "a", Rev: 1}, "b.html": {Content: "b", Rev: 1}, "c.html": {Content: "c", Rev: 1}, "d.html": {Content: "d", Rev: 4}}
	after := map[string]File{"y.html": {Content: "a", Rev: 3}, "c.html": {Content: "c2", Rev: 2}, "n.html": {Content: "n", Rev: 1}, "d.html": {Content: "d", Rev: 4}}
	steps := []StepPatch{
		{Op: OpRenameFile, Renamed: []Rename{{From: "a.html", To: "x.html"}}},
		{Op: OpRenameFile, Renamed: []Rename{{From: "x.html", To: "y.html"}}},
		{Op: OpDeleteFile, Deleted: []string{"b.html"}},
	}
	raw, err := json.Marshal(Changes(before, after, steps))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"files":{"c.html":{"content":"c2","rev":2},"n.html":{"content":"n","rev":1},"y.html":{"content":"a","rev":3}},"deleted":["b.html"],"renamed":[{"from":"a.html","to":"y.html"}]}`
	if string(raw) != want {
		t.Fatalf("got  %s\nwant %s", raw, want)
	}
	empty, _ := json.Marshal(Changes(before, before, nil))
	if string(empty) != `{"files":{},"deleted":[],"renamed":[]}` {
		t.Fatalf("got %s", empty)
	}
}
