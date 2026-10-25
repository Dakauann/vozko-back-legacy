package tools_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/agenttool"
	"vozko/domain/tools"
	wd "vozko/domain/workspace/workspace_department"
)

type departmentBook struct {
	byID map[string]*wd.Department
	err  error
}

func (d departmentBook) GetDepartmentByID(id string) (*wd.Department, error) {
	if d.err != nil {
		return nil, d.err
	}
	dept, ok := d.byID[id]
	if !ok {
		return nil, wd.ErrDepartmentNotFound
	}
	return dept, nil
}

func transferTool(book departmentBook) tools.BindingValidator {
	return NewTransferToHumanToolUseCase(&handOffStub{}, book).(tools.BindingValidator)
}

func TestTransferIsOneInstancePerDepartment(t *testing.T) {
	def := NewTransferToHumanToolUseCase(&handOffStub{}, departmentBook{}).Definition()
	if def.Instances != tools.PerTarget("department_id") {
		t.Fatalf("policy %+v", def.Instances)
	}
}

func TestATransferTargetMustBeADepartmentOfTheWorkspace(t *testing.T) {
	book := departmentBook{byID: map[string]*wd.Department{"d-1": {ID: "d-1", WorkspaceID: "ws-1"}, "d-x": {ID: "d-x", WorkspaceID: "ws-2"}}}
	v := transferTool(book)
	if err := v.ValidateBinding(context.Background(), "ws-1", map[string]interface{}{"department_id": "d-1"}); err != nil {
		t.Fatalf("own department: %v", err)
	}
	if err := v.ValidateBinding(context.Background(), "ws-1", map[string]interface{}{}); err != nil {
		t.Fatalf("any department: %v", err)
	}
	for _, id := range []string{"d-x", "ghost"} {
		problem, ok := tools.ProblemOf(v.ValidateBinding(context.Background(), "ws-1", map[string]interface{}{"department_id": id}))
		if !ok || problem.Code != agenttool.CodeDepartmentInvalid {
			t.Fatalf("%s: %+v", id, problem)
		}
	}
	down := transferTool(departmentBook{err: errors.New("db down")})
	if err := down.ValidateBinding(context.Background(), "ws-1", map[string]interface{}{"department_id": "d-1"}); err == nil {
		t.Fatal("an outage must fail")
	} else if _, ok := tools.ProblemOf(err); ok {
		t.Fatal("an outage is not a binding problem")
	}
}
