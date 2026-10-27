package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"vozko/delivery/http/response"
	authdomain "vozko/domain/auth"
	"vozko/domain/user"
	"vozko/infra/http/middleware"
)

type CookieConfig struct {
	Domain        string
	Secure        bool
	AccessMaxAge  time.Duration
	RefreshMaxAge time.Duration
}

type AuthHandler struct {
	credentials           authdomain.CredentialsLoginUseCase
	register              authdomain.RegisterUseCase
	adminRegister         authdomain.AdminRegisterUseCase
	refreshToken          authdomain.RefreshTokenUseCase
	requestPasswordReset  authdomain.RequestPasswordResetUseCase
	resetPassword         authdomain.ResetPasswordUseCase
	sendEmailVerification authdomain.SendEmailVerificationUseCase
	verifyEmail           authdomain.VerifyEmailTokenUseCase
	changePassword        authdomain.ChangePasswordUseCase
	logout                authdomain.LogoutUseCase
	logoutAll             authdomain.LogoutAllUseCase
	listSessions          authdomain.ListSessionsUseCase
	revokeSession         authdomain.RevokeSessionUseCase
	cookieCfg             CookieConfig
}

func NewAuthHandler(
	credentials authdomain.CredentialsLoginUseCase,
	register authdomain.RegisterUseCase,
	adminRegister authdomain.AdminRegisterUseCase,
	refreshToken authdomain.RefreshTokenUseCase,
	requestPasswordReset authdomain.RequestPasswordResetUseCase,
	resetPassword authdomain.ResetPasswordUseCase,
	sendEmailVerification authdomain.SendEmailVerificationUseCase,
	verifyEmail authdomain.VerifyEmailTokenUseCase,
	changePassword authdomain.ChangePasswordUseCase,
	logout authdomain.LogoutUseCase,
	logoutAll authdomain.LogoutAllUseCase,
	listSessions authdomain.ListSessionsUseCase,
	revokeSession authdomain.RevokeSessionUseCase,
	cookies CookieConfig,
) *AuthHandler {
	return &AuthHandler{
		cookieCfg:             cookies,
		credentials:           credentials,
		register:              register,
		adminRegister:         adminRegister,
		refreshToken:          refreshToken,
		requestPasswordReset:  requestPasswordReset,
		resetPassword:         resetPassword,
		sendEmailVerification: sendEmailVerification,
		verifyEmail:           verifyEmail,
		changePassword:        changePassword,
		logout:                logout,
		logoutAll:             logoutAll,
		listSessions:          listSessions,
		revokeSession:         revokeSession,
	}
}

func extractClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {

		if idx := strings.IndexByte(xff, ','); idx > 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx > 0 {
		return addr[:idx]
	}
	return addr
}

func extractUserAgent(r *http.Request) string {
	return r.Header.Get("User-Agent")
}

func resolveRegistrationReferralCode(r *http.Request, payloadCode string) string {
	if code := strings.TrimSpace(payloadCode); code != "" {
		return code
	}
	return middleware.ExtractReferralCode(r)
}

