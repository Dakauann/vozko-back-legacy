package auth

import "unicode"

type PasswordRuleCode string

const (
	PasswordRuleMinLength PasswordRuleCode = "min_length"
	PasswordRuleUppercase PasswordRuleCode = "uppercase"
	PasswordRuleLowercase PasswordRuleCode = "lowercase"
	PasswordRuleDigit     PasswordRuleCode = "digit"
)

type PasswordRule struct {
	Code PasswordRuleCode
	Min  int
}

type PasswordPolicy struct {
	Rules []PasswordRule
}

var passwordPolicyInForce = []PasswordRule{
	{Code: PasswordRuleMinLength, Min: 8},
	{Code: PasswordRuleUppercase, Min: 1},
	{Code: PasswordRuleLowercase, Min: 1},
	{Code: PasswordRuleDigit, Min: 1},
}

func CurrentPasswordPolicy() PasswordPolicy {
	return PasswordPolicy{Rules: append([]PasswordRule(nil), passwordPolicyInForce...)}
}

type WeakPasswordError struct {
	Unmet []PasswordRuleCode
}

func (e *WeakPasswordError) Error() string { return ErrWeakPassword.Error() }

func (e *WeakPasswordError) Is(target error) bool { return target == ErrWeakPassword }

func (p PasswordPolicy) Check(password string) error {
	if unmet := p.Unmet(password); len(unmet) > 0 {
		return &WeakPasswordError{Unmet: unmet}
	}
	return nil
}

func (p PasswordPolicy) Unmet(password string) []PasswordRuleCode {
	var unmet []PasswordRuleCode
	for _, rule := range p.Rules {
		if countMatching(rule.Code, password) < rule.Min {
			unmet = append(unmet, rule.Code)
		}
	}
	return unmet
}

func countMatching(code PasswordRuleCode, password string) int {
	matches, known := ruleCharacterClass[code]
	if !known {
		return -1
	}
	count := 0
	for _, r := range password {
		if matches(r) {
			count++
		}
	}
	return count
}

var ruleCharacterClass = map[PasswordRuleCode]func(rune) bool{
	PasswordRuleMinLength: func(rune) bool { return true },
	PasswordRuleUppercase: unicode.IsUpper,
	PasswordRuleLowercase: unicode.IsLower,
	PasswordRuleDigit:     unicode.IsDigit,
}
