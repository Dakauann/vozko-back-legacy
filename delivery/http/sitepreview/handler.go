package sitepreview

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"vozko/domain/site"
	"vozko/domain/sitepublish"
	"vozko/usecases/sitebuild"
)

type Server interface {
	Serve(ctx context.Context, host, path string) (sitebuild.Served, error)
}

type Handler struct {
	server Server
	csp    string
}

type Origins struct {
	Editors []string
	API     string
	Media   string
}

func NewHandler(server Server, origins Origins) (*Handler, error) {
	if server == nil {
		return nil, errors.New("sitepreview: the preview server is required")
	}
	api, ok := originOf(origins.API)
	if !ok {
		return nil, errors.New("sitepreview: the api origin must be an origin")
	}
	media, ok := originOf(origins.Media)
	if !ok {
		return nil, errors.New("sitepreview: the media origin must be an origin")
	}
	var ancestors []string
	for _, raw := range origins.Editors {
		origin, ok := originOf(raw)
		if !ok {
			return nil, errors.New("sitepreview: an editor origin is not a scheme and host")
		}
		ancestors = append(ancestors, origin)
	}
	if len(ancestors) == 0 {
		return nil, errors.New("sitepreview: the editor origins are required for frame-ancestors")
	}
	csp := strings.Join([]string{
		"default-src 'self'",
		"script-src 'self' " + api + site.WebchatLoaderPath,
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data: blob: https:",
		"font-src 'self' data: https:",
		"media-src 'self' blob: https:",
		"connect-src 'self' " + api + " " + media,
		"form-action 'self' " + api,
		"frame-src " + api + site.WebchatFramePath,
		"worker-src 'self' blob:",
		"object-src 'none'",
		"base-uri 'none'",
		"frame-ancestors " + strings.Join(ancestors, " "),
	}, "; ")
	return &Handler{server: server, csp: csp}, nil
}

func (h *Handler) headers(w http.ResponseWriter) {
	header := w.Header()
	header.Set("Content-Security-Policy", h.csp)
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("Cache-Control", "no-store")
	header.Set("Cross-Origin-Opener-Policy", "same-origin")
	header.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
}

func refuse(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(http.StatusText(status)))
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.headers(w)
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		refuse(w, http.StatusMethodNotAllowed)
		return
	}
	served, err := h.server.Serve(r.Context(), r.Host, r.URL.Path)
	switch {
	case errors.Is(err, sitebuild.ErrPreviewNotFound):
		refuse(w, http.StatusNotFound)
		return
	case err != nil:
		refuse(w, http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", served.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(served.Body)))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write(served.Body)
	}
}

func Dispatch(addresses sitepublish.Addresses, preview, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := addresses.PreviewToken(r.Host); ok {
			preview.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func originOf(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.User != nil || strings.ContainsAny(u.Host, " ;'\"") {
		return "", false
	}
	return strings.ToLower(u.Scheme + "://" + u.Host), true
}