// @Summary		Login com email e senha
// @Description	Autentica um usuário com credenciais e retorna um par de tokens de acesso e atualização. Clientes SPA de navegador que enviam `X-Auth-Mode: cookie` recebem os tokens como cookies httpOnly e os campos de token são omitidos do corpo.
// @Tags			Autenticação
// @Accept			json
// @Produce		json
// @Param			request	body		LoginRequest	true	"Credenciais"
// @Success		200		{object}	AuthTokenResponse
// @Failure		400		{object}	response.ErrorResponse
// @Failure		401		{object}	response.ErrorResponse	"AUTH_INVALID_CREDENTIALS"
// @Failure		429		{object}	response.ErrorResponse	"AUTH_RATE_LIMIT_EXCEEDED"
// @Failure		500		{object}	response.ErrorResponse
// @Router			/auth/login [post]
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteInvalidBodyError(w, map[string]string{
			"email":    "string (required)",
			"password": "string (required)",
		})
		return
	}

	if errs := req.Validate(); errs != nil {
		response.WriteValidationError(w, errs)
		return
	}

	tokens, err := h.credentials.Execute(authdomain.CredentialsInput{
		Email:      req.Email,
		Password:   req.Password,
		IPAddress:  extractClientIP(r),
		DeviceInfo: extractUserAgent(r),
	})
	if err != nil {
		var tooMany *authdomain.TooManyAttemptsError
		if errors.As(err, &tooMany) {
			if tooMany.RetryAfter > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(int(tooMany.RetryAfter.Seconds())))
			}
			response.WriteErrorWithCode(w, http.StatusTooManyRequests, AuthCodeRateLimitExceeded, "Too many login attempts. Please try again later.", map[string]string{
				"retry_after": tooMany.RetryAfter.String(),
			})
			return
		}
		if errors.Is(err, authdomain.ErrInvalidCredentials) {
			response.WriteErrorWithCode(w, http.StatusUnauthorized, AuthCodeInvalidCredentials, "Invalid credentials", nil)
			return
		}
		response.WriteErrorWithCode(w, http.StatusInternalServerError, AuthCodeInternal, "Authentication failed", nil)
		return
	}

	h.writeTokenResponse(w, r, tokens)
}

// @Summary		Cadastrar uma nova conta
// @Description	Cria uma nova conta de cliente após a verificação do email. Exige um `verificationToken` válido e um CPF (pessoa física) ou CNPJ (pessoa jurídica) correspondente ao `customerType`.
// @Tags			Autenticação
// @Accept			json
// @Produce		json
// @Param			request	body		RegisterRequest	true	"Dados de cadastro"
// @Success		200		{object}	AuthTokenResponse
// @Failure		400		{object}	response.ErrorResponse
// @Failure		409		{object}	response.ErrorResponse	"AUTH_EMAIL_ALREADY_EXISTS / AUTH_DOCUMENT_ALREADY_EXISTS"
// @Failure		422		{object}	response.ErrorResponse	"AUTH_INVALID_DOCUMENT"
// @Failure		500		{object}	response.ErrorResponse
// @Router			/auth/register [post]
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteInvalidBodyError(w, map[string]string{
			"name":              "string (required)",
			"email":             "string (required)",
			"password":          "string (required)",
			"customerType":      "'individual' or 'company' (required)",
			"cpf":               "string (required when customerType='individual')",
			"cnpj":              "string (required when customerType='company')",
			"verificationToken": "string (required)",
		})
		return
	}

	if errs := req.Validate(); errs != nil {
		response.WriteValidationError(w, errs)
		return
	}
	customerType := req.CustomerType

	tokens, err := h.register.Execute(authdomain.CredentialsInput{
		Name:              req.Name,
		Email:             req.Email,
		Password:          req.Password,
		CustomerType:      customerType,
		CPF:               req.CPF,
		CNPJ:              req.CNPJ,
		VerificationToken: req.VerificationToken,
		IPAddress:         extractClientIP(r),
		DeviceInfo:        extractUserAgent(r),
		ReferralCode:      resolveRegistrationReferralCode(r, req.ReferralCode),
	})
	if err != nil {
		if errors.Is(err, authdomain.ErrInvalidVerificationToken) {
			response.WriteErrorWithCode(w, http.StatusBadRequest, AuthCodeInvalidVerificationTok, "Invalid or expired verification token", nil)
			return
		}
		if errors.Is(err, authdomain.ErrVerificationTokenUsed) {
			response.WriteErrorWithCode(w, http.StatusBadRequest, AuthCodeVerificationTokUsed, "Verification token already used", nil)
			return
		}
		if errors.Is(err, authdomain.ErrMissingVerificationToken) {
			response.WriteErrorWithCode(w, http.StatusBadRequest, AuthCodeMissingVerificationTok, "Verification code required", map[string]string{"verificationToken": "required"})
			return
		}
		if errors.Is(err, authdomain.ErrEmailAlreadyExists) {
			response.WriteErrorWithCode(w, http.StatusConflict, AuthCodeEmailAlreadyExists, "Email already exists", nil)
			return
		}
		if errors.Is(err, authdomain.ErrDocumentAlreadyExists) {

			field := "cpf"
			if customerType == string(user.CustomerTypeCompany) {
				field = "cnpj"
			}
			response.WriteErrorWithCode(w, http.StatusConflict, AuthCodeDocumentAlreadyExists, "Document already linked to another account", map[string]string{field: "already_exists"})
			return
		}
		if errors.Is(err, authdomain.ErrWeakPassword) {
			response.WriteErrorWithCode(w, http.StatusBadRequest, AuthCodeWeakPassword, "Password does not meet security requirements", map[string]string{"password": "weak"})
			return
		}
		if errors.Is(err, authdomain.ErrInvalidCustomerType) {
			response.WriteErrorWithCode(w, http.StatusBadRequest, AuthCodeInvalidCustomerType, "Invalid customer type", map[string]string{"customerType": "must be 'individual' or 'company'"})
			return
		}
		if errors.Is(err, authdomain.ErrMissingCustomerDocument) {
			field := "cpf"
			if customerType == string(user.CustomerTypeCompany) {
				field = "cnpj"
			}
			response.WriteErrorWithCode(w, http.StatusBadRequest, AuthCodeMissingDocument, "Document required", map[string]string{field: "required"})
			return
		}
		if errors.Is(err, authdomain.ErrInvalidCustomerDocument) {
			field := "cpf"
			if customerType == string(user.CustomerTypeCompany) {
				field = "cnpj"
			}
			response.WriteErrorWithCode(w, http.StatusUnprocessableEntity, AuthCodeInvalidDocument, "Invalid document", map[string]string{field: "invalid"})
			return
		}
		response.WriteErrorWithCode(w, http.StatusInternalServerError, AuthCodeInternal, "Registration failed", nil)
		return
	}

	h.writeTokenResponse(w, r, tokens)
}

