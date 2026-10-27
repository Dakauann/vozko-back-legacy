package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"vozko/delivery/http/response"
	"vozko/delivery/http/response/responsetest"
	authdomain "vozko/domain/auth"
	"vozko/pkg/codegolden"
)

type recordingResetPassword struct {
	err   error
	input authdomain.ResetPasswordInput
	calls int
}

func (s *recordingResetPassword) Execute(input authdomain.ResetPasswordInput) error {
	s.calls++
	s.input = input
	return s.err
}

type recordingChangePassword struct {
	err   error
	input authdomain.ChangePasswordInput
}

func (s *recordingChangePassword) Execute(input authdomain.ChangePasswordInput) error {
	s.input = input
	return s.err
}

type refusalBody struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details PasswordRefusalDetails `json:"details"`
}

func decodeRefusal(t *testing.T, rr *httptest.ResponseRecorder) refusalBody {
	t.Helper()
	var body refusalBody
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %q: %v", rr.Body.String(), err)
	}
	return body
}

func resetHandler(uc authdomain.ResetPasswordUseCase) *AuthHandler {
	return NewAuthHandler(nil, nil, nil, nil, nil, uc, nil, nil, nil, nil, nil, nil, nil, CookieConfig{})
}

func changeHandler(uc authdomain.ChangePasswordUseCase) *AuthHandler {
	return NewAuthHandler(nil, nil, nil, nil, nil, nil, nil, nil, uc, nil, nil, nil, nil, CookieConfig{})
}

func validReset() map[string]string {
	return map[string]string{"email": "user@test.com", "token": "123456", "newPassword": "NovaSenha1"}
}

func postChange(h *AuthHandler, userID string, body interface{}) *httptest.ResponseRecorder {
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/auth/change-password", bytes.NewReader(data))
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
	}
	rr := httptest.NewRecorder()
	h.ChangePassword(rr, req)
	return rr
}

func TestResetPassword_ACompleteRequestResetsThePassword(t *testing.T) {
	uc := &recordingResetPassword{}
	rr := postJSON(resetHandler(uc).ResetPassword, "/auth/reset-password", validReset())

	if rr.Code != http.StatusOK || uc.calls != 1 {
		t.Fatalf("status %d calls %d body %s", rr.Code, uc.calls, rr.Body.String())
	}
}

func TestResetPassword_ThePasswordReachesThePolicyExactlyAsTyped(t *testing.T) {
	uc := &recordingResetPassword{}
	body := validReset()
	body["newPassword"] = " NovaSenha1 "
	postJSON(resetHandler(uc).ResetPassword, "/auth/reset-password", body)

	if uc.input.NewPassword != " NovaSenha1 " {
		t.Fatalf("the handler changed the typed password to %q", uc.input.NewPassword)
	}
}

