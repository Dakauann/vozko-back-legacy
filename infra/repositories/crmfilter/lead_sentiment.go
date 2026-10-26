package crmfilter

import (
	"fmt"

	"github.com/lib/pq"

	"vozko/domain/crmfilter"
	"vozko/domain/shared"
	"vozko/infra/database/schema"
)

var leadSentimentsTable = schema.LeadSentiment{}.TableName()

func LeadSentimentJoin(alias, leadCol, workspaceCol string) string {
	return "LEFT JOIN " + leadSentimentsTable + " " + alias + " ON " + alias + ".lead_id = " + leadCol + " AND " + alias + ".workspace_id = " + workspaceCol
}

func SentimentCountsSQL(alias string) string {
	count := func(s shared.Sentiment) string {
		return "COUNT(*) FILTER (WHERE " + alias + ".sentiment = " + pq.QuoteLiteral(string(s)) + ") AS " + string(s)
	}
	return count(shared.SentimentPositive) + ", " + count(shared.SentimentNeutral) + ", " + count(shared.SentimentNegative)
}

func (d LeadDescriptor) compileSentiment(p crmfilter.Predicate) (string, []interface{}, error) {
	ws, err := d.workspace()
	if err != nil {
		return "", nil, err
	}
	values := trimmedValues(p.Values)
	if len(values) == 0 {
		return "", nil, crmfilter.ErrMissingValue
	}
	for _, v := range values {
		if !shared.Sentiment(v).Valid() {
			return "", nil, fmt.Errorf("%w: %q is not a sentiment", crmfilter.ErrInvalidValue, v)
		}
	}
	held := d.id() + " IN (SELECT ls_f.lead_id FROM " + leadSentimentsTable + " ls_f WHERE ls_f.workspace_id = ? AND ls_f.sentiment = ANY(?::text[]))"
	args := []interface{}{ws, pq.Array(values)}
	switch p.Operator {
	case crmfilter.OpEquals, crmfilter.OpIn:
		return held, args, nil
	case crmfilter.OpNotEquals, crmfilter.OpNotIn:
		return negate(held), args, nil
	}
	return "", nil, unsupported(p)
}