func (h *AuthHandler) AdminRegister(w http.ResponseWriter, r *http.Request) {
	var req AdminRegisterRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteInvalidBodyError(w, map[string]string{
			"name":         "string (required)",
			"email":        "string (required)",
			"password":     "string (required)",
			"customerType": "'individual' or 'company' (required)",
			"cpf":          "string (required when customerType='individual')",
			"cnpj":         "string (required when customerType='company')",
		})
		return
	}

	if errs := req.Validate(); errs != nil {
		response.WriteValidationError(w, errs)
		return
	}
	customerType := req.CustomerType

	result, err := h.adminRegister.Execute(authdomain.CredentialsInput{
		Name:         req.Name,
		Email:        req.Email,
		Password:     req.Password,
		CustomerType: customerType,
		CPF:          req.CPF,
		CNPJ:         req.CNPJ,
	})
	if err != nil {
		if errors.Is(err, authdomain.ErrEmailAlreadyExists) {
			response.WriteError(w, http.StatusConflict, "Email already exists", nil)
			return
		}
		if errors.Is(err, authdomain.ErrInvalidCustomerType) {
			response.WriteValidationError(w, map[string]string{"customerType": "must be 'individual' or 'company'"})
			return
		}
		if errors.Is(err, authdomain.ErrMissingCustomerDocument) {
			field := "cpf"
			if customerType == string(user.CustomerTypeCompany) {
				field = "cnpj"
			}
			response.WriteValidationError(w, map[string]string{field: "required"})
			return
		}
		if errors.Is(err, authdomain.ErrInvalidCustomerDocument) {
			field := "cpf"
			if customerType == string(user.CustomerTypeCompany) {
				field = "cnpj"
			}
			response.WriteValidationError(w, map[string]string{field: "invalid"})
			return
		}
		response.WriteError(w, http.StatusInternalServerError, "Registration failed", nil)
		return
	}

	response.WriteSuccess(w, http.StatusCreated, AdminRegisterResponse{
		UserID:       result.UserID,
		Email:        result.Email,
		Name:         result.Name,
		Role:         result.Role,
		CustomerType: result.CustomerType,
		Message:      "User registered successfully",
	})
}

