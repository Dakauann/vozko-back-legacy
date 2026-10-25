package departmentscope

import (
	"fmt"

	"github.com/lib/pq"

	"vozko/infra/database/schema"
)

func Routed(entryIDColumn, entryType, departmentColumn string) string {
	return fmt.Sprintf(
		"COALESCE((SELECT cdr.department_id FROM %s cdr WHERE cdr.entry_id = %s AND cdr.entry_type = '%s'), %s)",
		schema.ConversationDepartmentRoutesTable, entryIDColumn, entryType, departmentColumn,
	)
}

func Clause(deptColumn, entryIDColumn string, departmentIDs []string, restrict bool, assigneeUserID string) (string, []interface{}) {
	if !restrict {
		return "", nil
	}
	if assigneeUserID != "" && entryIDColumn != "" {
		if len(departmentIDs) == 0 {
			clause := fmt.Sprintf(
				" AND EXISTS (SELECT 1 FROM inbox_assignments ia_d WHERE ia_d.entry_id = %s AND ia_d.assigned_user_id = ?)",
				entryIDColumn,
			)
			return clause, []interface{}{assigneeUserID}
		}
		clause := fmt.Sprintf(
			" AND (%s = ANY(?::uuid[]) OR EXISTS (SELECT 1 FROM inbox_assignments ia_d WHERE ia_d.entry_id = %s AND ia_d.assigned_user_id = ?))",
			deptColumn, entryIDColumn,
		)
		return clause, []interface{}{pq.Array(departmentIDs), assigneeUserID}
	}
	if len(departmentIDs) == 0 {
		return " AND 1 = 0", nil
	}
	return fmt.Sprintf(" AND %s = ANY(?::uuid[])", deptColumn), []interface{}{pq.Array(departmentIDs)}
}
