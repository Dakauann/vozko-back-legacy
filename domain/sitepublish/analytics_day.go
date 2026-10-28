package sitepublish

import (
	"time"

	"vozko/domain/config"
)

const AnalyticsTimeZone = "America/Sao_Paulo"

func AnalyticsDay(at time.Time) time.Time {
	y, m, d := at.In(config.BrazilianTimezone).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func AnalyticsDayStart(day time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, config.BrazilianTimezone)
}