// @Summary		Renovar o token de acesso
// @Description	Rotaciona o token de atualização e retorna um novo par de tokens. O token de atualização pode ser enviado no corpo ou, para clientes SPA de navegador, como o cookie httpOnly `refreshToken`.
// @Tags			Autenticação
// @Accept			json
// @Produce		json
// @Param			request	body		RefreshTokenRequest	false	"Token de atualização (opcional quando enviado como cookie)"
// @Success		200		{object}	AuthTokenResponse
// @Failure		400		{object}	response.ErrorResponse
// @Failure		401		{object}	response.ErrorResponse
// @Failure		500		{object}	response.ErrorResponse
// @Router			/auth/refresh [post]
func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	var req RefreshTokenRequest

	_ = json.NewDecoder(r.Body).Decode(&req)

	if req.RefreshToken == "" {
		if c, err := r.Cookie("refreshToken"); err == nil {
			req.RefreshToken = c.Value
		}
	}

	if req.RefreshToken == "" {
		response.WriteValidationError(w, map[string]string{"refreshToken": "required in body or as httpOnly cookie"})
		return
	}

	tokens, err := h.refreshToken.Execute(req.RefreshToken, extractClientIP(r), extractUserAgent(r))
	if err != nil {
		if errors.Is(err, authdomain.ErrInvalidCredentials) ||
			errors.Is(err, authdomain.ErrSessionRevoked) ||
			errors.Is(err, authdomain.ErrSessionExpired) {
			response.WriteError(w, http.StatusUnauthorized, "Invalid refresh token", nil)
			return
		}
		response.WriteError(w, http.StatusInternalServerError, "Token refresh failed", nil)
		return
	}

	h.writeTokenResponse(w, r, tokens)
}

func (h *AuthHandler) cookieMode(r *http.Request) bool {
	return h.cookieCfg.Domain != "" &&
		strings.EqualFold(r.Header.Get("X-Auth-Mode"), "cookie")
}

func (h *AuthHandler) writeTokenResponse(w http.ResponseWriter, r *http.Request, tokens *authdomain.TokenPair) {
	resp := AuthTokenResponse{
		TokenType:    "Bearer",
		UserID:       tokens.UserID,
		Email:        tokens.Email,
		Name:         tokens.Name,
		Picture:      tokens.Picture,
		Role:         tokens.Role,
		CustomerType: tokens.CustomerType,
	}

	if h.cookieMode(r) {
		h.setAuthCookies(w, tokens)
	} else {
		resp.AccessToken = tokens.AccessToken
		resp.RefreshToken = tokens.RefreshToken
	}

	response.WriteSuccess(w, http.StatusOK, resp)
}

type authCookieUserData struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	Picture      string `json:"picture"`
	Role         string `json:"role"`
	CustomerType string `json:"customerType"`
}

func encodeUserDataCookie(tokens *authdomain.TokenPair) string {
	payload, err := json.Marshal(authCookieUserData{
		ID:           tokens.UserID,
		Email:        tokens.Email,
		Name:         tokens.Name,
		Picture:      tokens.Picture,
		Role:         tokens.Role,
		CustomerType: tokens.CustomerType,
	})
	if err != nil {
		return ""
	}

	return strings.ReplaceAll(url.QueryEscape(string(payload)), "+", "%20")
}

func (h *AuthHandler) setAuthCookies(w http.ResponseWriter, tokens *authdomain.TokenPair) {
	if h.cookieCfg.Domain == "" {
		return
	}

	sameSite := http.SameSiteLaxMode

	http.SetCookie(w, &http.Cookie{
		Name:     "accessToken",
		Value:    tokens.AccessToken,
		Path:     "/",
		Domain:   h.cookieCfg.Domain,
		MaxAge:   int(h.cookieCfg.AccessMaxAge.Seconds()),
		HttpOnly: true,
		Secure:   h.cookieCfg.Secure,
		SameSite: sameSite,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     "refreshToken",
		Value:    tokens.RefreshToken,
		Path:     "/auth/refresh",
		Domain:   h.cookieCfg.Domain,
		MaxAge:   int(h.cookieCfg.RefreshMaxAge.Seconds()),
		HttpOnly: true,
		Secure:   h.cookieCfg.Secure,
		SameSite: sameSite,
	})

	userData := encodeUserDataCookie(tokens)
	if userData == "" {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "userData",
		Value:    userData,
		Path:     "/",
		Domain:   h.cookieCfg.Domain,
		MaxAge:   int(h.cookieCfg.RefreshMaxAge.Seconds()),
		HttpOnly: false,
		Secure:   h.cookieCfg.Secure,
		SameSite: sameSite,
	})
}

