package middleware

import (
	"net/http"
	"strings"
)

type CORSMiddleware struct {
	trustedOrigins map[string]struct{}
	delegated      []string
}

func NewCORSMiddleware(trustedOrigins []string) *CORSMiddleware {
	m := &CORSMiddleware{
		trustedOrigins: make(map[string]struct{}, len(trustedOrigins)),
	}
	for _, o := range trustedOrigins {
		m.trustedOrigins[strings.TrimRight(o, "/")] = struct{}{}
	}
	return m
}

const (
	corsAllowHeaders = "Authorization, Content-Type, X-Workspace-ID, X-Department-ID, X-Auth-Mode, Idempotency-Key, If-Match"
	corsAllowMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
	corsMaxAge       = "86400"
)

func (c *CORSMiddleware) Delegating(prefixes ...string) *CORSMiddleware {
	c.delegated = append(c.delegated, prefixes...)
	return c
}

func (c *CORSMiddleware) delegates(path string) bool {
	for _, prefix := range c.delegated {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func (c *CORSMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c.delegates(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")

		if _, trusted := c.trustedOrigins[strings.TrimRight(origin, "/")]; origin != "" && trusted {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", corsAllowMethods)
			w.Header().Set("Access-Control-Allow-Headers", corsAllowHeaders)
			w.Header().Set("Access-Control-Max-Age", corsMaxAge)
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
