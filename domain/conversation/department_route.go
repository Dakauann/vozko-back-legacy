package conversation

import "errors"

var ErrDepartmentRoutesUnavailable = errors.New("conversation: the department a conversation was sent to cannot be tracked")

type DepartmentRoutes interface {
	RouteToDepartment(entryID, entryType, departmentID string) error
	RoutedDepartment(entryID, entryType string) (string, bool, error)
}
