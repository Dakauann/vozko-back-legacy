package auth

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	authdomain "vozko/domain/auth"
)

const maxResetPasswordBodyBytes = 16 << 10

func readResetPasswordRequest(body io.Reader) (ResetPasswordRequest, passwordFlowRefusal, bool) {
	var req ResetPasswordRequest
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		return req, refusalPasswordInvalidBody, false
	}
	if refusal, ok := req.Validate(); !ok {
		return req, refusal, false
	}
	return req, passwordFlowRefusal{}, true
}

func RefuseResetsBreakingThePasswordRules(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxResetPasswordBodyBytes))
		if err != nil {
			writePasswordFlowRefusal(w, refusalPasswordInvalidBody, nil)
			return
		}
		req, refusal, ok := readResetPasswordRequest(bytes.NewReader(raw))
		if !ok {
			writePasswordFlowRefusal(w, refusal, nil)
			return
		}
		if err := authdomain.CurrentPasswordPolicy().Check(req.NewPassword); err != nil {
			writePasswordFlowError(w, err, refusalWeakPassword)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
		next.ServeHTTP(w, r)
	})
}
