package conversation_repository

import (
	"strings"
	"testing"

	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/infra/repositories/departmentscope"
)

func TestEveryChannelsRestrictedSelectReadsTheRoutedDepartment(t *testing.T) {
	scope := entrySourceScope{DepartmentIDs: []string{"d1"}, RestrictDepartments: true}
	for _, src := range entrySources {
		if src.Department == "" {
			continue
		}
		sql, args := src.inboxSelect(scope, "ws-1")
		routed := departmentscope.Routed(src.EntryID, string(src.EntryType), src.Department)
		if !strings.Contains(sql, routed+" = ANY(?::uuid[])") {
			t.Errorf("%s does not scope by the routed department:\n%s", src.EntryType, sql)
		}
		assertPlaceholdersMatchArgs(t, sql, args)
	}
}

func TestEveryChannelsVisibleEntryBranchReadsTheRoutedDepartment(t *testing.T) {
	visibility := conversation.VisibilityScope{ReadScope: conversation.ReadScope{DepartmentIDs: []string{"d1"}, RestrictDepartments: true}}
	clause, _ := VisibleEntrySQL("r.entry_id", "r.entry_type", "ws-1", visibility)
	for _, src := range entrySources {
		if src.Department == "" {
			continue
		}
		if !strings.Contains(clause, departmentscope.Routed(src.EntryID, string(src.EntryType), src.Department)) {
			t.Errorf("%s branch does not read the routed department:\n%s", src.EntryType, clause)
		}
	}
}

func TestEveryEntrySourceIsACodeConstantChannel(t *testing.T) {
	for _, src := range entrySources {
		if !src.EntryType.IsKnown() {
			t.Errorf("entry source %q is not a known channel; only code constants may reach the routed SQL", src.EntryType)
		}
	}
	for _, q := range channelQueries {
		if !q.EntryType.IsKnown() {
			t.Errorf("channel query %q is not a known channel; only code constants may reach the routed SQL", q.EntryType)
		}
	}
}

func TestUnofficialDepartmentIsTheCampaignThenTheNumber(t *testing.T) {
	got := UnofficialDepartment("uwc", "uwi")
	want := "COALESCE((SELECT camp_dept.department_id FROM unofficial_whatsapp_campaigns camp_dept" +
		" WHERE camp_dept.id = NULLIF(uwc.campaign_id, '')::uuid AND camp_dept.deleted_at IS NULL), uwi.department_id)"
	if got != want {
		t.Fatalf("UnofficialDepartment = %q, want %q", got, want)
	}
	if strings.Contains(got, "?") {
		t.Fatalf("the department expression must bind nothing: %q", got)
	}
}

func TestTheUnofficialInboxScopesByTheResolversDepartment(t *testing.T) {
	src := sourceFor(t, shared.EntryTypeUnofficialWhatsApp)
	if src.Department != UnofficialDepartment("uwc", "uwi") {
		t.Errorf("the inbox list scopes by %q, want the campaign then the number", src.Department)
	}
	ch, ok := channelQueryFor(shared.EntryTypeUnofficialWhatsApp)
	if !ok {
		t.Fatal("no unofficial channel query")
	}
	if ch.DepartmentColumn != UnofficialDepartment("uwc_f", "uwi_f") {
		t.Errorf("the number container scopes by %q, want the campaign then the number", ch.DepartmentColumn)
	}
}

func TestRoutedDepartmentOfAMissingColumnStaysMissing(t *testing.T) {
	ch, _ := channelQueryFor(shared.EntryTypeWhatsApp)
	if got := ch.routedDepartment("wce_f.id", ""); got != "" {
		t.Fatalf("a channel without a department column must not gain one: %q", got)
	}
}
