package whatsapp_campaign_entry

import (
	"strings"
	"testing"
	"time"

	"vozko/domain/campaign"
	wce "vozko/domain/whatsapp_campaign_entry"
)

func TestSendsQueriesBindOnlyNamedArguments(t *testing.T) {
	from, to := time.Now().Add(-time.Hour), time.Now()
	f := wce.WorkspaceSummaryFilter{WorkspaceID: "ws", DepartmentIDs: []string{"d"}, Window: campaign.SendWindow{From: &from, To: &to}}
	campaignSQL, _ := campaignSendsQuery(f)
	oneOffSQL, _ := oneOffSendsQuery(f)
	for name, sql := range map[string]string{"campaign": campaignSQL, "one-off": oneOffSQL} {
		if strings.Contains(sql, "?") {
			t.Errorf("%s query carries a question mark, which GORM would bind as a positional argument", name)
		}
		if strings.Contains(sql, "{") {
			t.Errorf("%s query kept a placeholder: %s", name, sql)
		}
	}
}

func TestOneOffStatusSQLFollowsTheDomainRule(t *testing.T) {
	want := "CASE WHEN s.status IN ('rejected', 'refunded') THEN 'FAILED'" +
		" WHEN r = 'delivered' THEN 'DELIVERED' WHEN r = 'failed' THEN 'FAILED' WHEN r = 'read' THEN 'READ' ELSE 'SENT' END"
	if got := oneOffStatus("s", "r"); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}
