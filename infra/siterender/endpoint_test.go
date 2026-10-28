package siterender

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	renderuc "vozko/usecases/siterender"
)

var endpointToken = strings.Repeat("k", 40)

func devtoolsServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server.URL
}

func TestTheDebuggerURLCarriesTheRelayToken(t *testing.T) {
	var seen []string
	endpoint := devtoolsServer(t, func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		if r.URL.Path != "/"+endpointToken+"/json/version" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"webSocketDebuggerUrl":"ws://10.9.9.9:9222/devtools/browser/0b1c-2d"}`))
	})
	client, err := New(Config{Endpoint: endpoint, Token: endpointToken})
	if err != nil {
		t.Fatal(err)
	}

	got, err := client.debuggerURL(context.Background())

	host := strings.TrimPrefix(endpoint, "http://")
	if err != nil || got != "ws://"+host+"/"+endpointToken+"/devtools/browser/0b1c-2d" {
		t.Fatalf("debugger url %q %v, seen %v", got, err, seen)
	}
}

func TestADebuggerURLFailureIsUnavailableAndNeverEchoesTheToken(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"refused":     func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "forbidden", http.StatusForbidden) },
		"not json":    func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("hello")) },
		"no debugger": func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"Browser":"x"}`)) },
		"not a browser": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"webSocketDebuggerUrl":"ws://h:1/devtools/page/1"}`))
		},
		"other scheme": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"webSocketDebuggerUrl":"http://h:1/devtools/browser/1"}`))
		},
	}
	for name, handler := range cases {
		client, err := New(Config{Endpoint: devtoolsServer(t, handler), Token: endpointToken})
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.debuggerURL(context.Background())
		if !errors.Is(err, renderuc.ErrUnavailable) || strings.Contains(err.Error(), endpointToken) {
			t.Fatalf("%s: err = %v", name, err)
		}
	}
	client, err := New(Config{Endpoint: "http://127.0.0.1:1", Token: endpointToken})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.debuggerURL(context.Background()); !errors.Is(err, renderuc.ErrUnavailable) || strings.Contains(err.Error(), endpointToken) {
		t.Fatalf("unreachable: err = %v", err)
	}
}

func TestNewRefusesAClientWithoutEndpointOrToken(t *testing.T) {
	for name, config := range map[string]Config{
		"no token":      {Endpoint: "http://siterender-gateway:9222"},
		"no endpoint":   {Token: endpointToken},
		"relative":      {Endpoint: "siterender-gateway:9222", Token: endpointToken},
		"unsafe token":  {Endpoint: "http://siterender-gateway:9222", Token: endpointToken + "/x"},
		"websocket url": {Endpoint: "ws://siterender-gateway:9222/devtools/browser/1", Token: endpointToken},
	} {
		if _, err := New(config); err == nil {
			t.Fatalf("%s: New accepted %+v", name, config)
		}
	}
}
