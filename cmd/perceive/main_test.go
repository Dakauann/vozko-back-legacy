package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

const surface = `{"canvas":{"width":1080,"height":1080,"background":"#ffffff"},"layers":[{"id":"a","type":"shape","shape":"rect","fill":"#111111","transform":{"x":0.5,"y":0.5,"w":0.3,"h":0.3,"rotation":0,"opacity":1},"effects":[{"id":"d","type":"drop-shadow","color":"#000000","opacity":1,"x":0,"y":30,"blur":0,"spread":0}]}],"texts":{}}`

func TestEachLineGetsTheAnswerOfItsOp(t *testing.T) {
	in := strings.Join([]string{
		`{"id":1,"op":"image","facts":{"approximate":false,"surface":` + surface + `}}`,
		`{"id":2,"op":"checks","facts":{"approximate":false,"artboards":[]}}`,
		`{"id":3,"op":"nope","facts":{"approximate":false}}`,
		`{"id":4,"op":"checks","facts":{"approximate":false}}`,
	}, "\n")
	var out bytes.Buffer
	if err := serve(strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("answers %v", lines)
	}
	var first struct {
		ID     int `json:"id"`
		Result struct {
			Depth struct {
				Findings []struct{ Kind string } `json:"findings"`
			} `json:"depth"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil || first.ID != 1 || len(first.Result.Depth.Findings) == 0 {
		t.Fatalf("image answer %s", lines[0])
	}
	if lines[1] != `{"id":2,"result":[]}` {
		t.Fatalf("checks answer %s", lines[1])
	}
	if !strings.Contains(lines[2], `"error":"unknown op`) || !strings.Contains(lines[3], `"error":"perception: the measured facts miss`) {
		t.Fatalf("errors %s %s", lines[2], lines[3])
	}
}
