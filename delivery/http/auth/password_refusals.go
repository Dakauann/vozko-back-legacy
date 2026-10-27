package auth

import (
	"errors"
	"net/http"

	"vozko/delivery/http/response"
	authdomain "vozko/domain/auth"
)

const (
	fieldEmail           = "email"
	fieldResetCode       = "token"
	fieldNewPassword     = "newPassword"
	fieldCurrentPassword = "currentPassword"
)

type PasswordRefusalDetails struct {
	Field      string   `json:"field,omitempty" enums:"email,token,newPassword,currentPassword" example:"newPassword"`
	UnmetRules []string `json:"unmetRules,omitempty" example:"uppercase,lowercase"`
}

type passwordFlowRefusal struct {
	response.Refusal
	Field string
}

func refusalOn(field string, status int, code, message string) passwordFlowRefusal {
	return passwordFlowRefusal{Refusal: response.Refusal{Status: status, Code: code, Message: message}, Field: field}
}

var (
	refusalPasswordInvalidBody     = refusalOn("", http.StatusBadRequest, "invalid_body", "Requisição inválida")
	refusalPasswordUnauthorized    = refusalOn("", http.StatusUnauthorized, "unauthorized", "Faça login para continuar")
	refusalResetEmailRequired      = refusalOn(fieldEmail, http.StatusBadRequest, "reset_email_required", "Informe o e-mail da conta")
	refusalResetCodeRequired       = refusalOn(fieldResetCode, http.StatusBadRequest, "reset_code_required", "Informe o código de verificação")
	refusalNewPasswordRequired     = refusalOn(fieldNewPassword, http.StatusBadRequest, "new_password_required", "Informe a nova senha")
	refusalCurrentPasswordRequired = refusalOn(fieldCurrentPassword, http.StatusBadRequest, "current_password_required", "Informe a senha atual")
	refusalInvalidResetCode        = refusalOn(fieldResetCode, http.StatusBadRequest, "invalid_reset_code", "Código inválido ou expirado")
	refusalResetCodeUsed           = refusalOn(fieldResetCode, http.StatusBadRequest, "reset_code_used", "Este código já foi usado")
	refusalResetCodeAttempts       = refusalOn(fieldResetCode, http.StatusBadRequest, "reset_code_attempts_exceeded", "Muitas tentativas com código errado; peça um novo código")
	refusalCurrentPasswordInvalid  = refusalOn(fieldCurrentPassword, http.StatusUnauthorized, "current_password_invalid", "A senha atual está incorreta")
	refusalWeakPassword            = refusalOn(fieldNewPassword, http.StatusBadRequest, "weak_password", "A senha não atende aos requisitos")
	refusalPasswordResetFailed     = refusalOn("", http.StatusInternalServerError, "password_reset_failed", "Não foi possível redefinir a senha agora")
	refusalPasswordChangeFailed    = refusalOn("", http.StatusInternalServerError, "password_change_failed", "Não foi possível alterar a senha agora")
)

var passwordRuleRefusals = map[authdomain.PasswordRuleCode]passwordFlowRefusal{
	authdomain.PasswordRuleMinLength: refusalOn(fieldNewPassword, http.StatusBadRequest, "password_too_short", "A senha é curta demais"),
	authdomain.PasswordRuleUppercase: refusalOn(fieldNewPassword, http.StatusBadRequest, "password_missing_uppercase", "A senha precisa de uma letra maiúscula"),
	authdomain.PasswordRuleLowercase: refusalOn(fieldNewPassword, http.StatusBadRequest, "password_missing_lowercase", "A senha precisa de uma letra minúscula"),
	authdomain.PasswordRuleDigit:     refusalOn(fieldNewPassword, http.StatusBadRequest, "password_missing_digit", "A senha precisa de um número"),
}

var resetCodeRefusals = []struct {
	err     error
	refusal passwordFlowRefusal
}{
	{authdomain.ErrInvalidResetToken, refusalInvalidResetCode},
	{authdomain.ErrResetTokenUsed, refusalResetCodeUsed},
	{authdomain.ErrResetCodeAttemptsExceeded, refusalResetCodeAttempts},
	{authdomain.ErrCurrentPasswordInvalid, refusalCurrentPasswordInvalid},
}

func allPasswordFlowRefusals() []passwordFlowRefusal {
	all := []passwordFlowRefusal{
		refusalPasswordInvalidBody, refusalPasswordUnauthorized,
		refusalResetEmailRequired, refusalResetCodeRequired, refusalNewPasswordRequired, refusalCurrentPasswordRequired,
		refusalWeakPassword, refusalPasswordResetFailed, refusalPasswordChangeFailed,
	}
	for _, entry := range resetCodeRefusals {
		all = append(all, entry.refusal)
	}
	for _, refusal := range passwordRuleRefusals {
		all = append(all, refusal)
	}
	return all
}

func passwordFlowRefusalsOf(refusals []passwordFlowRefusal) []response.Refusal {
	out := make([]response.Refusal, len(refusals))
	for i, r := range refusals {
		out[i] = r.Refusal
	}
	return out
}

func writePasswordFlowRefusal(w http.ResponseWriter, refusal passwordFlowRefusal, unmet []authdomain.PasswordRuleCode) {
	details := PasswordRefusalDetails{Field: refusal.Field}
	for _, code := range unmet {
		details.UnmetRules = append(details.UnmetRules, string(code))
	}
	response.WriteErrorWithDetails(w, refusal.Status, refusal.Code, refusal.Message, details)
}

func writePasswordFlowError(w http.ResponseWriter, err error, unexpected passwordFlowRefusal) {
	var weak *authdomain.WeakPasswordError
	if errors.As(err, &weak) && len(weak.Unmet) > 0 {
		refusal, ok := passwordRuleRefusals[weak.Unmet[0]]
		if !ok {
			refusal = refusalWeakPassword
		}
		writePasswordFlowRefusal(w, refusal, weak.Unmet)
		return
	}
	if errors.Is(err, authdomain.ErrWeakPassword) {
		writePasswordFlowRefusal(w, refusalWeakPassword, nil)
		return
	}
	for _, entry := range resetCodeRefusals {
		if errors.Is(err, entry.err) {
			writePasswordFlowRefusal(w, entry.refusal, nil)
			return
		}
	}
	writePasswordFlowRefusal(w, unexpected, nil)
}
