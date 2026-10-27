package campaignstamp

import (
	"strconv"
	"strings"

	"vozko/domain/campaign"
	"vozko/infra/database/sqlexpr"
)

func OutcomeMoment(alias string) string {
	milestones := campaign.OutcomeMilestones()
	stamps := make([]string, len(milestones))
	for i, m := range milestones {
		stamps[i] = alias + "." + column(m)
	}
	codes := campaign.SkipFailureCodes()
	skipCodes := make([]string, len(codes))
	for i, code := range codes {
		skipCodes[i] = strconv.Itoa(code)
	}
	return "COALESCE(LEAST(" + strings.Join(stamps, ", ") + "), CASE WHEN " +
		alias + ".status IN (" + sqlexpr.LiteralList(campaign.Strings(campaign.DecidedWithoutSending())) + ")" +
		" OR (" + alias + ".status = " + sqlexpr.Literal(string(campaign.SendStatusFailed)) +
		" AND " + alias + ".error_code IN (" + strings.Join(skipCodes, ", ") + ")) THEN " + alias + ".created_at END)"
}
