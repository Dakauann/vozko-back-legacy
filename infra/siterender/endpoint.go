package siterender

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	renderuc "vozko/usecases/siterender"
)

const browserDebuggerPath = "/devtools/browser/"

func (c *Client) debuggerURL(ctx context.Context) (string, error) {
	unavailable := func(reason string) error {
		return fmt.Errorf("%w: the devtools endpoint %s", renderuc.ErrUnavailable, reason)
	}
	host, err := devtoolsHost(ctx, c.endpoint)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", unavailable("does not resolve")
	}
	discovery := url.URL{Scheme: c.endpoint.Scheme, Host: host, Path: "/" + c.config.Token + "/json/version"}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discovery.String(), nil)
	if err != nil {
		return "", unavailable("is not a url")
	}
	req.Close = true
	resp, err := c.discovery.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return "", unavailable("is unreachable: " + strings.ReplaceAll(err.Error(), c.config.Token, "[token]"))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", unavailable("replied " + strconv.Itoa(resp.StatusCode))
	}
	var version struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, discoveryReplyLimit)).Decode(&version); err != nil {
		return "", unavailable("replied without a version document")
	}
	ws, err := url.Parse(version.WebSocketDebuggerURL)
	if err != nil || (ws.Scheme != "ws" && ws.Scheme != "wss") || !strings.HasPrefix(ws.Path, browserDebuggerPath) {
		return "", unavailable("has no browser debugger")
	}
	scheme := "ws"
	if c.endpoint.Scheme == "https" {
		scheme = "wss"
	}
	return (&url.URL{Scheme: scheme, Host: host, Path: "/" + c.config.Token + ws.Path}).String(), nil
}

func devtoolsHost(ctx context.Context, endpoint *url.URL) (string, error) {
	host, port := endpoint.Hostname(), endpoint.Port()
	if port == "" {
		port = "80"
		if endpoint.Scheme == "https" {
			port = "443"
		}
	}
	if host == "localhost" || net.ParseIP(host) != nil {
		return net.JoinHostPort(host, port), nil
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return "", err
	}
	if len(addrs) == 0 {
		return "", errors.New("no address")
	}
	return net.JoinHostPort(addrs[0].IP.String(), port), nil
}
