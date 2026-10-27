package auth

import (
	"errors"
	"reflect"
	"testing"
)

func TestThePolicyInForceAsksForEightCharactersWithUpperLowerAndDigit(t *testing.T) {
	want := []PasswordRule{
		{Code: PasswordRuleMinLength, Min: 8},
		{Code: PasswordRuleUppercase, Min: 1},
		{Code: PasswordRuleLowercase, Min: 1},
		{Code: PasswordRuleDigit, Min: 1},
	}
	if got := CurrentPasswordPolicy().Rules; !reflect.DeepEqual(got, want) {
		t.Fatalf("rules = %+v, want %+v", got, want)
	}
}

func TestCallersCannotChangeThePolicyInForce(t *testing.T) {
	policy := CurrentPasswordPolicy()
	policy.Rules[0].Min = 1
	if CurrentPasswordPolicy().Rules[0].Min != 8 {
		t.Fatal("mutating a returned policy changed the policy in force")
	}
}

func TestUnmetListsEveryRuleThePasswordBreaksInPolicyOrder(t *testing.T) {
	tests := []struct {
		name     string
		password string
		unmet    []PasswordRuleCode
	}{
		{"a strong password meets every rule", "Abcdefg1", nil},
		{"symbols count toward the length", "MyP@ssw0rd", nil},
		{"digits and a symbol without letters miss both cases", "12345678901!", []PasswordRuleCode{PasswordRuleUppercase, PasswordRuleLowercase}},
		{"an empty password misses everything", "", []PasswordRuleCode{PasswordRuleMinLength, PasswordRuleUppercase, PasswordRuleLowercase, PasswordRuleDigit}},
		{"a short password misses the length", "Ab1", []PasswordRuleCode{PasswordRuleMinLength}},
		{"no uppercase", "abcdefgh1", []PasswordRuleCode{PasswordRuleUppercase}},
		{"no lowercase", "ABCDEFGH1", []PasswordRuleCode{PasswordRuleLowercase}},
		{"no digit", "Abcdefgh", []PasswordRuleCode{PasswordRuleDigit}},
		{"accented letters count as one character each", "Ááéíóú1x", nil},
		{"seven accented characters stay short", "Ááéíó1x", []PasswordRuleCode{PasswordRuleMinLength}},
		{"spaces are characters the person typed", " Abcdef1", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := CurrentPasswordPolicy().Unmet(tc.password); !reflect.DeepEqual(got, tc.unmet) {
				t.Fatalf("Unmet(%q) = %v, want %v", tc.password, got, tc.unmet)
			}
		})
	}
}

func TestARuleThePolicyCannotEvaluateIsNeverMet(t *testing.T) {
	policy := PasswordPolicy{Rules: []PasswordRule{{Code: "unknown_rule", Min: 1}}}
	if got := policy.Unmet("Abcdefgh1!"); !reflect.DeepEqual(got, []PasswordRuleCode{"unknown_rule"}) {
		t.Fatalf("Unmet = %v, want the unknown rule refused", got)
	}
}

func TestCheckRefusesAWeakPasswordWithTheRulesItBreaks(t *testing.T) {
	err := CurrentPasswordPolicy().Check("12345678901!")
	if !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("Check = %v, want ErrWeakPassword", err)
	}
	var weak *WeakPasswordError
	if !errors.As(err, &weak) {
		t.Fatalf("Check = %T, want *WeakPasswordError", err)
	}
	if !reflect.DeepEqual(weak.Unmet, []PasswordRuleCode{PasswordRuleUppercase, PasswordRuleLowercase}) {
		t.Fatalf("unmet = %v", weak.Unmet)
	}
}

func TestCheckAcceptsAPasswordThatMeetsEveryRule(t *testing.T) {
	if err := CurrentPasswordPolicy().Check("NovaSenha1"); err != nil {
		t.Fatalf("Check = %v, want nil", err)
	}
}
