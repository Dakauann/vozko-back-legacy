package workspace_department_repository

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"vozko/infra/testenv"
)

func departmentsDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testenv.Postgres(t).DSN()
	silent := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	admin, err := gorm.Open(postgres.Open(dsn), silent)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	schemaName := "departments_test_" + uuid.New().String()[:8]
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
	for _, ddl := range []string{
		`CREATE TABLE workspace_departments (
			id uuid PRIMARY KEY,
			workspace_id uuid NOT NULL,
			name varchar(255) NOT NULL,
			description varchar(500),
			working_hours jsonb,
			created_at timestamptz,
			updated_at timestamptz,
			deleted_at timestamptz
		)`,
		`CREATE TABLE workspace_department_members (
			id uuid PRIMARY KEY,
			department_id uuid NOT NULL,
			member_id uuid NOT NULL,
			created_at timestamptz
		)`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	return db
}

func seedDepartment(t *testing.T, db *gorm.DB, workspaceID string, deletedAt *time.Time, members int) {
	t.Helper()
	departmentID := uuid.NewString()
	if err := db.Exec(
		"INSERT INTO workspace_departments (id, workspace_id, name, created_at, updated_at, deleted_at) VALUES (?, ?, ?, now(), now(), ?)",
		departmentID, workspaceID, "Vendas", deletedAt,
	).Error; err != nil {
		t.Fatalf("seed department: %v", err)
	}
	for i := 0; i < members; i++ {
		if err := db.Exec(
			"INSERT INTO workspace_department_members (id, department_id, member_id, created_at) VALUES (?, ?, ?, now())",
			uuid.NewString(), departmentID, uuid.NewString(),
		).Error; err != nil {
			t.Fatalf("seed member: %v", err)
		}
	}
}

func TestHasDepartmentsAgreesWithListingDepartmentsOnRealPostgres(t *testing.T) {
	db := departmentsDB(t)
	repo := &repositoryImpl{db: db}
	deleted := time.Now()

	empty := uuid.NewString()
	live := uuid.NewString()
	seedDepartment(t, db, live, nil, 0)
	staffed := uuid.NewString()
	seedDepartment(t, db, staffed, nil, 3)
	seedDepartment(t, db, staffed, &deleted, 2)
	onlyDeleted := uuid.NewString()
	seedDepartment(t, db, onlyDeleted, &deleted, 1)

	for name, workspaceID := range map[string]string{
		"no departments":                  empty,
		"one empty department":            live,
		"live and deleted departments":    staffed,
		"only a deleted department":       onlyDeleted,
		"a workspace that does not exist": uuid.NewString(),
	} {
		t.Run(name, func(t *testing.T) {
			listed, err := repo.ListDepartments(workspaceID)
			if err != nil {
				t.Fatal(err)
			}
			has, err := repo.HasDepartments(workspaceID)
			if err != nil {
				t.Fatal(err)
			}
			if has != (len(listed) > 0) {
				t.Fatalf("HasDepartments = %v but ListDepartments returned %d rows", has, len(listed))
			}
		})
	}
}
