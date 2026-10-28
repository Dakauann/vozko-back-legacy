package cloudflare

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestKVClientTalksToTheNamespaceValuesAPI(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		seen = append(seen, r.Method+" "+r.URL.RequestURI()+" "+string(body))
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/values/host:missing"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/values/host:boom"):
			w.WriteHeader(http.StatusInternalServerError)
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"v":1}`))
		default:
			_, _ = w.Write([]byte(`{"success":true}`))
		}
	}))
	defer server.Close()
	kv := NewKVClient("acc", "ns", "token").WithBaseURL(server.URL)
	ctx := context.Background()

	if err := kv.Put(ctx, "host:a.example", []byte(`{"v":1}`), 0); err != nil {
		t.Fatal(err)
	}
	if err := kv.Put(ctx, "replicas:r1", []byte(`{}`), 60); err != nil {
		t.Fatal(err)
	}
	value, found, err := kv.Get(ctx, "host:a.example")
	if err != nil || !found || string(value) != `{"v":1}` {
		t.Fatalf("got %q %v %v", value, found, err)
	}
	if _, found, err := kv.Get(ctx, "host:missing"); err != nil || found {
		t.Fatalf("a missing key is not an error, got %v %v", found, err)
	}
	if _, _, err := kv.Get(ctx, "host:boom"); err == nil {
		t.Fatal("a server error must surface")
	}
	if err := kv.Delete(ctx, "host:a.example"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"PUT /accounts/acc/storage/kv/namespaces/ns/values/host%3Aa.example {\"v\":1}",
		"PUT /accounts/acc/storage/kv/namespaces/ns/values/replicas:r1?expiration_ttl=60 {}",
	}
	for i, w := range want {
		if strings.ReplaceAll(seen[i], "%3A", ":") != strings.ReplaceAll(w, "%3A", ":") {
			t.Fatalf("call %d = %q, want %q", i, seen[i], w)
		}
	}
	if !strings.HasPrefix(seen[len(seen)-1], "DELETE ") {
		t.Fatalf("got %v", seen)
	}
}

func TestKVClientRefusesFailedWrites(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	kv := NewKVClient("acc", "ns", "token").WithBaseURL(server.URL)
	if err := kv.Put(context.Background(), "host:a", []byte("x"), 0); err == nil {
		t.Fatal("a refused write must error")
	}
	if err := kv.Delete(context.Background(), "host:a"); err == nil {
		t.Fatal("a refused delete must error")
	}
}
