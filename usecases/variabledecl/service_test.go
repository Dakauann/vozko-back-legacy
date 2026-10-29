package variabledecl

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	domain "vozko/domain/variabledecl"
)

type fakeRepo struct {
	rows    map[string]map[string]domain.Declaration
	failing bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{rows: map[string]map[string]domain.Declaration{}}
}

func (f *fakeRepo) List(_ context.Context, workspaceID string) ([]domain.Declaration, error) {
	if f.failing {
		return nil, errors.New("db down")
	}
	var out []domain.Declaration
	for _, d := range f.rows[workspaceID] {
		out = append(out, d)
	}
	return out, nil
}

func (f *fakeRepo) Save(_ context.Context, workspaceID string, d domain.Declaration) error {
	if f.failing {
		return errors.New("db down")
	}
	if f.rows[workspaceID] == nil {
		f.rows[workspaceID] = map[string]domain.Declaration{}
	}
	f.rows[workspaceID][d.Name] = d
	return nil
}

func (f *fakeRepo) Delete(_ context.Context, workspaceID, name string) (bool, error) {
	if f.failing {
		return false, errors.New("db down")
	}
	_, ok := f.rows[workspaceID][name]
	delete(f.rows[workspaceID], name)
	return ok, nil
}

func newService(t *testing.T, repo *fakeRepo) *Service {
	t.Helper()
	registry, err := domain.NewRegistry(domain.Builtin()...)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewService(ServiceDeps{Repo: repo, Registry: registry, Automations: &fakeAutomations{}, Permissions: grants{}})
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC) }
	return s
}

func TestSaveChecksStampsAndStoresPerWorkspace(t *testing.T) {
	repo := newFakeRepo()
	s := newService(t, repo)
	saved, err := s.Save(context.Background(), "ws", editor("user-1"), domain.Declaration{Name: " valor ", Label: "Valor", Type: domain.Type{Kind: domain.KindNumber}})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Name != "valor" || saved.UpdatedBy != "user-1" || saved.UpdatedAt.IsZero() {
		t.Fatalf("saved = %+v", saved)
	}
	if _, ok := repo.rows["ws"]["valor"]; !ok {
		t.Fatal("not stored")
	}
	if _, err := s.Save(context.Background(), "ws", editor("user-1"), domain.Declaration{Name: "lead_name", Label: "X", Type: domain.Type{Kind: domain.KindText}}); !errors.Is(err, domain.ErrNameReserved) {
		t.Fatalf("err = %v", err)
	}
}

func TestListAndCatalogFailClosed(t *testing.T) {
	repo := newFakeRepo()
	s := newService(t, repo)
	repo.failing = true
	if _, err := s.List(context.Background(), "ws"); !errors.Is(err, domain.ErrUnreadable) {
		t.Fatalf("err = %v", err)
	}
	if _, err := s.Catalog(context.Background(), "ws"); !errors.Is(err, domain.ErrUnreadable) {
		t.Fatalf("err = %v", err)
	}
	if _, err := s.Rejected(context.Background(), "ws", map[string]string{"a": "b"}); !errors.Is(err, domain.ErrUnreadable) {
		t.Fatalf("err = %v", err)
	}
}

func TestDeleteSaysWhenNothingWasThere(t *testing.T) {
	repo := newFakeRepo()
	s := newService(t, repo)
	if err := s.Delete(context.Background(), "ws", editor("u"), "nada"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	_, _ = s.Save(context.Background(), "ws", editor("u"), domain.Declaration{Name: "x", Label: "X", Type: domain.Type{Kind: domain.KindText}})
	if err := s.Delete(context.Background(), "ws", editor("u"), "x"); err != nil {
		t.Fatal(err)
	}
}

func TestRejectedReadsTheWorkspaceDeclarations(t *testing.T) {
	repo := newFakeRepo()
	s := newService(t, repo)
	_, _ = s.Save(context.Background(), "ws", editor("u"), domain.Declaration{Name: "valor", Label: "Valor", Type: domain.Type{Kind: domain.KindNumber}})
	got, err := s.Rejected(context.Background(), "ws", map[string]string{"valor": "dez", "livre": "ok"})
	if err != nil || !reflect.DeepEqual(got, []string{"valor"}) {
		t.Fatalf("rejected = %v, err = %v", got, err)
	}
	got, err = s.Rejected(context.Background(), "other", map[string]string{"valor": "dez"})
	if err != nil || len(got) != 0 {
		t.Fatalf("other workspace = %v, err = %v", got, err)
	}
}

type titledSheets struct{}

func (titledSheets) Kind() domain.Kind                           { return domain.KindSheet }
func (titledSheets) Shape() domain.Shape                         { return domain.ShapeSheet }
func (titledSheets) Sample(t domain.Type) string                 { return t.Settings[domain.SettingModelTitle] }
func (titledSheets) CheckType(t domain.Type) error               { return nil }
func (titledSheets) Accepts(context.Context, domain.Check) error { return nil }

func (titledSheets) Prepare(_ context.Context, workspaceID string, t domain.Type) (domain.Type, error) {
	if workspaceID != "ws" {
		return domain.Type{}, domain.ErrTypeInvalid
	}
	return domain.Type{Kind: t.Kind, Settings: map[string]string{domain.SettingModel: t.Settings[domain.SettingModel], domain.SettingModelTitle: "Modelo Altiva"}}, nil
}

func TestSaveLetsTheTypeReadTheWorkspaceFirst(t *testing.T) {
	registry, err := domain.NewRegistry(append(domain.Builtin(), titledSheets{})...)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := NewService(ServiceDeps{Repo: newFakeRepo(), Registry: registry, Automations: &fakeAutomations{}, Permissions: grants{}})
	in := domain.Declaration{Name: "planilha", Label: "Planilha", Type: domain.Type{Kind: domain.KindSheet, Settings: map[string]string{domain.SettingModel: "sp-model"}}}
	saved, err := s.Save(context.Background(), "ws", editor("user-1"), in)
	if err != nil || saved.Type.Settings[domain.SettingModelTitle] != "Modelo Altiva" {
		t.Fatalf("Save = %+v, %v", saved, err)
	}
	if _, err := s.Save(context.Background(), "other", editor("user-1"), in); !errors.Is(err, domain.ErrTypeInvalid) {
		t.Fatalf("a refused preparation = %v", err)
	}
}
