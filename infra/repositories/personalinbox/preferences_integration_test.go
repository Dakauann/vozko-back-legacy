package personalinbox_repository

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"vozko/domain/personalinbox"
	"vozko/infra/database/schema"
	"vozko/infra/repositories/repotest"
)

var _ personalinbox.PreferencesRepository = (*Repository)(nil)

func TestInboxPreferencesAreStoredPerPersonAndWorkspaceAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	db := repotest.IsolatedDB(t, "personal_inbox_prefs_test", &schema.PersonalInboxPreference{})
	repo := New(db)
	workspace, other, user := uuid.NewString(), uuid.NewString(), uuid.NewString()

	fresh, err := repo.FindPreferences(ctx, workspace, user)
	if err != nil || fresh != personalinbox.DefaultPreferences() {
		t.Fatalf("unsaved = %+v, %v; want the default", fresh, err)
	}

	if err := repo.SavePreferences(ctx, workspace, user, personalinbox.Preferences{HideGroups: true}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := repo.SavePreferences(ctx, workspace, user, personalinbox.Preferences{HideGroups: true}); err != nil {
		t.Fatalf("saving twice must upsert: %v", err)
	}
	stored, err := repo.FindPreferences(ctx, workspace, user)
	if err != nil || !stored.HideGroups {
		t.Fatalf("stored = %+v, %v", stored, err)
	}
	elsewhere, err := repo.FindPreferences(ctx, other, user)
	if err != nil || elsewhere.HideGroups {
		t.Fatalf("another workspace = %+v, %v; want the default", elsewhere, err)
	}

	if err := repo.SavePreferences(ctx, workspace, user, personalinbox.Preferences{}); err != nil {
		t.Fatalf("unhide: %v", err)
	}
	back, err := repo.FindPreferences(ctx, workspace, user)
	if err != nil || back.HideGroups {
		t.Fatalf("after unhiding = %+v, %v", back, err)
	}
	var rows int64
	if err := db.Model(&schema.PersonalInboxPreference{}).Count(&rows).Error; err != nil || rows != 1 {
		t.Fatalf("rows = %d, %v; want one row per person and workspace", rows, err)
	}
}
