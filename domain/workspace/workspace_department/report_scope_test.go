package workspace_department

import (
	"reflect"
	"testing"
)

func TestReportScopeNarrowsAReportToWhatTheCallerMayRead(t *testing.T) {
	cases := []struct {
		name        string
		filter      *DepartmentFilter
		requested   string
		departments []string
		blocked     bool
	}{
		{"no filter reads nothing", nil, "", nil, true},
		{"an owner without a choice reads every department", owner, "", nil, false},
		{"an owner choosing a department reads only it", owner, "d9", []string{"d9"}, false},
		{"a member of a flat workspace reads everything", flatMember, "", nil, false},
		{"a member without a choice reads their own departments", twoDeptMember, "", []string{"d1", "d2"}, false},
		{"a member choosing their department reads only it", twoDeptMember, "d2", []string{"d2"}, false},
		{"a member choosing another department reads nothing", oneDeptMember, "d2", nil, true},
		{"a member in no department reads nothing", strandedMember, "", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			departments, blocked := tc.filter.ReportScope(tc.requested)
			if blocked != tc.blocked || !reflect.DeepEqual(departments, tc.departments) {
				t.Fatalf("ReportScope(%q) = %v, %v; want %v, %v", tc.requested, departments, blocked, tc.departments, tc.blocked)
			}
		})
	}
}
