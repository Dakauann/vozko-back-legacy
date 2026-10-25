package tools

import "strings"

func PublicEndpoint(raw string) string {
	endpoint := strings.TrimSpace(raw)
	if i := strings.IndexAny(endpoint, "?#"); i >= 0 {
		endpoint = endpoint[:i]
	}
	scheme, rest, hasScheme := strings.Cut(endpoint, "://")
	if !hasScheme {
		return endpoint
	}
	host, path, _ := strings.Cut(rest, "/")
	if at := strings.LastIndex(host, "@"); at >= 0 {
		host = host[at+1:]
	}
	if path != "" {
		path = "/" + path
	}
	return scheme + "://" + host + path
}