func (h *AuthHandler) clearAuthCookies(w http.ResponseWriter) {
	if h.cookieCfg.Domain == "" {
		return
	}

	for _, name := range []string{"accessToken", "refreshToken", "userData"} {
		path := "/"
		if name == "refreshToken" {
			path = "/auth/refresh"
		}
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     path,
			Domain:   h.cookieCfg.Domain,
			MaxAge:   -1,
			HttpOnly: name != "userData",
			Secure:   h.cookieCfg.Secure,
			SameSite: http.SameSiteLaxMode,
		})
	}
}

// @Summary		Solicitar redefinição de senha
// @Description	Envia um link de redefinição de senha se existir uma conta com o email informado. Sempre retorna 200 para não revelar quais emails estão cadastrados.
// @Tags			Autenticação
// @Accept			json
// @Produce		json
// @Param			request	body		ForgotPasswordRequest	true	"Email da conta"
// @Success		200		{object}	MessageResponse
// @Failure		400		{object}	response.ErrorResponse{details=PasswordRefusalDetails}	"invalid_body / reset_email_required"
// @Failure		429		{object}	response.ErrorResponse
// @Router			/auth/forgot-password [post]
func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req ForgotPasswordRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writePasswordFlowRefusal(w, refusalPasswordInvalidBody, nil)
		return
	}

	if refusal, ok := req.Validate(); !ok {
		writePasswordFlowRefusal(w, refusal, nil)
		return
	}

	_ = h.requestPasswordReset.Execute(authdomain.RequestPasswordResetInput{
		Email: req.Email,
	})

	response.WriteSuccess(w, http.StatusOK, MessageResponse{
		Message: "If an account with that email exists, a password reset link has been sent",
	})
}

// @Summary		Redefinir a senha com um código
// @Description	Conclui a redefinição de senha usando o código enviado por email. A nova senha segue as regras de `GET /auth/password-policy` e é usada exatamente como digitada.
// @Description
// @Description	Toda recusa traz um `code` estável e, em `details`, o campo envolvido (`field`) e, quando a senha é fraca, todas as regras não atendidas (`unmetRules`).
// @Description
// @Description	| code | Quando |
// @Description	| --- | --- |
// @Description	| `invalid_body` | Corpo ilegível |
// @Description	| `reset_email_required` | Email ausente |
// @Description	| `reset_code_required` | Código ausente |
// @Description	| `new_password_required` | Nova senha ausente |
// @Description	| `password_too_short` | Menos caracteres que o mínimo |
// @Description	| `password_missing_uppercase` | Sem letra maiúscula |
// @Description	| `password_missing_lowercase` | Sem letra minúscula |
// @Description	| `password_missing_digit` | Sem número |
// @Description	| `weak_password` | Outra regra não atendida |
// @Description	| `invalid_reset_code` | Código inválido ou expirado |
// @Description	| `reset_code_used` | Código já usado |
// @Description	| `reset_code_attempts_exceeded` | Tentativas esgotadas; peça um novo código |
// @Description	| `password_reset_failed` | Falha inesperada |
// @Tags			Autenticação
// @Accept			json
// @Produce		json
// @Param			request	body		ResetPasswordRequest	true	"Email, código de redefinição e nova senha"
// @Success		200		{object}	MessageResponse
// @Failure		400		{object}	response.ErrorResponse{details=PasswordRefusalDetails}
// @Failure		429		{object}	response.ErrorResponse
// @Failure		500		{object}	response.ErrorResponse{details=PasswordRefusalDetails}
// @Router			/auth/reset-password [post]
func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req ResetPasswordRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writePasswordFlowRefusal(w, refusalPasswordInvalidBody, nil)
		return
	}

	if refusal, ok := req.Validate(); !ok {
		writePasswordFlowRefusal(w, refusal, nil)
		return
	}

	err := h.resetPassword.Execute(authdomain.ResetPasswordInput{
		Email:       req.Email,
		Token:       req.Token,
		NewPassword: req.NewPassword,
	})
	if err != nil {
		writePasswordFlowError(w, err, refusalPasswordResetFailed)
		return
	}

	response.WriteSuccess(w, http.StatusOK, MessageResponse{
		Message: "Password has been reset successfully",
	})
}

