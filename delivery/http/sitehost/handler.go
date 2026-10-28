package sitehost

import (
	"context"
	"errors"
	"html"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"vozko/infra/http/middleware"
	siteserve_usecase "vozko/usecases/siteserve"
)

const (
	maxPasswordForm = 8 << 10
	generatedPolicy = "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'"
	strictTransport = "max-age=31536000"
	referrerPolicy  = "strict-origin-when-cross-origin"
	permissions     = "camera=(), microphone=(), geolocation=(), payment=(), usb=(), interest-cohort=()"
)

type Pages interface {
	Claims(ctx context.Context, host string) (bool, error)
	Serve(ctx context.Context, req siteserve_usecase.Request) siteserve_usecase.Outcome
}

type Handler struct {
	pages Pages
}

func NewHandler(pages Pages) (*Handler, error) {
	if pages == nil {
		return nil, errors.New("sitehost: the page server is required")
	}
	return &Handler{pages: pages}, nil
}

func Dispatch(pages Pages, handler, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claimed, err := pages.Claims(r.Context(), r.Host)
		if err != nil {
			log.Printf("[sitehost] claim of %s: %v", r.Host, err)
			writeMessage(w, http.StatusServiceUnavailable, "Serviço indisponível")
			return
		}
		if claimed {
			handler.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	req := siteserve_usecase.Request{
		Host: r.Host, Method: r.Method, Path: r.URL.Path, RawQuery: r.URL.RawQuery,
		Cookie:    func(name string) string { return cookie(r, name) },
		UserAgent: r.UserAgent(), Referer: r.Referer(), ClientIP: middleware.GetClientIP(r),
	}
	if r.Method == http.MethodPost {
		switch r.URL.Path {
		case siteserve_usecase.BeaconPath:
			body, err := io.ReadAll(io.LimitReader(r.Body, siteserve_usecase.MaxBeaconBytes+1))
			if err != nil {
				writeMessage(w, http.StatusBadRequest, "Corpo inválido")
				return
			}
			req.Body = body
		case siteserve_usecase.PasswordPath:
			r.Body = http.MaxBytesReader(w, r.Body, maxPasswordForm)
			if err := r.ParseForm(); err == nil {
				req.Password, req.Next = r.PostForm.Get("password"), r.PostForm.Get("next")
			}
		}
	}
	write(w, r, h.pages.Serve(r.Context(), req))
}

func cookie(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return c.Value
}

func write(w http.ResponseWriter, r *http.Request, out siteserve_usecase.Outcome) {
	for _, c := range out.Cookies {
		http.SetCookie(w, &http.Cookie{Name: c.Name, Value: c.Value, Path: c.Path, MaxAge: int(c.MaxAge.Seconds()), HttpOnly: c.HTTPOnly, Secure: true, SameSite: http.SameSiteLaxMode})
	}
	switch out.Kind {
	case siteserve_usecase.KindContent:
		writeContent(w, r, out.Content)
	case siteserve_usecase.KindRedirect:
		w.Header().Set("Location", out.Location)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(out.Status)
	case siteserve_usecase.KindAccepted:
		w.WriteHeader(http.StatusNoContent)
	case siteserve_usecase.KindSuspended:
		writePage(w, http.StatusServiceUnavailable, "Página indisponível", "<h1>Página indisponível</h1><p>Esta página está indisponível no momento.</p>")
	case siteserve_usecase.KindPasswordPrompt:
		writePage(w, http.StatusUnauthorized, "Página protegida", passwordForm(out.Prompt))
	case siteserve_usecase.KindNotFound:
		writeMessage(w, http.StatusNotFound, "Página não encontrada")
	case siteserve_usecase.KindTooManyRequests:
		writeMessage(w, http.StatusTooManyRequests, "Muitas tentativas. Aguarde um pouco.")
	case siteserve_usecase.KindBadRequest:
		writeMessage(w, http.StatusBadRequest, "Evento inválido")
	case siteserve_usecase.KindPayloadTooLarge:
		writeMessage(w, http.StatusRequestEntityTooLarge, "Corpo grande demais")
	case siteserve_usecase.KindMethodNotAllowed:
		w.Header().Set("Allow", strings.Join(out.Allow, ", "))
		writeMessage(w, http.StatusMethodNotAllowed, "Método não permitido")
	default:
		writeMessage(w, http.StatusServiceUnavailable, "Serviço indisponível")
	}
}

func writeContent(w http.ResponseWriter, r *http.Request, c siteserve_usecase.Content) {
	h := w.Header()
	h.Set("Content-Type", c.ContentType)
	h.Set("Content-Length", strconv.Itoa(len(c.Body)))
	h.Set("Cache-Control", c.CacheControl)
	h.Set("Content-Security-Policy", c.Policy)
	h.Set("Strict-Transport-Security", strictTransport)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", referrerPolicy)
	h.Set("Permissions-Policy", permissions)
	h.Set("X-Frame-Options", "SAMEORIGIN")
	if c.ETag != "" {
		h.Set("ETag", c.ETag)
	}
	if c.NoIndex {
		h.Set("X-Robots-Tag", "noindex, nofollow")
	}
	status := http.StatusOK
	if c.Missing {
		status = http.StatusNotFound
	}
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(c.Body)
	}
}

func generatedHeaders(w http.ResponseWriter, contentType string) {
	h := w.Header()
	h.Set("Content-Type", contentType)
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Security-Policy", generatedPolicy)
	h.Set("X-Frame-Options", "DENY")
	h.Set("Strict-Transport-Security", strictTransport)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", referrerPolicy)
}

func writeMessage(w http.ResponseWriter, status int, text string) {
	generatedHeaders(w, "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, text)
}

func writePage(w http.ResponseWriter, status int, title, body string) {
	generatedHeaders(w, "text/html; charset=utf-8")
	w.Header().Set("X-Robots-Tag", "noindex")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, `<!doctype html><html lang="pt-BR"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex"><title>`+title+`</title><style>body{font-family:system-ui,sans-serif;display:grid;place-items:center;min-height:100vh;margin:0;padding:16px;color:#1f2937;background:#f9fafb}main{max-width:420px;text-align:center}input,button{font:inherit;padding:10px 12px;margin-top:8px;width:100%;box-sizing:border-box}</style></head><body><main>`+body+`</main></body></html>`)
}

func passwordForm(p siteserve_usecase.Prompt) string {
	notice := ""
	if p.Failed {
		notice = "<p>Senha incorreta. Tente de novo.</p>"
	}
	return `<h1>Página protegida</h1>` + notice + `<form method="post" action="` + siteserve_usecase.PasswordPath + `"><input type="hidden" name="next" value="` + html.EscapeString(p.Next) + `"><input type="password" name="password" autocomplete="current-password" placeholder="Senha" required><button type="submit">Entrar</button></form>`
}
