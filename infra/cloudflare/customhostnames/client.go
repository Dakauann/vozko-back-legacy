package customhostnames

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"vozko/domain/sitepublish"
)

const (
	defaultBaseURL = "https://api.cloudflare.com/client/v4"
	maxResponse    = 1 << 20
)

var ErrNotFound = fmt.Errorf("customhostnames: the custom hostname does not exist: %w", sitepublish.ErrHostnameNotFound)

var hostnamePhases = map[string]sitepublish.HostnameStatus{
	"pending":             sitepublish.HostnamePendingDNS,
	"pending_provisioned": sitepublish.HostnamePendingDNS,
	"pending_migration":   sitepublish.HostnamePendingDNS,
	"test_pending":        sitepublish.HostnamePendingDNS,
	"provisioned":         sitepublish.HostnamePendingSSL,
	"active":              sitepublish.HostnamePendingSSL,
	"active_redeploying":  sitepublish.HostnamePendingSSL,
	"test_active":         sitepublish.HostnamePendingSSL,
	"test_active_apex":    sitepublish.HostnamePendingSSL,
	"moved":               sitepublish.HostnameFailed,
	"deleted":             sitepublish.HostnameFailed,
	"pending_deletion":    sitepublish.HostnameFailed,
	"blocked":             sitepublish.HostnameFailed,
	"pending_blocked":     sitepublish.HostnameFailed,
	"test_blocked":        sitepublish.HostnameFailed,
	"test_failed":         sitepublish.HostnameFailed,
}

var failedSSL = map[string]bool{
	"initializing_timed_out": true, "validation_timed_out": true, "issuance_timed_out": true,
	"deployment_timed_out": true, "expired": true, "deleted": true, "inactive": true,
}

type Client struct {
	zoneID     string
	apiToken   string
	baseURL    string
	httpClient *http.Client
}

func New(zoneID, apiToken string) *Client {
	return &Client{zoneID: zoneID, apiToken: apiToken, baseURL: defaultBaseURL, httpClient: &http.Client{Timeout: 10 * time.Second}}
}

