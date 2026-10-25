package conversationroute

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"vozko/domain/shared"
	"vozko/infra/database/schema"
	"vozko/infra/testenv"
)

func routesDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testenv.Postgres(t).DSN()
	silent := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	admin, err := gorm.Open(postgres.Open(dsn), silent)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	schemaName := "routes_test_" + uuid.New().String()[:8]
	if err := admin.Exec("CREATE SCHEMA " + schemaName).Error; err != nil {
		t.Fatalf("create schema: %v", err)
	}
	db, err := gorm.Open(postgres.Open(dsn+" search_path="+schemaName), silent)
	if err != nil {
		t.Fatalf("open schema: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		_ = admin.Exec("DROP SCHEMA " + schemaName + " CASCADE").Error
		if sqlDB, err := admin.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&schema.ConversationDepartmentRoute{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestARouteIsReadBackOnlyForTheSameConversationAndChannel(t *testing.T) {
	routes := New(routesDB(t))
	entryID, sales := uuid.NewString(), uuid.NewString()

	if err := routes.RouteToDepartment(entryID, string(shared.EntryTypeUnofficialWhatsApp), sales); err != nil {
		t.Fatal(err)
	}

	got, found, err := routes.RoutedDepartment(entryID, string(shared.EntryTypeUnofficialWhatsApp))
	if err != nil || !found || got != sales {
		t.Fatalf("route = %q %v %v, want %s", got, found, err, sales)
	}
	if got, found, err := routes.RoutedDepartment(entryID, string(shared.EntryTypeTelegram)); err != nil || found || got != "" {
		t.Fatalf("the same id on another channel is another conversation: %q %v %v", got, found, err)
	}
	if got, found, err := routes.RoutedDepartment(uuid.NewString(), string(shared.EntryTypeUnofficialWhatsApp)); err != nil || found || got != "" {
		t.Fatalf("another conversation must not see the route: %q %v %v", got, found, err)
	}
}

func TestTheLatestTransferWins(t *testing.T) {
	routes := New(routesDB(t))
	entryID, sales, support := uuid.NewString(), uuid.NewString(), uuid.NewString()

	if err := routes.RouteToDepartment(entryID, string(shared.EntryTypeWhatsApp), sales); err != nil {
		t.Fatal(err)
	}
	if err := routes.RouteToDepartment(entryID, string(shared.EntryTypeWhatsApp), support); err != nil {
		t.Fatal(err)
	}

	got, found, err := routes.RoutedDepartment(entryID, string(shared.EntryTypeWhatsApp))
	if err != nil || !found || got != support {
		t.Fatalf("route = %q %v %v, want the latest transfer %s", got, found, err, support)
	}
}

func TestAConversationNeverTransferredHasNoRoute(t *testing.T) {
	routes := New(routesDB(t))

	got, found, err := routes.RoutedDepartment(uuid.NewString(), string(shared.EntryTypeWhatsApp))
	if err != nil || found || got != "" {
		t.Fatalf("route = %q %v %v, want none", got, found, err)
	}
}
