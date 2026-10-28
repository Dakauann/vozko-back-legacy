package sitepublish_repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"vozko/infra/database/schema"
)

const upsertServingKeySQL = `INSERT INTO site_serving_keys (key, value, updated_at) VALUES (?, ?, ?) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at`

type ServingKeys struct {
	db  *gorm.DB
	now func() time.Time
}

func NewServingKeys(db *gorm.DB, now func() time.Time) *ServingKeys {
	return &ServingKeys{db: db, now: now}
}

func (r *ServingKeys) Put(ctx context.Context, key string, value []byte) error {
	return r.db.WithContext(ctx).Exec(upsertServingKeySQL, key, string(value), r.now()).Error
}

func (r *ServingKeys) Delete(ctx context.Context, key string) error {
	return r.db.WithContext(ctx).Where("key = ?", key).Delete(&schema.SiteServingKey{}).Error
}

func (r *ServingKeys) Get(ctx context.Context, key string) ([]byte, bool, error) {
	var row schema.SiteServingKey
	err := r.db.WithContext(ctx).Where("key = ?", key).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return []byte(row.Value), true, nil
}
