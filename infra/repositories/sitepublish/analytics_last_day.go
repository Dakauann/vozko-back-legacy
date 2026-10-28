package sitepublish_repository

import (
	"context"
	"database/sql"
	"time"
)

const lastDaySQL = `SELECT MAX(day) FROM site_analytics_daily`

func (r *Analytics) LastDay(ctx context.Context) (time.Time, bool, error) {
	var day sql.NullTime
	if err := r.db.WithContext(ctx).Raw(lastDaySQL).Row().Scan(&day); err != nil {
		return time.Time{}, false, err
	}
	return day.Time, day.Valid, nil
}
