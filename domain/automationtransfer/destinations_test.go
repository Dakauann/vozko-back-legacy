package automationtransfer

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"vozko/domain/conversation"
	"vozko/domain/stage"
)

func agentTarget(id string) conversation.Automation {
	return conversation.Automation{Kind: conversation.AutomationAgent, ID: id}
}

func workflowTarget(id string) conversation.Automation {
	return conversation.Automation{Kind: conversation.AutomationWorkflow, ID: id}
}

func destinationsConfig(items ...map[string]interface{}) map[string]interface{} {
	list := make([]interface{}, 0, len(items))
	for _, item := range items {
		list = append(list, item)
	}
	return map[string]interface{}{DestinationsKey: list}
}

func sequentialIDs() func() string {
	n := 0
	return func() string {
		n++
		return []string{"d1", "d2", "d3", "d4"}[n-1]
	}
}

func TestNormalizingGivesEveryDestinationAnIDAndKeepsTheUsersOrder(t *testing.T) {
	config := destinationsConfig(
		map[string]interface{}{"kind": "agent", "automation_id": " ag-academico ", "stage_id": "st-centro", "description": "  Alunos do polo Centro.  "},
		map[string]interface{}{"id": "keep", "kind": "workflow", "automation_id": "wf-matricula"},
	)

	got, err := Normalize(config, sequentialIDs())

	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	want := map[string]interface{}{DestinationsKey: []interface{}{
		map[string]interface{}{"id": "d1", "kind": "agent", "automation_id": "ag-academico", "stage_id": "st-centro", "description": "Alunos do polo Centro."},
		map[string]interface{}{"id": "keep", "kind": "workflow", "automation_id": "wf-matricula"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
}

func TestNormalizingGivesARepeatedIDAFreshOne(t *testing.T) {
	config := destinationsConfig(
		map[string]interface{}{"id": "same", "kind": "agent", "automation_id": "a"},
		map[string]interface{}{"id": "same", "kind": "agent", "automation_id": "b", "stage_id": "s"},
	)
	got, err := Normalize(config, sequentialIDs())
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	parsed, err := Parse(got)
	if err != nil || parsed[0].ID != "same" || parsed[1].ID != "d1" {
		t.Fatalf("parsed %+v, err %v", parsed, err)
	}
}

func TestNormalizingRefusesWhatCannotBeRead(t *testing.T) {
	long := strings.Repeat("a", MaxDescriptionRunes+1)
	many := make([]map[string]interface{}, MaxDestinations+1)
	for i := range many {
		many[i] = map[string]interface{}{"kind": "agent", "automation_id": "a"}
	}
	cases := map[string]struct {
		config map[string]interface{}
		want   error
	}{
		"not a list":        {map[string]interface{}{DestinationsKey: "agent"}, ErrConfigUnreadable},
		"item not a map":    {map[string]interface{}{DestinationsKey: []interface{}{"agent"}}, ErrConfigUnreadable},
		"unknown kind":      {destinationsConfig(map[string]interface{}{"kind": "person", "automation_id": "u"}), ErrConfigUnreadable},
		"no automation":     {destinationsConfig(map[string]interface{}{"kind": "agent", "automation_id": " "}), ErrConfigUnreadable},
		"unknown key":       {destinationsConfig(map[string]interface{}{"kind": "agent", "automation_id": "a", "label": "x"}), ErrConfigUnreadable},
		"value not text":    {destinationsConfig(map[string]interface{}{"kind": "agent", "automation_id": "a", "stage_id": 3}), ErrConfigUnreadable},
		"long description":  {destinationsConfig(map[string]interface{}{"kind": "agent", "automation_id": "a", "description": long}), ErrDescriptionTooLong},
		"many destinations": {destinationsConfig(many...), ErrTooManyDestinations},
	}
	for name, c := range cases {
		if _, err := Normalize(c.config, sequentialIDs()); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}
}

func TestAStoredDestinationWithoutAnIDIsUnreadable(t *testing.T) {
	if _, err := Parse(destinationsConfig(map[string]interface{}{"kind": "agent", "automation_id": "a"})); !errors.Is(err, ErrConfigUnreadable) {
		t.Fatalf("err = %v", err)
	}
}

func TestNoDestinationsParsesAsAnEmptyList(t *testing.T) {
	for _, config := range []map[string]interface{}{nil, {}, destinationsConfig()} {
		got, err := Parse(config)
		if err != nil || len(got) != 0 {
			t.Fatalf("config %v: got %v, err %v", config, got, err)
		}
	}
}

func TestCheckingTheListRefusesNoneItselfAndRepeats(t *testing.T) {
	academic := Destination{ID: "d1", Target: agentTarget("ag-academico"), StageID: "st-centro"}
	cases := map[string]struct {
		list []Destination
		want error
	}{
		"none":                         {nil, ErrNoDestinations},
		"itself":                       {[]Destination{{ID: "d1", Target: agentTarget("ag-self")}}, ErrSelf},
		"same target and stage":        {[]Destination{academic, {ID: "d2", Target: agentTarget("ag-academico"), StageID: "st-centro"}}, ErrDuplicateDestination},
		"same agent in another funnel": {[]Destination{academic, {ID: "d2", Target: agentTarget("ag-academico"), StageID: "st-norte"}}, nil},
		"a workflow with the same id":  {[]Destination{{ID: "d1", Target: workflowTarget("ag-self")}}, nil},
	}
	for name, c := range cases {
		if err := Check(c.list, "ag-self"); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}
}

func TestFindingADestinationByTheIDTheModelChose(t *testing.T) {
	list := []Destination{{ID: "d1", Target: agentTarget("a")}, {ID: "d2", Target: workflowTarget("w")}}
	if got, ok := Find(list, " d2 "); !ok || got.Target != workflowTarget("w") {
		t.Fatalf("got %+v, %v", got, ok)
	}
	if _, ok := Find(list, "ag-academico"); ok {
		t.Fatal("an id outside the list must not be found")
	}
}

func TestTheHandOffNoteIsRequiredAndBounded(t *testing.T) {
	if _, err := NoteOf("   "); !errors.Is(err, ErrNoteMissing) {
		t.Fatalf("err = %v", err)
	}
	got, err := NoteOf("  " + strings.Repeat("é", MaxNoteRunes+20) + "  ")
	if err != nil || len([]rune(got)) != MaxNoteRunes {
		t.Fatalf("got %d runes, err %v", len([]rune(got)), err)
	}
}

func TestTheLoopCapCountsEveryTransferInTheWindow(t *testing.T) {
	if LoopReached(LoopCap - 1) {
		t.Fatal("below the cap a transfer goes on")
	}
	if !LoopReached(LoopCap) {
		t.Fatal("at the cap a transfer is refused")
	}
}

func TestAStageTheMoverRefusesIsAConfigurationProblemNotAnOutage(t *testing.T) {
	for _, err := range []error{stage.ErrTagNotFound, stage.ErrUnauthorized, stage.ErrStageNotConversationFunnel} {
		if !StageRefused(fmt.Errorf("move: %w", err)) {
			t.Errorf("%v must be a stage problem", err)
		}
	}
	if StageRefused(errors.New("connection reset")) {
		t.Error("an outage is not a stage problem")
	}
}

func TestAProblemSaysWhichDestinationItIsAbout(t *testing.T) {
	err := Check([]Destination{{ID: "d1", Target: agentTarget("a")}, {ID: "d2", Target: agentTarget("a")}}, "self")
	if position, ok := PositionOf(err); !ok || position != 2 || !errors.Is(err, ErrDuplicateDestination) {
		t.Fatalf("err = %v, position = %d", err, position)
	}
	if _, ok := PositionOf(ErrNoDestinations); ok {
		t.Fatal("a list-wide problem has no position")
	}
}
