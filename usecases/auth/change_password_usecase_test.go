package auth_usecase

import (
	"errors"
	"reflect"
	"testing"

	"vozko/domain/auth"
)

func TestChangePassword_RefusesAWeakPasswordNamingTheRulesItBreaks(t *testing.T) {
	userRepo := newTestUserRepo()
	seedResetUser(userRepo, "u1", "user@test.com", "bcrypt:Old1pass")

	uc := NewChangePasswordUseCase(userRepo, &testPasswordService{}, newTestSessionRepo(), newTestSharedState())

	err := uc.Execute(auth.ChangePasswordInput{UserID: "u1", CurrentPassword: "Old1pass", NewPassword: "12345678901!"})
	var weak *auth.WeakPasswordError
	if !errors.As(err, &weak) {
		t.Fatalf("expected a WeakPasswordError, got: %v", err)
	}
	if !reflect.DeepEqual(weak.Unmet, []auth.PasswordRuleCode{auth.PasswordRuleUppercase, auth.PasswordRuleLowercase}) {
		t.Fatalf("unmet = %v", weak.Unmet)
	}
	if userRepo.byID["u1"].Password != "bcrypt:Old1pass" {
		t.Fatal("a weak password must not replace the current one")
	}
}

func TestChangePassword_AcceptsAPasswordThatMeetsThePolicy(t *testing.T) {
	userRepo := newTestUserRepo()
	seedResetUser(userRepo, "u1", "user@test.com", "bcrypt:Old1pass")

	uc := NewChangePasswordUseCase(userRepo, &testPasswordService{}, newTestSessionRepo(), newTestSharedState())

	if err := uc.Execute(auth.ChangePasswordInput{UserID: "u1", CurrentPassword: "Old1pass", NewPassword: "NovaSenha1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if userRepo.byID["u1"].Password == "bcrypt:Old1pass" {
		t.Fatal("expected the password to change")
	}
}