// @Summary		Regras de senha
// @Description	Lista as regras que toda nova senha precisa atender (redefinição e troca de senha). Cada regra tem um `code` estável e o mínimo exigido em `min`: `min_length` conta caracteres, `uppercase` letras maiúsculas, `lowercase` letras minúsculas e `digit` números. A validação acontece no servidor; esta lista serve para mostrar os requisitos enquanto a pessoa digita.
// @Tags			Autenticação
// @Produce		json
// @Success		200	{object}	PasswordPolicyResponse
// @Router			/auth/password-policy [get]
func (h *AuthHandler) PasswordPolicy(w http.ResponseWriter, r *http.Request) {
	response.WriteSuccess(w, http.StatusOK, passwordPolicyResponseOf(authdomain.CurrentPasswordPolicy()))
}

func passwordPolicyResponseOf(policy authdomain.PasswordPolicy) PasswordPolicyResponse {
	rules := make([]PasswordRuleResponse, len(policy.Rules))
	for i, rule := range policy.Rules {
		rules[i] = PasswordRuleResponse{Code: string(rule.Code), Min: rule.Min}
	}
	return PasswordPolicyResponse{Rules: rules}
}

// @Summary		Enviar um código de verificação de email
// @Description	Coloca na fila um email de verificação para o endereço informado. Possui limite de envio por email.
// @Tags			Autenticação
// @Accept			json
// @Produce		json
// @Param			request	body		SendEmailVerificationRequest	true	"Email a verificar"
// @Success		202		{object}	EmailVerificationQueuedResponse
// @Failure		400		{object}	response.ErrorResponse
// @Failure		429		{object}	response.ErrorResponse	"AUTH_RATE_LIMIT_EXCEEDED"
// @Failure		500		{object}	response.ErrorResponse
// @Router			/auth/send-email-verification [post]
func (h *AuthHandler) SendEmailVerification(w http.ResponseWriter, r *http.Request) {
	var req SendEmailVerificationRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteInvalidBodyError(w, map[string]string{
			"email": "string (required)",
		})
		return
	}

	if errs := req.Validate(); errs != nil {
		response.WriteValidationError(w, errs)
		return
	}

	if err := h.sendEmailVerification.Execute(req.Email); err != nil {
		if errors.Is(err, authdomain.ErrEmailVerificationRateExceeded) {
			response.WriteErrorWithCode(w, http.StatusTooManyRequests, AuthCodeRateLimitExceeded, "Too many verification requests. Please try again later.", nil)
			return
		}
		response.WriteErrorWithCode(w, http.StatusInternalServerError, AuthCodeEmailDeliveryFail, "Failed to send verification email", nil)
		return
	}

	response.WriteSuccess(w, http.StatusAccepted, EmailVerificationQueuedResponse{
		Message: "Verification email queued",
		Success: true,
	})
}