func TestResetPassword_EachRefusalNamesItsCodeAndField(t *testing.T) {
	tests := []struct {
		name   string
		body   map[string]string
		err    error
		status int
		code   string
		field  string
		unmet  []string
	}{
		{"missing email", map[string]string{"token": "123456", "newPassword": "NovaSenha1"}, nil, http.StatusBadRequest, "reset_email_required", "email", nil},
		{"missing code", map[string]string{"email": "user@test.com", "newPassword": "NovaSenha1"}, nil, http.StatusBadRequest, "reset_code_required", "token", nil},
		{"a code of spaces is missing", map[string]string{"email": "user@test.com", "token": "   ", "newPassword": "NovaSenha1"}, nil, http.StatusBadRequest, "reset_code_required", "token", nil},
		{"missing new password", map[string]string{"email": "user@test.com", "token": "123456"}, nil, http.StatusBadRequest, "new_password_required", "newPassword", nil},
		{"invalid or expired code", validReset(), authdomain.ErrInvalidResetToken, http.StatusBadRequest, "invalid_reset_code", "token", nil},
		{"used code", validReset(), authdomain.ErrResetTokenUsed, http.StatusBadRequest, "reset_code_used", "token", nil},
		{"too many wrong codes", validReset(), authdomain.ErrResetCodeAttemptsExceeded, http.StatusBadRequest, "reset_code_attempts_exceeded", "token", nil},
		{"letters missing on a digits and symbol password", validReset(), &authdomain.WeakPasswordError{Unmet: []authdomain.PasswordRuleCode{authdomain.PasswordRuleUppercase, authdomain.PasswordRuleLowercase}}, http.StatusBadRequest, "password_missing_uppercase", "newPassword", []string{"uppercase", "lowercase"}},
		{"too short", validReset(), &authdomain.WeakPasswordError{Unmet: []authdomain.PasswordRuleCode{authdomain.PasswordRuleMinLength}}, http.StatusBadRequest, "password_too_short", "newPassword", []string{"min_length"}},
		{"no lowercase", validReset(), &authdomain.WeakPasswordError{Unmet: []authdomain.PasswordRuleCode{authdomain.PasswordRuleLowercase}}, http.StatusBadRequest, "password_missing_lowercase", "newPassword", []string{"lowercase"}},
		{"no digit", validReset(), &authdomain.WeakPasswordError{Unmet: []authdomain.PasswordRuleCode{authdomain.PasswordRuleDigit}}, http.StatusBadRequest, "password_missing_digit", "newPassword", []string{"digit"}},
		{"a rule without its own refusal", validReset(), &authdomain.WeakPasswordError{Unmet: []authdomain.PasswordRuleCode{"symbol"}}, http.StatusBadRequest, "weak_password", "newPassword", []string{"symbol"}},
		{"a weak password without rules", validReset(), authdomain.ErrWeakPassword, http.StatusBadRequest, "weak_password", "newPassword", nil},
		{"an unexpected failure", validReset(), errors.New("db error"), http.StatusInternalServerError, "password_reset_failed", "", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rr := postJSON(resetHandler(&recordingResetPassword{err: tc.err}).ResetPassword, "/auth/reset-password", tc.body)
			body := decodeRefusal(t, rr)
			if rr.Code != tc.status || body.Code != tc.code || body.Details.Field != tc.field || !reflect.DeepEqual(body.Details.UnmetRules, tc.unmet) {
				t.Fatalf("got %d %s", rr.Code, rr.Body.String())
			}
			if strings.Contains(rr.Body.String(), "Validation failed") || body.Message == "" {
				t.Fatalf("refusal must carry its own fallback message, got %s", rr.Body.String())
			}
		})
	}
}

func TestResetPassword_AnUnreadableBodyIsCoded(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/auth/reset-password", bytes.NewReader([]byte("bad")))
	rr := httptest.NewRecorder()
	resetHandler(&recordingResetPassword{}).ResetPassword(rr, req)

	if rr.Code != http.StatusBadRequest || decodeRefusal(t, rr).Code != "invalid_body" {
		t.Fatalf("got %d %s", rr.Code, rr.Body.String())
	}
}

