package attendance_repository

import (
	"strings"
	"testing"
	"time"

	"vozko/domain/attendance"
	"vozko/domain/conversation"
	"vozko/infra/database/sqlexpr"
)

func originArgs(args []interface{}, origin conversation.Origin) int {
	count := 0
	for _, arg := range args {
		if value, ok := arg.(string); ok && value == string(origin) {
			count++
		}
	}
	return count
}

func receptiveWindow() attendance.OverviewFilter {
	filter := activityWindow()
	filter.Origin = conversation.OriginReceptive
	return filter
}

func TestTheScopeKeepsOnlyConversationsOfTheChosenOrigin(t *testing.T) {
	for name, filter := range map[string]attendance.OverviewFilter{
		"all time":    {Origin: conversation.OriginReceptive},
		"date window": receptiveWindow(),
	} {
		t.Run(name, func(t *testing.T) {
			sql, args := overviewEntrySelect("ws-1", filter)
			branches := len(channelSources)
			if filter.DateFrom != nil {
				branches *= 2
			}
			for _, src := range channelSources {
				predicate := sqlexpr.ConversationOriginIs(src.EntryAlias+".id", "'"+string(src.EntryType)+"'")
				if !strings.Contains(sql, predicate) {
					t.Fatalf("%s branch does not narrow by origin:\n%s", src.EntryType, sql)
				}
			}
			if got := originArgs(args, conversation.OriginReceptive); got != branches {
				t.Fatalf("origin bound %d times, want once per branch (%d)", got, branches)
			}
			if got := strings.Count(sql, "?"); got != len(args) {
				t.Fatalf("placeholders = %d, args = %d", got, len(args))
			}
		})
	}
}

func TestWithoutAnOriginTheScopeNeverReadsTheOriginTable(t *testing.T) {
	sql, _ := overviewEntrySelect("ws-1", activityWindow())
	if strings.Contains(sql, sqlexpr.ConversationOriginTable) {
		t.Fatalf("an unfiltered scope must not pay for the origin lookup:\n%s", sql)
	}
}

func TestTheTrendFollowsTheOrigin(t *testing.T) {
	from, to, loc := trendWindowForTest()
	filter := attendance.OverviewFilter{Origin: conversation.OriginCampaign, DepartmentID: "dept-1"}
	sources := selectedChannelSources("")

	sql, args := trendQuery("ws-1", filter, sources, from, to, loc).build()
	assertArgsMatchTheirPlaceholders(t, "trend query", sql, args)
	if got := originArgs(args, conversation.OriginCampaign); got != strings.Count(sql, sqlexpr.ConversationOriginTable) || got == 0 {
		t.Fatalf("trend binds the origin %d times for %d lookups", got, strings.Count(sql, sqlexpr.ConversationOriginTable))
	}

	unbucketed, unbucketedArgs := trendUnbucketedQuery("ws-1", filter, sources, from, to).build()
	assertArgsMatchTheirPlaceholders(t, "trend unbucketed", unbucketed, unbucketedArgs)
	if originArgs(unbucketedArgs, conversation.OriginCampaign) == 0 {
		t.Fatalf("the unbucketed count ignores the origin:\n%s", unbucketed)
	}
}

func TestFirstResponseFollowsTheOrigin(t *testing.T) {
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	sql, args := frtSamplesQuery("ws-1", attendance.StatsFilter{DateFrom: &from, Origin: conversation.OriginReceptive}).build()
	if !strings.Contains(sql, sqlexpr.ConversationOriginIs("ah.entry_id", "ah.entry_type")) {
		t.Fatalf("FRT samples do not narrow by origin:\n%s", sql)
	}
	if got := strings.Count(sql, "?"); got != len(args) || originArgs(args, conversation.OriginReceptive) != 1 {
		t.Fatalf("placeholders = %d, args = %v", got, args)
	}

	plain, _ := frtSamplesQuery("ws-1", attendance.StatsFilter{DateFrom: &from}).build()
	if strings.Contains(plain, sqlexpr.ConversationOriginTable) {
		t.Fatalf("FRT without an origin must not read the origin table:\n%s", plain)
	}
}

func TestAISessionsAndReopensFollowTheOrigin(t *testing.T) {
	filter := attendance.OverviewFilter{Origin: conversation.OriginOutreach}

	ai, aiArgs := aiSessionScope("s.", filter).build()
	if !strings.Contains(ai, sqlexpr.ConversationOriginIs(sqlexpr.CanonicalUUID("s.entry_id"), "s.entry_type")) {
		t.Fatalf("AI sessions do not narrow by origin through a safe uuid cast:\n%s", ai)
	}
	if originArgs(aiArgs, conversation.OriginOutreach) != 1 || strings.Count(ai, "?") != len(aiArgs) {
		t.Fatalf("AI session scope args = %v for %s", aiArgs, ai)
	}

	reopen, reopenArgs := reopenCountsQuery("ws-1", filter).build()
	if !strings.Contains(reopen, sqlexpr.ConversationOriginIs("ce.entry_id", "ce.entry_type")) {
		t.Fatalf("reopens do not narrow by origin:\n%s", reopen)
	}
	if originArgs(reopenArgs, conversation.OriginOutreach) != 1 || strings.Count(reopen, "?") != len(reopenArgs) {
		t.Fatalf("reopen args = %v for %s", reopenArgs, reopen)
	}
}

func TestOverviewFRTCarriesTheOriginIntoItsSamples(t *testing.T) {
	filter := attendance.OverviewFilter{Origin: conversation.OriginCampaign, CampaignID: "camp-1"}
	if got := frtFilterOf(filter); got.Origin != conversation.OriginCampaign || got.CampaignID != "camp-1" {
		t.Fatalf("frt filter = %+v, want the overview origin and campaign", got)
	}
}
