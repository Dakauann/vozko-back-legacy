package variabledecl

import (
	"context"
	"errors"
	"testing"
)

type preparingRule struct {
	portRule
	err error
}

func (preparingRule) Kind() Kind { return KindSheet }

func (p preparingRule) Prepare(_ context.Context, workspaceID string, t Type) (Type, error) {
	if p.err != nil {
		return Type{}, p.err
	}
	t.Settings = map[string]string{SettingModel: t.Settings[SettingModel], SettingModelTitle: "Modelo de " + workspaceID}
	return t, nil
}

func TestPrepareLetsATypeReadTheWorkspaceBeforeTheDeclarationIsChecked(t *testing.T) {
	r, err := NewRegistry(append(Builtin(), preparingRule{})...)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.Prepare(context.Background(), "ws-1", Declaration{Name: "planilha", Type: Type{Kind: KindSheet, Settings: map[string]string{SettingModel: "sp-1"}}})
	if err != nil || got.Type.Settings[SettingModelTitle] != "Modelo de ws-1" || got.Type.Settings[SettingModel] != "sp-1" {
		t.Fatalf("Prepare = %+v, %v", got, err)
	}
	plain := Declaration{Name: "valor", Type: Type{Kind: KindNumber}}
	if got, err := r.Prepare(context.Background(), "ws-1", plain); err != nil || got.Type.Kind != KindNumber {
		t.Fatalf("a type without a preparer = %+v, %v", got, err)
	}
	broken := errors.New("sheets down")
	r2, _ := NewRegistry(append(Builtin(), preparingRule{err: broken})...)
	if _, err := r2.Prepare(context.Background(), "ws-1", Declaration{Type: Type{Kind: KindSheet}}); !errors.Is(err, broken) {
		t.Fatalf("a failing preparer = %v", err)
	}
}

func TestADeclaredDefaultIsReadFromTheTypeSettings(t *testing.T) {
	if got := DefaultOf(Declaration{Type: Type{Kind: KindSheet, Settings: map[string]string{SettingDefault: " sp-9 "}}}); got != "sp-9" {
		t.Fatalf("DefaultOf = %q", got)
	}
	if got := DefaultOf(Declaration{Type: Type{Kind: KindText}}); got != "" {
		t.Fatalf("DefaultOf = %q", got)
	}
}