// @Summary		Verificar um email com um token
// @Description	Marca o email do usuário como verificado usando o token enviado a ele por email.
// @Tags			Autenticação
// @Accept			json
// @Produce		json
// @Param			request	body		VerifyEmailRequest	true	"Token de verificação"
// @Success		200		{object}	MessageResponse
// @Failure		400		{object}	response.ErrorResponse
// @Failure		500		{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/auth/verify-email [post]
func (h *AuthHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req VerifyEmailRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteInvalidBodyError(w, map[string]string{
			"token": "string (required)",
		})
		return
	}

	if errs := req.Validate(); errs != nil {
		response.WriteValidationError(w, errs)
		return
	}

	if err := h.verifyEmail.Execute(req.Token); err != nil {
		if errors.Is(err, authdomain.ErrInvalidVerificationToken) {
			response.WriteError(w, http.StatusBadRequest, "Invalid or expired verification token", nil)
			return
		}
		if errors.Is(err, authdomain.ErrVerificationTokenUsed) {
			response.WriteError(w, http.StatusBadRequest, "Verification token already used", nil)
			return
		}
		response.WriteError(w, http.StatusInternalServerError, "Email verification failed", nil)
		return
	}

	response.WriteSuccess(w, http.StatusOK, MessageResponse{
		Message: "Email verified successfully",
	})
}

// @Summary		Alterar a senha do usuário atual
// @Description	Altera a senha do usuário autenticado e revoga todas as suas sessões. Exige a senha atual. A nova senha segue as regras de `GET /auth/password-policy`.
// @Description
// @Description	Toda recusa traz um `code` estável e, em `details`, o campo envolvido (`field`) e, quando a senha é fraca, todas as regras não atendidas (`unmetRules`).
// @Description
// @Description	| code | Quando |
// @Description	| --- | --- |
// @Description	| `unauthorized` | Sem sessão |
// @Description	| `invalid_body` | Corpo ilegível |
// @Description	| `current_password_required` | Senha atual ausente |
// @Description	| `new_password_required` | Nova senha ausente |
// @Description	| `current_password_invalid` | Senha atual incorreta |
// @Description	| `password_too_short`, `password_missing_uppercase`, `password_missing_lowercase`, `password_missing_digit`, `weak_password` | Nova senha fora das regras |
// @Description	| `password_change_failed` | Falha inesperada |
// @Tags			Autenticação
// @Accept			json
// @Produce		json
// @Param			request	body		ChangePasswordRequest	true	"Senha atual e nova senha"
// @Success		200		{object}	MessageResponse
// @Failure		400		{object}	response.ErrorResponse{details=PasswordRefusalDetails}
// @Failure		401		{object}	response.ErrorResponse{details=PasswordRefusalDetails}
// @Failure		500		{object}	response.ErrorResponse{details=PasswordRefusalDetails}
// @Security		BearerAuth
// @Router			/auth/change-password [post]
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		writePasswordFlowRefusal(w, refusalPasswordUnauthorized, nil)
		return
	}

	var req ChangePasswordRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writePasswordFlowRefusal(w, refusalPasswordInvalidBody, nil)
		return
	}

	if refusal, ok := req.Validate(); !ok {
		writePasswordFlowRefusal(w, refusal, nil)
		return
	}

	err := h.changePassword.Execute(authdomain.ChangePasswordInput{
		UserID:          userID,
		CurrentPassword: req.CurrentPassword,
		NewPassword:     req.NewPassword,
	})
	if err != nil {
		writePasswordFlowError(w, err, refusalPasswordChangeFailed)
		return
	}

	response.WriteSuccess(w, http.StatusOK, MessageResponse{
		Message: "Password changed successfully. All sessions have been revoked.",
	})
}

// @Summary		Encerrar uma sessão
// @Description	Revoga a sessão do usuário (ou a sessão indicada por `sessionId`) e limpa os cookies de autenticação.
// @Tags			Autenticação
// @Accept			json
// @Produce		json
// @Param			request	body		LogoutRequest	false	"ID de sessão opcional (padrão é a sessão atual)"
// @Success		200		{object}	MessageResponse
// @Failure		401		{object}	response.ErrorResponse
// @Failure		404		{object}	response.ErrorResponse
// @Failure		500		{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/auth/logout [post]
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		response.WriteError(w, http.StatusUnauthorized, "Authentication required", nil)
		return
	}

	var callerJTI string
	if claims, ok := r.Context().Value(middleware.ClaimsContextKey).(*authdomain.Claims); ok && claims != nil {
		callerJTI = claims.JTI
	}

	var req LogoutRequest

	_ = json.NewDecoder(r.Body).Decode(&req)

	if err := h.logout.Execute(userID, req.SessionID, callerJTI); err != nil {
		if errors.Is(err, authdomain.ErrSessionNotFound) {
			response.WriteError(w, http.StatusNotFound, "Session not found", nil)
			return
		}
		response.WriteError(w, http.StatusInternalServerError, "Logout failed", nil)
		return
	}

	h.clearAuthCookies(w)

	response.WriteSuccess(w, http.StatusOK, MessageResponse{
		Message: "Logged out successfully",
	})
}

