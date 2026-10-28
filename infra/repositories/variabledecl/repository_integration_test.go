package variabledecl_repository

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	domain "vozko/domain/variabledecl"
	"vozko/infra/database/schema"
	"vozko/infra/testenv"
)

var errRollback = errors.New("rollback")

func inRolledBackTx(t *testing.T, run func(tx *gorm.DB)) {
	t.Helper()
	db, err := gorm.Open(postgres.Open(testenv.Postgres(t).DSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		schemaName := "variable_decl_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		for _, ddl := range []string{"CREATE SCHEMA " + schemaName, "SET LOCAL search_path TO " + schemaName} {
			if err := tx.Exec(ddl).Error; err != nil {
				t.Fatalf("fixture: %v", err)
			}
		}
		if err := tx.AutoMigrate(&schema.ConversationVariableDeclaration{}); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		run(tx)
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("transaction: %v", err)
	}
}

func TestDeclarationsAreUpsertedListedAndDeletedPerWorkspace(t *testing.T) {
	inRolledBackTx(t, func(tx *gorm.DB) {
		repo := New(tx)
		ctx := context.Background()
		ws, other := uuid.NewString(), uuid.NewString()
		at := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
		turno := domain.Declaration{Name: "turno", Label: "Turno", Description: "Período", Type: domain.Type{Kind: domain.KindChoice, Options: []string{"Manhã", "Tarde"}}, UpdatedBy: "u1", UpdatedAt: at}
		for _, d := range []domain.Declaration{turno, {Name: "valor", Label: "Valor", Type: domain.Type{Kind: domain.KindNumber}, UpdatedAt: at}} {
			if err := repo.Save(ctx, ws, d); err != nil {
				t.Fatal(err)
			}
		}
		turno.Label = "Turno da aula"
		if err := repo.Save(ctx, ws, turno); err != nil {
			t.Fatal(err)
		}
		got, err := repo.List(ctx, ws)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].Label != "Turno da aula" || !reflect.DeepEqual(got[0].Type.Options, []string{"Manhã", "Tarde"}) || got[1].Type.Kind != domain.KindNumber {
			t.Fatalf("list = %+v", got)
		}
		if none, _ := repo.List(ctx, other); len(none) != 0 {
			t.Fatalf("other workspace = %+v", none)
		}
		removed, err := repo.Delete(ctx, ws, "valor")
		if err != nil || !removed {
			t.Fatalf("delete = %v %v", removed, err)
		}
		if removed, _ := repo.Delete(ctx, ws, "valor"); removed {
			t.Fatal("deleted twice")
		}
	})
}
