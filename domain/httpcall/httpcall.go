package httpcall

import (
	"net/http"
	"strings"
	"time"
)

const (
	DefaultTimeout = 15 * time.Second
	MaxTimeout     = 30 * time.Second
)

const (
	AuthNone   = ""
	AuthBearer = "bearer"
	AuthBasic  = "basic"
	AuthAPIKey = "api_key"

	DefaultAPIKeyHeader = "X-API-Key"
)

type Auth struct {
	Mode    string
	User    string
	Token   string
	KeyName string
}

func Timeout(seconds float64) time.Duration {
	if seconds <= 0 {
		return DefaultTimeout
	}
	return min(time.Duration(seconds*float64(time.Second)), MaxTimeout)
}

func (a Auth) Apply(req *http.Request) {
	switch a.Mode {
	case AuthBearer:
		token := strings.TrimSpace(a.Token)
		if len(token) >= 7 && strings.EqualFold(token[:7], "bearer ") {
			token = strings.TrimSpace(token[7:])
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	case AuthBasic:
		if a.User != "" || a.Token != "" {
			req.SetBasicAuth(a.User, a.Token)
		}
	case AuthAPIKey:
		name := a.KeyName
		if name == "" {
			name = DefaultAPIKeyHeader
		}
		if a.Token != "" {
			req.Header.Set(name, a.Token)
		}
	}
}

func ReadOnlyMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodGet, http.MethodHead:
		return true
	}
	return false
}

func IdempotentMethod(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete, http.MethodOptions:
		return true
	}
	return false
}
