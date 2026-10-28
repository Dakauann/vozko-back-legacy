package sitepublish

import (
	"net/url"
	"slices"
	"strconv"
	"strings"
)

const (
	htmlMaxAge      = 60
	fileMaxAge      = 300
	immutableCache  = "public, max-age=31536000, immutable"
	personalCache   = "private, no-store"
	etagHexLength   = 32
	htmlContentType = "text/html"
)

type Resolution struct {
	File  string
	Route string
	Found bool
}

func (m DeploymentManifest) Resolve(path string) Resolution {
	if _, ok := m.Files[path]; ok {
		return Resolution{File: path, Found: true}
	}
	if file, ok := m.Routes[path]; ok {
		return Resolution{File: file, Route: path, Found: true}
	}
	return Resolution{File: m.NotFound}
}

func (m DeploymentManifest) VariantFile(route, key string) (string, string) {
	if file, ok := m.Variants[route][key]; ok && key != ControlVariant {
		return file, key
	}
	return m.Routes[route], ControlVariant
}

func (m DeploymentManifest) ContentSecurityPolicy() string {
	connect := []string{"'self'", m.APIOrigin}
	frame := m.Policy.Frame
	if len(frame) == 0 {
		frame = []string{"'none'"}
	}
	return strings.Join([]string{
		"default-src 'self'",
		"script-src " + joinSources([]string{"'self'"}, m.Policy.Script),
		"style-src 'self' 'unsafe-inline'",
		"img-src " + joinSources([]string{"'self'", "data:", "blob:"}, m.Policy.Img),
		"font-src 'self' data:",
		"media-src 'self' blob:",
		"connect-src " + joinSources(connect, m.Policy.Connect),
		"frame-src " + joinSources(nil, frame),
		"form-action 'self' " + m.APIOrigin,
		"frame-ancestors 'self'",
		"base-uri 'none'",
		"object-src 'none'",
	}, "; ")
}

func joinSources(base, extra []string) string {
	out := slices.Clone(base)
	for _, source := range extra {
		if !slices.Contains(out, source) {
			out = append(out, source)
		}
	}
	return strings.Join(out, " ")
}

func (e FileEntry) IsHTML() bool {
	return strings.HasPrefix(strings.ToLower(e.ContentType), htmlContentType)
}

func (e FileEntry) CacheControl(personal bool) string {
	switch {
	case e.IsHTML() && personal:
		return personalCache
	case e.IsHTML():
		return "public, max-age=" + strconv.Itoa(htmlMaxAge)
	case e.Immutable:
		return immutableCache
	}
	return "public, max-age=" + strconv.Itoa(fileMaxAge)
}

func (e FileEntry) ETag() string {
	return `"` + e.SHA256[:etagHexLength] + `"`
}

func (s ServingSettings) ExperimentFor(route string) (Experiment, bool) {
	for _, e := range s.Experiments {
		if e.Route == route {
			return e, true
		}
	}
	return Experiment{}, false
}

func (e Experiment) Pick(current string, roll func(total int) int) (string, bool) {
	total := 0
	for _, v := range e.Variants {
		if v.Key == current {
			return current, false
		}
		total += v.Weight
	}
	if total <= 0 {
		return ControlVariant, true
	}
	at := roll(total)
	for _, v := range e.Variants {
		if at < v.Weight {
			return v.Key, true
		}
		at -= v.Weight
	}
	return ControlVariant, true
}

func ServedPath(raw string) string {
	path, err := url.PathUnescape(raw)
	if err != nil {
		path = raw
	}
	if trimmed := strings.TrimRight(path, "/"); trimmed != "" {
		return trimmed
	}
	return "/"
}
