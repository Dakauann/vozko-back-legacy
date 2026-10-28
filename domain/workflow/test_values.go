package workflow

import "errors"

type TestValuesRepository interface {
	TestValues(workspaceID, workflowID, userID string) (map[string]string, error)
	SaveTestValues(workspaceID, workflowID, userID string, values map[string]string) error
	ClearTestValues(workspaceID, workflowID, userID string) error
}

var (
	ErrTestValueNotNeeded    = errors.New("workflow: this workflow's test does not use this variable")
	ErrTestValueInvalid      = errors.New("workflow: a test value has up to 2000 characters")
	ErrTestValueTypeInvalid  = errors.New("workflow: a test value does not fit the declared type")
	ErrTestValuesUnavailable = errors.New("workflow: the saved test values could not be read or written")
)
