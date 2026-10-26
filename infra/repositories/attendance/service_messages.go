package attendance_repository

import (
	"context"

	"gorm.io/gorm"

	"vozko/domain/attendance"
	"vozko/domain/shared"
	"vozko/infra/database/sqlexpr"
)

func NewServiceMessageReader(db *gorm.DB) attendance.ServiceMessageReader {
	return &repository{db: db}
}

var chargedServiceMessage = sqlexpr.ServiceMessage("cm") + `
			  AND ` + sqlexpr.MetaChargedService("cm")

func officialWhatsAppSource() channelSource {
	return selectedChannelSources(string(shared.EntryTypeWhatsApp))[0]
}

func (r *repository) ReadServiceMessages(
	ctx context.Context,
	workspaceID string,
	q attendance.ServiceMessageQuery,
) ([]attendance.ServiceMessageCount, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	rows := []attendance.ServiceMessageCount{}
	if blankWorkspace(workspaceID) || !q.Applies() {
		return rows, nil
	}
	sql, args := serviceMessageCountQuery(workspaceID, q).build()
	err := r.analyticsContext(ctx, func(tx *gorm.DB) error {
		return tx.Raw(sql, args...).Scan(&rows).Error
	})
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func serviceMessageCountQuery(workspaceID string, q attendance.ServiceMessageQuery) *sqlQuery {
	src := officialWhatsAppSource()
	from, to := *q.Filter.DateFrom, *q.Filter.DateTo
	query := newSQLQuery().add(`
		SELECT date_trunc(`+sqlexpr.Literal(string(q.Granularity()))+`, cm.created_at AT TIME ZONE 'UTC') AS bucket,
			COUNT(*)::bigint AS messages
		`+trendJoins(src)+`
		JOIN conversation_messages cm
			ON cm.entry_id = `+src.EntryAlias+`.id AND cm.entry_type = '`+string(src.EntryType)+`'
		WHERE `+src.WorkspaceColumn+` = ?
		  AND `+src.EntryAlias+`.deleted_at IS NULL
		  AND `+src.LastMessageColumn+` >= ?
		  AND cm.created_at >= ? AND cm.created_at <= ?
		  AND `+chargedServiceMessage, workspaceID, from, from, to)
	query.addQuery(scopeClause(src, q.Filter))
	if len(q.Departments) > 0 {
		query.add(" AND "+src.DepartmentColumn+"::text IN ?", q.Departments)
	}
	return query.add(`
		GROUP BY 1
		ORDER BY 1`)
}
