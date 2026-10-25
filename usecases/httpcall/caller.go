package httpcall_usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"vozko/domain/calltrace"
	"vozko/domain/httpcall"
	"vozko/domain/redact"
	"vozko/domain/shared"
	"vozko/domain/workspacesecret"
	"vozko/usecases/httpcall/httptrace"
)

var ErrEgressNotConfigured = errors.New("outbound HTTP is not configured on this server")

type Egress struct {
	Client    *http.Client
	LegacyTLS *http.Client
}

type Prepared struct {
	Method        string
	URL           string
	Body          string
	Headers       map[string]string
	Auth          httpcall.Auth
	Timeout       time.Duration
	ResponseLimit int
}

type Outcome struct {
	Exchange calltrace.HTTPExchange
	Status   int
	Body     []byte
	Header   http.Header
	Kind     calltrace.ErrorKind
	Err      error
}

func (o Outcome) Sent() bool {
	return o.Err == nil
}

type Caller struct {
	egress Egress
}

func NewCaller(egress Egress) *Caller {
	return &Caller{egress: egress}
}

func (c *Caller) Do(ctx context.Context, p Prepared) Outcome {
	method := strings.ToUpper(strings.TrimSpace(p.Method))
	if method == "" {
		method = http.MethodGet
	}
	if c == nil || c.egress.Client == nil {
		return Outcome{Kind: calltrace.ErrorKindConfig, Err: ErrEgressNotConfigured, Exchange: calltrace.HTTPExchange{Method: method, URL: p.URL, Error: ErrEgressNotConfigured.Error()}}
	}
	timeout := p.Timeout
	if timeout <= 0 || timeout > httpcall.MaxTimeout {
		timeout = httpcall.Timeout(timeout.Seconds())
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	makeReq := func() (*http.Request, error) {
		var body io.Reader
		if p.Body != "" {
			body = strings.NewReader(p.Body)
		}
		req, err := http.NewRequestWithContext(requestCtx, method, p.URL, body)
		if err != nil {
			return nil, err
		}
		if p.Body != "" && req.Header.Get("Content-Type") == "" {
			req.Header.Set("Content-Type", "application/json")
		}
		for k, v := range p.Headers {
			req.Header.Set(k, v)
		}
		p.Auth.Apply(req)
		return req, nil
	}

	req, err := makeReq()
	if err != nil {
		return Outcome{Kind: calltrace.ErrorKindConfig, Err: fmt.Errorf("failed to create request: %w", err), Exchange: calltrace.HTTPExchange{Method: method, URL: p.URL, Error: err.Error()}}
	}
	exchange := calltrace.HTTPExchange{
		Method:         method,
		URL:            req.URL.String(),
		RequestHeaders: httptrace.Headers(req.Header),
		RequestBody:    p.Body,
	}
	sentAt := time.Now()
	resp, err := c.egress.Client.Do(req)
	if err != nil && IsTLSHandshakeError(err) && c.egress.LegacyTLS != nil {
		if retry, mkErr := makeReq(); mkErr == nil {
			exchange.TLSFallback = true
			resp, err = c.egress.LegacyTLS.Do(retry)
		}
	}
	exchange.LatencyMs = time.Since(sentAt).Milliseconds()
	if err != nil {
		kind := calltrace.ErrorKindTransport
		var netErr net.Error
		if (errors.As(err, &netErr) && netErr.Timeout()) || errors.Is(err, context.DeadlineExceeded) {
			kind = calltrace.ErrorKindTimeout
		}
		exchange.Error = TransportCause(err)
		if errors.Is(err, shared.ErrEgressBlocked) {
			exchange.EgressRefusal = exchange.Error
		}
		return Outcome{Exchange: exchange, Kind: kind, Err: err}
	}
	defer resp.Body.Close()

	limit := p.ResponseLimit
	if limit <= 0 {
		limit = 64 * 1024
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, int64(limit)+1))
	if len(raw) > limit {
		raw = raw[:limit]
		exchange.Truncated = true
	}
	exchange.URL = resp.Request.URL.String()
	exchange.Status = resp.StatusCode
	exchange.ResponseHeaders = httptrace.Headers(resp.Header)
	exchange.ResponseBody = string(raw)
	exchange.Redirects = httptrace.Redirects(resp)
	return Outcome{Exchange: exchange, Status: resp.StatusCode, Body: raw, Header: resp.Header}
}

func IsTLSHandshakeError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "tls handshake timeout") ||
		strings.Contains(msg, "remote error: tls") ||
		strings.Contains(msg, "tls: handshake failure") ||
		strings.Contains(msg, "tls: protocol version")
}

func TransportCause(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		return urlErr.Err.Error()
	}
	return err.Error()
}

type Secrets struct {
	Values map[string]string
	Scrub  redact.Redactor
}

func ResolveSecrets(ctx context.Context, resolver workspacesecret.Resolver, workspaceID string, names []string) (Secrets, error) {
	secrets, err := RevealSecrets(ctx, resolver, workspaceID, names)
	if err != nil {
		return Secrets{}, err
	}
	if missing := workspacesecret.MissingNames(names, secrets.Values); len(missing) > 0 {
		return Secrets{}, &workspacesecret.MissingError{Names: missing}
	}
	return secrets, nil
}

func RevealSecrets(ctx context.Context, resolver workspacesecret.Resolver, workspaceID string, names []string) (Secrets, error) {
	if len(names) == 0 {
		return Secrets{Values: map[string]string{}, Scrub: redact.SecretsOnly()}, nil
	}
	if resolver == nil {
		return Secrets{}, fmt.Errorf("%w: %s", workspacesecret.ErrUnavailable, strings.Join(names, ", "))
	}
	if strings.TrimSpace(workspaceID) == "" {
		return Secrets{}, workspacesecret.ErrWorkspaceRequired
	}
	values, err := resolver.Resolve(ctx, workspaceID, names)
	if err != nil {
		return Secrets{}, err
	}
	revealed := make([]string, 0, len(values))
	for _, v := range values {
		revealed = append(revealed, v)
	}
	return Secrets{Values: values, Scrub: redact.SecretsOnly(revealed...)}, nil
}
