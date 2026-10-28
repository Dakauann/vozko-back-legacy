package site

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"vozko/domain/inlinemarkup"
	"vozko/domain/strictjson"
)

const contractPath = "testdata/edit_contract.json"

var updateContract = flag.Bool("update-contract", false, "rewrite the expected results of testdata/edit_contract.json")

type contractBatch struct {
	Name       string            `json:"name"`
	Base       string            `json:"base"`
	Operations json.RawMessage   `json:"operations"`
	Accepted   []json.RawMessage `json:"accepted,omitempty"`
	Ignored    []Ignored         `json:"ignored,omitempty"`
	Refusal    *BatchError       `json:"refusal,omitempty"`
	Malformed  bool              `json:"malformed,omitempty"`
	Executed   json.RawMessage   `json:"executed,omitempty"`
}

type executedFailure struct {
	Index  int    `json:"index"`
	Code   string `json:"code"`
	Field  string `json:"field,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type contractExecution struct {
	Texts    map[string][]inlinemarkup.Run `json:"texts,omitempty"`
	Media    map[string]string             `json:"media,omitempty"`
	Sections map[string]string             `json:"sections,omitempty"`
	Steps    json.RawMessage               `json:"steps,omitempty"`
	Final    map[string]File               `json:"final,omitempty"`
	Failures []executedFailure             `json:"failures,omitempty"`
	Skipped  []Skip                        `json:"skipped,omitempty"`
}

type contractReplay struct {
	Name    string          `json:"name"`
	Base    string          `json:"base"`
	Steps   json.RawMessage `json:"steps"`
	Final   map[string]File `json:"final,omitempty"`
	Refusal string          `json:"refusal,omitempty"`
}

type contract struct {
	Bases   map[string]Project `json:"bases"`
	Batches []contractBatch    `json:"batches"`
	Replays []contractReplay   `json:"replays"`
}

func runContractBatch(t *testing.T, bases map[string]Project, c contractBatch) contractBatch {
	t.Helper()
	base, ok := bases[c.Base]
	if !ok {
		t.Fatalf("%s: unknown base %q", c.Name, c.Base)
	}
	out := contractBatch{Name: c.Name, Base: c.Base, Operations: c.Operations, Executed: c.Executed}
	batch, err := PrepareBatch(base, c.Operations)
	var refused *BatchError
	switch {
	case errors.Is(err, ErrBatchMalformed):
		out.Malformed = true
	case errors.As(err, &refused):
		out.Refusal = refused
		out.Ignored = batch.Ignored
	case err != nil:
		t.Fatal(err)
	default:
		for _, o := range batch.Operations {
			var raw bytes.Buffer
			enc := json.NewEncoder(&raw)
			enc.SetEscapeHTML(false)
			if err := enc.Encode(o); err != nil {
				t.Fatal(err)
			}
			out.Accepted = append(out.Accepted, bytes.TrimSpace(raw.Bytes()))
		}
		out.Ignored = batch.Ignored
	}
	return out
}

func runContractReplay(t *testing.T, bases map[string]Project, c contractReplay) contractReplay {
	t.Helper()
	base, ok := bases[c.Base]
	if !ok {
		t.Fatalf("%s: unknown base %q", c.Name, c.Base)
	}
	out := contractReplay{Name: c.Name, Base: c.Base, Steps: c.Steps}
	steps, err := DecodeSteps(c.Steps)
	if err == nil {
		var final Project
		final, err = ApplySteps(base, steps)
		if err == nil {
			out.Final = final.Files
		}
	}
	if err != nil {
		out.Refusal = ErrorCode(err)
		if out.Refusal == "" {
			t.Fatal(err)
		}
	}
	return out
}

func sameJSON(t *testing.T, name string, want, have any) {
	t.Helper()
	left, errA := json.Marshal(want)
	right, errB := json.Marshal(have)
	if errA != nil || errB != nil {
		t.Fatal(errA, errB)
	}
	if !bytes.Equal(left, right) {
		t.Errorf("%s differs\nwant %s\ngot  %s", name, left, right)
	}
}

func TestTheEditContractFixtureMatchesGo(t *testing.T) {
	raw, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	var fixture contract
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	for name, base := range fixture.Bases {
		if issues := Validate(base); len(issues) != 0 {
			t.Fatalf("base %s must be valid: %+v", name, issues)
		}
	}
	computed := contract{Bases: fixture.Bases}
	for _, c := range fixture.Batches {
		got := runContractBatch(t, fixture.Bases, c)
		computed.Batches = append(computed.Batches, got)
		if !*updateContract {
			sameJSON(t, c.Name, c, got)
		}
	}
	for _, c := range fixture.Replays {
		got := runContractReplay(t, fixture.Bases, c)
		computed.Replays = append(computed.Replays, got)
		if !*updateContract {
			sameJSON(t, c.Name, c, got)
		}
	}
	if *updateContract {
		var encoded bytes.Buffer
		enc := json.NewEncoder(&encoded)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(computed); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(contractPath, encoded.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func loadContract(t *testing.T) contract {
	t.Helper()
	raw, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	var fixture contract
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func expectedRuns(t *testing.T, text string) []inlinemarkup.Run {
	t.Helper()
	if strings.TrimSpace(text) == "" {
		return []inlinemarkup.Run{}
	}
	paragraphs, err := inlinemarkup.Parse(text)
	if err != nil || len(paragraphs) != 1 {
		t.Fatalf("text %q must be one paragraph: %v", text, err)
	}
	return paragraphs[0].Runs
}

func TestTheBuilderExecutionsInTheContractApplyInGo(t *testing.T) {
	fixture := loadContract(t)
	executed := 0
	for _, c := range fixture.Batches {
		if len(c.Executed) == 0 {
			continue
		}
		executed++
		var run contractExecution
		if err := strictjson.Decode(c.Executed, &run); err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		if len(c.Accepted) == 0 {
			t.Fatalf("%s: only accepted batches reach the builder", c.Name)
		}
		batch, err := PrepareBatch(fixture.Bases[c.Base], c.Operations)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		for i, o := range batch.Operations {
			if o.Op != OpSetText {
				continue
			}
			sameJSON(t, c.Name+" text "+strconv.Itoa(i), expectedRuns(t, *o.Text), run.Texts[strconv.Itoa(i)])
		}
		if (len(run.Steps) == 0) == (len(run.Failures) == 0) {
			t.Fatalf("%s: an execution has steps or failures", c.Name)
		}
		for _, f := range run.Failures {
			if !slices.Contains(Codes(), f.Code) {
				t.Fatalf("%s: unknown failure code %q", c.Name, f.Code)
			}
		}
		if len(run.Steps) == 0 {
			continue
		}
		steps, err := DecodeSteps(run.Steps)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		final, err := ApplySteps(fixture.Bases[c.Base], steps)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		sameJSON(t, c.Name+" final", run.Final, final.Files)
		if issues := Validate(final); len(issues) != 0 {
			t.Fatalf("%s: the executed project must stay valid: %+v", c.Name, issues)
		}
	}
	if executed == 0 {
		t.Fatal("the contract must hold builder executions")
	}
}