// @Summary		Encerrar todas as sessões
// @Description	Revoga todas as sessões do usuário autenticado e limpa os cookies de autenticação.
// @Tags			Autenticação
// @Produce		json
// @Success		200	{object}	MessageResponse
// @Failure		401	{object}	response.ErrorResponse
// @Failure		500	{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/auth/logout-all [post]
func (h *AuthHandler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		response.WriteError(w, http.StatusUnauthorized, "Authentication required", nil)
		return
	}

	if err := h.logoutAll.Execute(userID); err != nil {
		response.WriteError(w, http.StatusInternalServerError, "Logout failed", nil)
		return
	}

	h.clearAuthCookies(w)

	response.WriteSuccess(w, http.StatusOK, MessageResponse{
		Message: "All sessions have been revoked",
	})
}

// @Summary		Listar sessões ativas
// @Description	Retorna todas as sessões de autenticação ativas do usuário atual, destacando a que fez a requisição.
// @Tags			Autenticação
// @Produce		json
// @Success		200	{array}		SessionResponse
// @Failure		401	{object}	response.ErrorResponse
// @Failure		500	{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/auth/sessions [get]
func (h *AuthHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		response.WriteError(w, http.StatusUnauthorized, "Authentication required", nil)
		return
	}

	sessions, err := h.listSessions.Execute(userID)
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "Failed to list sessions", nil)
		return
	}

	var currentJTI string
	if claims, ok := r.Context().Value(middleware.ClaimsContextKey).(*authdomain.Claims); ok && claims != nil {
		currentJTI = claims.JTI
	}

	result := make([]SessionResponse, len(sessions))
	for i, s := range sessions {
		result[i] = SessionResponse{
			ID:         s.ID,
			DeviceInfo: s.DeviceInfo,
			IPAddress:  s.IPAddress,
			Location:   s.Location,
			IsCurrent:  currentJTI != "" && s.AccessJTI == currentJTI,
			CreatedAt:  s.CreatedAt,
			ExpiresAt:  s.ExpiresAt,
		}
	}

	response.WriteSuccess(w, http.StatusOK, result)
}

// @Summary		Revogar uma sessão específica
// @Description	Revoga uma única sessão do usuário autenticado pelo id.
// @Tags			Autenticação
// @Produce		json
// @Param			sessionId	path		string	true	"ID da sessão a revogar"
// @Success		200			{object}	MessageResponse
// @Failure		400			{object}	response.ErrorResponse
// @Failure		401			{object}	response.ErrorResponse
// @Failure		404			{object}	response.ErrorResponse
// @Failure		500			{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/auth/sessions/{sessionId} [delete]
func (h *AuthHandler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		response.WriteError(w, http.StatusUnauthorized, "Authentication required", nil)
		return
	}

	sessionID := mux.Vars(r)["sessionId"]
	if sessionID == "" {
		response.WriteValidationError(w, map[string]string{"sessionId": "required in URL path"})
		return
	}

	err := h.revokeSession.Execute(userID, sessionID)
	if err != nil {
		if errors.Is(err, authdomain.ErrSessionNotFound) {
			response.WriteError(w, http.StatusNotFound, "Session not found", nil)
			return
		}
		response.WriteError(w, http.StatusInternalServerError, "Failed to revoke session", nil)
		return
	}

	response.WriteSuccess(w, http.StatusOK, MessageResponse{
		Message: "Session revoked successfully",
	})
}