func TestForgotPassword_RefusalsAreCoded(t *testing.T) {
	rr := postJSON(newTestHandler().ForgotPassword, "/auth/forgot-password", map[string]string{"email": "  "})
	if body := decodeRefusal(t, rr); rr.Code != http.StatusBadRequest || body.Code != "reset_email_required" || body.Details.Field != "email" {
		t.Fatalf("got %d %s", rr.Code, rr.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/auth/forgot-password", bytes.NewReader([]byte("bad")))
	bad := httptest.NewRecorder()
	newTestHandler().ForgotPassword(bad, req)
	if bad.Code != http.StatusBadRequest || decodeRefusal(t, bad).Code != "invalid_body" {
		t.Fatalf("got %d %s", bad.Code, bad.Body.String())
	}
}

func TestChangePassword_EachRefusalNamesItsCodeAndField(t *testing.T) {
	tests := []struct {
		name   string
		userID string
		body   map[string]string
		err    error
		status int
		code   string
		field  string
	}{
		{"no session", "", map[string]string{"currentPassword": "Old1pass", "newPassword": "NovaSenha1"}, nil, http.StatusUnauthorized, "unauthorized", ""},
		{"missing current password", "u1", map[string]string{"newPassword": "NovaSenha1"}, nil, http.StatusBadRequest, "current_password_required", "currentPassword"},
		{"missing new password", "u1", map[string]string{"currentPassword": "Old1pass"}, nil, http.StatusBadRequest, "new_password_required", "newPassword"},
		{"wrong current password", "u1", map[string]string{"currentPassword": "nope", "newPassword": "NovaSenha1"}, authdomain.ErrCurrentPasswordInvalid, http.StatusUnauthorized, "current_password_invalid", "currentPassword"},
		{"weak new password", "u1", map[string]string{"currentPassword": "Old1pass", "newPassword": "123456789"}, &authdomain.WeakPasswordError{Unmet: []authdomain.PasswordRuleCode{authdomain.PasswordRuleUppercase}}, http.StatusBadRequest, "password_missing_uppercase", "newPassword"},
		{"an unexpected failure", "u1", map[string]string{"currentPassword": "Old1pass", "newPassword": "NovaSenha1"}, errors.New("db error"), http.StatusInternalServerError, "password_change_failed", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rr := postChange(changeHandler(&recordingChangePassword{err: tc.err}), tc.userID, tc.body)
			body := decodeRefusal(t, rr)
			if rr.Code != tc.status || body.Code != tc.code || body.Details.Field != tc.field {
				t.Fatalf("got %d %s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestPasswordPolicy_DescribesTheRulesInForce(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/auth/password-policy", nil)
	rr := httptest.NewRecorder()
	newTestHandler().PasswordPolicy(rr, req)

	var body PasswordPolicyResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := PasswordPolicyResponse{Rules: []PasswordRuleResponse{
		{Code: "min_length", Min: 8},
		{Code: "uppercase", Min: 1},
		{Code: "lowercase", Min: 1},
		{Code: "digit", Min: 1},
	}}
	if rr.Code != http.StatusOK || !reflect.DeepEqual(body, want) {
		t.Fatalf("got %d %+v", rr.Code, body)
	}
}

func TestEveryRuleInForceHasItsOwnRefusal(t *testing.T) {
	for _, rule := range authdomain.CurrentPasswordPolicy().Rules {
		if _, ok := passwordRuleRefusals[rule.Code]; !ok {
			t.Errorf("rule %s has no refusal; add one so the person learns which rule failed", rule.Code)
		}
	}
}

func TestPasswordRefusalsUseStableCodes(t *testing.T) {
	responsetest.RequireStableCodes(t, passwordFlowRefusalsOf(allPasswordFlowRefusals()))
}

func TestPasswordErrorCodesMatchTheGoldenSoTheFrontTranslatesThem(t *testing.T) {
	codes := response.RefusalCodes(passwordFlowRefusalsOf(allPasswordFlowRefusals()))
	codegolden.Compare(t, filepath.Join("testdata", "password_error_codes.golden"), codes,
		"update testdata/password_error_codes.golden, and translate auth.errors.<code> in the pt, en, es and de catalogs of vozko-front")
}

func TestPasswordRuleCodesMatchTheGoldenSoTheFrontLabelsThem(t *testing.T) {
	var codes []string
	for _, rule := range authdomain.CurrentPasswordPolicy().Rules {
		codes = append(codes, string(rule.Code))
	}
	codegolden.Compare(t, filepath.Join("testdata", "password_rule_codes.golden"), codes,
		"update testdata/password_rule_codes.golden, and label auth.password.rules.<code> in the pt, en, es and de catalogs of vozko-front")
}
