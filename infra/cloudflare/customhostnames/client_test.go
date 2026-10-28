package customhostnames

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"vozko/domain/sitepublish"
)

const pendingBody = `{"success":true,"errors":[],"messages":[],"result":{"id":"ch-1","hostname":"www.cliente.com.br","status":"pending",
"ssl":{"id":"ssl-1","status":"pending_validation","method":"txt","type":"dv","validation_records":[{"txt_name":"_acme-challenge.www.cliente.com.br","txt_value":"abc"}]},
"ownership_verification":{"type":"txt","name":"_cf-custom-hostname.www.cliente.com.br","value":"own-1"},"created_at":"2026-10-28T10:00:00Z"}}`

func serve(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, body []byte)) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		body, _ := io.ReadAll(r.Body)
		handler(w, r, body)
	}))
	t.Cleanup(server.Close)
	return New("zone-1", "token").WithBaseURL(server.URL)
}

func TestCreateAsksForTXTValidatedDVCertificates(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, r *http.Request, body []byte) {
		if r.Method != http.MethodPost || r.URL.Path != "/zones/zone-1/custom_hostnames" {
			t.Fatalf("got %s %s", r.Method, r.URL.Path)
		}
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		ssl := req["ssl"].(map[string]any)
		if req["hostname"] != "www.cliente.com.br" || ssl["method"] != "txt" || ssl["type"] != "dv" {
			t.Fatalf("got %s", body)
		}
		_, _ = w.Write([]byte(pendingBody))
	})
	state, err := client.Create(context.Background(), "www.cliente.com.br")
	if err != nil {
		t.Fatal(err)
	}
	if state.Ref != "ch-1" || state.Phase != sitepublish.HostnamePendingDNS || state.SSLStatus != "pending_validation" || len(state.Records) != 2 {
		t.Fatalf("got %+v", state)
	}
	if state.Records[0] != (sitepublish.DNSRecord{Type: "TXT", Name: "_cf-custom-hostname.www.cliente.com.br", Value: "own-1"}) {
		t.Fatalf("got %+v", state.Records)
	}
}

func TestCreateRefusesAnswersThatDoNotProveTheHostname(t *testing.T) {
	cases := map[string]string{
		"not success":    `{"success":false,"errors":[{"code":1406,"message":"Duplicate custom hostname found."}],"result":null}`,
		"other hostname": strings.Replace(pendingBody, `"hostname":"www.cliente.com.br"`, `"hostname":"evil.com"`, 1),
		"no id":          strings.Replace(pendingBody, `"id":"ch-1"`, `"id":""`, 1),
		"unknown status": strings.Replace(pendingBody, `"status":"pending",`, `"status":"weird",`, 1),
		"not json":       `<html>`,
	}
	for name, body := range cases {
		client := serve(t, func(w http.ResponseWriter, r *http.Request, _ []byte) { _, _ = w.Write([]byte(body)) })
		if _, err := client.Create(context.Background(), "www.cliente.com.br"); err == nil {
			t.Fatalf("%s: expected a refusal", name)
		}
	}
}

func TestStatusMapping(t *testing.T) {
	cases := []struct {
		status, ssl string
		errs        string
		phase       sitepublish.HostnameStatus
		failing     bool
	}{
		{"pending", "pending_validation", "", sitepublish.HostnamePendingDNS, false},
		{"active", "pending_validation", "", sitepublish.HostnamePendingSSL, false},
		{"active", "pending_deployment", "", sitepublish.HostnamePendingSSL, false},
		{"active", "validation_timed_out", "", sitepublish.HostnamePendingSSL, true},
		{"active", "pending_validation", `,"validation_errors":[{"message":"CAA record blocks"}]`, sitepublish.HostnamePendingSSL, true},
		{"active", "active", "", sitepublish.HostnameActive, false},
		{"moved", "active", "", sitepublish.HostnameFailed, false},
		{"blocked", "active", "", sitepublish.HostnameFailed, false},
	}
	for _, c := range cases {
		body := `{"success":true,"result":{"id":"ch-1","hostname":"www.cliente.com.br","status":"` + c.status + `","ssl":{"status":"` + c.ssl + `"` + c.errs + `}}}`
		client := serve(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
			if r.Method != http.MethodGet || r.URL.Path != "/zones/zone-1/custom_hostnames/ch-1" {
				t.Fatalf("got %s %s", r.Method, r.URL.Path)
			}
			_, _ = w.Write([]byte(body))
		})
		state, err := client.Get(context.Background(), "ch-1")
		if err != nil || state.Phase != c.phase || state.SSLFailing != c.failing {
			t.Fatalf("%s/%s: got %+v %v", c.status, c.ssl, state, err)
		}
	}
}

func TestGetOfAVanishedHostnameIsGone(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":1436,"message":"not found"}]}`))
	})
	if _, err := client.Get(context.Background(), "ch-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestDeleteTreatsAMissingHostnameAsDone(t *testing.T) {
	calls := 0
	client := serve(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		calls++
		if r.Method != http.MethodDelete {
			t.Fatalf("got %s", r.Method)
		}
		if calls == 1 {
			_, _ = w.Write([]byte(`{"success":true,"result":{"id":"ch-1"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	if err := client.Delete(context.Background(), "ch-1"); err != nil {
		t.Fatal(err)
	}
	if err := client.Delete(context.Background(), "ch-1"); err != nil {
		t.Fatalf("a missing hostname is already deleted, got %v", err)
	}
}

func TestDeleteSurfacesServerErrors(t *testing.T) {
	client := serve(t, func(w http.ResponseWriter, r *http.Request, _ []byte) { w.WriteHeader(http.StatusBadGateway) })
	if err := client.Delete(context.Background(), "ch-1"); err == nil {
		t.Fatal("expected an error")
	}
}
