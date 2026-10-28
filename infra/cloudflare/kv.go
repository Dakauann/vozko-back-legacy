package cloudflare

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

const (
	cloudflareAPI = "https://api.cloudflare.com/client/v4"
	maxKVValue    = 25 << 20
)

type KVClient struct {
	accountID   string
	namespaceID string
	apiToken    string
	baseURL     string
	httpClient  *http.Client
}

func NewKVClient(accountID, namespaceID, apiToken string) *KVClient {
	return &KVClient{accountID: accountID, namespaceID: namespaceID, apiToken: apiToken, baseURL: cloudflareAPI, httpClient: &http.Client{Timeout: httpTimeout}}
}

func (c *KVClient) WithBaseURL(base string) *KVClient {
	clone := *c
	clone.baseURL = base
	return &clone
}

func (c *KVClient) valueURL(key string, ttlSeconds int) string {
	u := fmt.Sprintf("%s/accounts/%s/storage/kv/namespaces/%s/values/%s", c.baseURL, url.PathEscape(c.accountID), url.PathEscape(c.namespaceID), url.PathEscape(key))
	if ttlSeconds > 0 {
		u += "?expiration_ttl=" + strconv.Itoa(ttlSeconds)
	}
	return u
}

func (c *KVClient) do(ctx context.Context, method, target string, body []byte) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	return c.httpClient.Do(req)
}

func failure(op string, resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return fmt.Errorf("KV %s %d: %s", op, resp.StatusCode, string(raw))
}

func (c *KVClient) Put(ctx context.Context, key string, value []byte, ttlSeconds int) error {
	if value == nil {
		value = []byte{}
	}
	resp, err := c.do(ctx, http.MethodPut, c.valueURL(key, ttlSeconds), value)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return failure("PUT", resp)
	}
	return nil
}

func (c *KVClient) Get(ctx context.Context, key string) ([]byte, bool, error) {
	resp, err := c.do(ctx, http.MethodGet, c.valueURL(key, 0), nil)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, false, nil
	}
	if resp.StatusCode >= 300 {
		return nil, false, failure("GET", resp)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxKVValue))
	if err != nil {
		return nil, false, err
	}
	return raw, true, nil
}

func (c *KVClient) Delete(ctx context.Context, key string) error {
	resp, err := c.do(ctx, http.MethodDelete, c.valueURL(key, 0), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotFound {
		return failure("DELETE", resp)
	}
	return nil
}