func (c *Client) WithBaseURL(base string) *Client {
	clone := *c
	clone.baseURL = strings.TrimRight(base, "/")
	return &clone
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type envelope struct {
	Success bool            `json:"success"`
	Errors  []apiError      `json:"errors"`
	Result  json.RawMessage `json:"result"`
}

type validationRecord struct {
	TXTName  string `json:"txt_name"`
	TXTValue string `json:"txt_value"`
}

type message struct {
	Message string `json:"message"`
}

type result struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
	Status   string `json:"status"`
	SSL      struct {
		Status            string             `json:"status"`
		ValidationRecords []validationRecord `json:"validation_records"`
		ValidationErrors  []message          `json:"validation_errors"`
	} `json:"ssl"`
	OwnershipVerification struct {
		Type  string `json:"type"`
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"ownership_verification"`
	VerificationErrors []string `json:"verification_errors"`
}

type createRequest struct {
	Hostname string     `json:"hostname"`
	SSL      sslRequest `json:"ssl"`
}

type sslRequest struct {
	Method   string      `json:"method"`
	Type     string      `json:"type"`
	Settings sslSettings `json:"settings"`
}

type sslSettings struct {
	MinTLSVersion string `json:"min_tls_version"`
}

func (c *Client) endpoint(id string) string {
	base := fmt.Sprintf("%s/zones/%s/custom_hostnames", c.baseURL, url.PathEscape(c.zoneID))
	if id == "" {
		return base
	}
	return base + "/" + url.PathEscape(id)
}

func (c *Client) call(ctx context.Context, method, target string, body any) (int, envelope, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, envelope{}, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return 0, envelope{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, envelope{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return resp.StatusCode, envelope{}, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return resp.StatusCode, envelope{}, ErrNotFound
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return resp.StatusCode, envelope{}, fmt.Errorf("customhostnames: unreadable answer (status %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode >= 300 || !env.Success {
		return resp.StatusCode, env, fmt.Errorf("customhostnames: %s refused with status %d: %s", method, resp.StatusCode, describe(env.Errors))
	}
	return resp.StatusCode, env, nil
}

func describe(errs []apiError) string {
	parts := make([]string, 0, len(errs))
	for _, e := range errs {
		parts = append(parts, fmt.Sprintf("%d %s", e.Code, e.Message))
	}
	return strings.Join(parts, "; ")
}

func decodeResult(env envelope) (result, error) {
	var r result
	if err := json.Unmarshal(env.Result, &r); err != nil {
		return result{}, fmt.Errorf("customhostnames: unreadable result: %w", err)
	}
	if strings.TrimSpace(r.ID) == "" || strings.TrimSpace(r.Hostname) == "" {
		return result{}, errors.New("customhostnames: the result has no id or hostname")
	}
	return r, nil
}

func state(r result) (sitepublish.CustomHostnameState, error) {
	phase, known := hostnamePhases[r.Status]
	if !known {
		return sitepublish.CustomHostnameState{}, fmt.Errorf("customhostnames: unknown hostname status %q", r.Status)
	}
	s := sitepublish.CustomHostnameState{Ref: r.ID, Hostname: strings.ToLower(r.Hostname), SSLStatus: r.SSL.Status, Errors: append([]string{}, r.VerificationErrors...)}
	for _, e := range r.SSL.ValidationErrors {
		s.Errors = append(s.Errors, e.Message)
	}
	if phase == sitepublish.HostnamePendingSSL && r.SSL.Status == "active" {
		phase = sitepublish.HostnameActive
	}
	s.Phase = phase
	s.SSLFailing = phase == sitepublish.HostnamePendingSSL && (failedSSL[r.SSL.Status] || len(r.SSL.ValidationErrors) > 0)
	if v := r.OwnershipVerification; v.Name != "" && v.Value != "" {
		s.Records = append(s.Records, sitepublish.DNSRecord{Type: strings.ToUpper(v.Type), Name: v.Name, Value: v.Value})
	}
	for _, rec := range r.SSL.ValidationRecords {
		if rec.TXTName != "" && rec.TXTValue != "" {
			s.Records = append(s.Records, sitepublish.DNSRecord{Type: "TXT", Name: rec.TXTName, Value: rec.TXTValue})
		}
	}
	return s, nil
}

func (c *Client) Create(ctx context.Context, hostname string) (sitepublish.CustomHostnameState, error) {
	body := createRequest{Hostname: hostname, SSL: sslRequest{Method: "txt", Type: "dv", Settings: sslSettings{MinTLSVersion: "1.2"}}}
	_, env, err := c.call(ctx, http.MethodPost, c.endpoint(""), body)
	if err != nil {
		return sitepublish.CustomHostnameState{}, err
	}
	r, err := decodeResult(env)
	if err != nil {
		return sitepublish.CustomHostnameState{}, err
	}
	if !strings.EqualFold(r.Hostname, hostname) {
		return sitepublish.CustomHostnameState{}, fmt.Errorf("customhostnames: asked for %s and got %s", hostname, r.Hostname)
	}
	return state(r)
}

func (c *Client) Get(ctx context.Context, ref string) (sitepublish.CustomHostnameState, error) {
	_, env, err := c.call(ctx, http.MethodGet, c.endpoint(ref), nil)
	if err != nil {
		return sitepublish.CustomHostnameState{}, err
	}
	r, err := decodeResult(env)
	if err != nil {
		return sitepublish.CustomHostnameState{}, err
	}
	if r.ID != ref {
		return sitepublish.CustomHostnameState{}, fmt.Errorf("customhostnames: asked for %s and got %s", ref, r.ID)
	}
	return state(r)
}

func (c *Client) Delete(ctx context.Context, ref string) error {
	_, _, err := c.call(ctx, http.MethodDelete, c.endpoint(ref), nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}
