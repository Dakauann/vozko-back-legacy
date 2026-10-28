package site

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"vozko/domain/strictjson"
)

const (
	HomeRoute       = "/"
	maxNameRunes    = 120
	maxRoutePathLen = 200
)

var ErrManifestInvalid = errors.New("site: site.json is invalid")

var (
	routePath = regexp.MustCompile(`^/([a-z0-9]+(-[a-z0-9]+)*(/[a-z0-9]+(-[a-z0-9]+)*)*)?$`)
	langTag   = regexp.MustCompile(`^[a-z]{2}(-[A-Z]{2})?$`)
)

type Route struct {
	Path string `json:"path"`
	Page string `json:"page"`
}

type Manifest struct {
	Name   string  `json:"name"`
	Lang   string  `json:"lang,omitempty"`
	Routes []Route `json:"routes"`
}

func manifestError(reason string) error {
	return fmt.Errorf("%w: %s", ErrManifestInvalid, reason)
}

func DecodeManifest(stack Stack, raw []byte) (Manifest, error) {
	var m Manifest
	if err := strictjson.Decode(raw, &m); err != nil {
		return Manifest{}, fmt.Errorf("%w: %w", ErrManifestInvalid, err)
	}
	if err := m.validate(stack); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func ValidRoutePath(p string) bool {
	return len(p) <= maxRoutePathLen && routePath.MatchString(p)
}

func (m Manifest) validate(stack Stack) error {
	name := strings.TrimSpace(m.Name)
	if name == "" || name != m.Name || utf8.RuneCountInString(name) > maxNameRunes {
		return manifestError("name must be 1 to 120 characters without outer spaces")
	}
	if m.Lang != "" && !langTag.MatchString(m.Lang) {
		return manifestError("lang must look like pt-BR")
	}
	if len(m.Routes) == 0 {
		return manifestError("at least the home route is required")
	}
	seen := map[string]bool{}
	for _, route := range m.Routes {
		if !ValidRoutePath(route.Path) {
			return manifestError("route " + route.Path + " is not a lowercase path")
		}
		if seen[route.Path] {
			return manifestError("route " + route.Path + " repeats")
		}
		seen[route.Path] = true
		if !PageKind(stack, route.Page) {
			return manifestError("route " + route.Path + " does not point to a page file")
		}
	}
	if !seen[HomeRoute] {
		return manifestError("the home route / is required")
	}
	return nil
}

func (m Manifest) RoutePaths() []string {
	out := make([]string, 0, len(m.Routes))
	for _, route := range m.Routes {
		out = append(out, route.Path)
	}
	return out
}

func (m Manifest) Route(p string) (Route, bool) {
	for _, route := range m.Routes {
		if route.Path == p {
			return route, true
		}
	}
	return Route{}, false
}
