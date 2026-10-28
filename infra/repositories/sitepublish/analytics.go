package sitepublish_repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"vozko/domain/sitepublish"
	"vozko/infra/database/schema"
)

const upsertDailySQL = `INSERT INTO site_analytics_daily (project_id, day, route, variant, views, visitors, cta, form_views, form_starts, form_submits, whatsapp) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (project_id, day, route, variant) DO UPDATE SET views = EXCLUDED.views, visitors = EXCLUDED.visitors, cta = EXCLUDED.cta, form_views = EXCLUDED.form_views, form_starts = EXCLUDED.form_starts, form_submits = EXCLUDED.form_submits, whatsapp = EXCLUDED.whatsapp`

type Analytics struct {
	db *gorm.DB
}

func NewAnalytics(db *gorm.DB) *Analytics {
	return &Analytics{db: db}
}

func (r *Analytics) Upsert(ctx context.Context, stats []sitepublish.DailyStat) error {
	if len(stats) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, s := range stats {
			if err := tx.Exec(upsertDailySQL, s.ProjectID, s.Day, s.Route, s.Variant, s.Views, s.Visitors, s.CTA, s.FormViews, s.FormStarts, s.FormSubmits, s.WhatsApp).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Analytics) List(ctx context.Context, projectID string, from, to time.Time) ([]sitepublish.DailyStat, error) {
	var rows []schema.SiteAnalyticsDaily
	if err := r.db.WithContext(ctx).Where("project_id = ? AND day >= ? AND day <= ?", projectID, from, to).
		Order("day, route, variant").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]sitepublish.DailyStat, len(rows))
	for i, row := range rows {
		out[i] = sitepublish.DailyStat{
			ProjectID: row.ProjectID, Day: row.Day, Route: row.Route, Variant: row.Variant, Views: row.Views, Visitors: row.Visitors,
			CTA: row.CTA, FormViews: row.FormViews, FormStarts: row.FormStarts, FormSubmits: row.FormSubmits, WhatsApp: row.WhatsApp,
		}
	}
	return out, nil
}

const upsertSourcesSQL = `INSERT INTO site_analytics_sources (project_id, day, kind, value, views, submits) VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT (project_id, day, kind, value) DO UPDATE SET views = EXCLUDED.views, submits = EXCLUDED.submits`

func (r *Analytics) UpsertSources(ctx context.Context, stats []sitepublish.SourceStat) error {
	if len(stats) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, s := range stats {
			if err := tx.Exec(upsertSourcesSQL, s.ProjectID, s.Day, string(s.Kind), s.Value, s.Views, s.Submits).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Analytics) ListSources(ctx context.Context, projectID string, from, to time.Time) ([]sitepublish.SourceStat, error) {
	var rows []schema.SiteAnalyticsSource
	if err := r.db.WithContext(ctx).Where("project_id = ? AND day >= ? AND day <= ?", projectID, from, to).
		Order("day, kind, value").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]sitepublish.SourceStat, len(rows))
	for i, row := range rows {
		out[i] = sitepublish.SourceStat{ProjectID: row.ProjectID, Day: row.Day, Kind: sitepublish.SourceKind(row.Kind), Value: row.Value, Views: row.Views, Submits: row.Submits}
	}
	return out, nil
}
