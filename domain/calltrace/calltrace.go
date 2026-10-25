package calltrace

import (
	"errors"

	"vozko/domain/redact"
	"vozko/domain/tracepayload"
)

type ErrorKind string

const (
	ErrorKindConfig     ErrorKind = "config"
	ErrorKindArguments  ErrorKind = "arguments"
	ErrorKindRender     ErrorKind = "render"
	ErrorKindSecret     ErrorKind = "secret"
	ErrorKindTransport  ErrorKind = "transport"
	ErrorKindTimeout    ErrorKind = "timeout"
	ErrorKindHTTPStatus ErrorKind = "http_status"
	ErrorKindMapping    ErrorKind = "mapping"
	ErrorKindInternal   ErrorKind = "internal"
	ErrorKindCancelled  ErrorKind = "cancelled"
)

type HTTPExchange struct {
	Method          string            `json:"method"`
	URL             string            `json:"url"`
	RequestHeaders  map[string]string `json:"requestHeaders,omitempty"`
	RequestBody     string            `json:"requestBody,omitempty"`
	Status          int               `json:"status,omitempty"`
	ResponseHeaders map[string]string `json:"responseHeaders,omitempty"`
	ResponseBody    string            `json:"responseBody,omitempty"`
	LatencyMs       int64             `json:"latencyMs"`
	TLSFallback     bool              `json:"tlsFallback,omitempty"`
	Redirects       int               `json:"redirects,omitempty"`
	EgressRefusal   string            `json:"egressRefusal,omitempty"`
	Error           string            `json:"error,omitempty"`
	Truncated       bool              `json:"truncated,omitempty"`
	Attempt         int               `json:"attempt,omitempty"`
}

func (x HTTPExchange) Redact(r redact.Redactor) HTTPExchange {
	x.URL = r.String(x.URL)
	x.RequestHeaders = redactHeaders(r, x.RequestHeaders)
	x.RequestBody = r.String(x.RequestBody)
	x.ResponseHeaders = redactHeaders(r, x.ResponseHeaders)
	x.ResponseBody = r.String(x.ResponseBody)
	x.EgressRefusal = r.String(x.EgressRefusal)
	x.Error = r.String(x.Error)
	return x
}

func redactHeaders(r redact.Redactor, headers map[string]string) map[string]string {
	if headers == nil {
		return nil
	}
	masked, _ := r.Value(headers).(map[string]string)
	return masked
}

const minExchangeBodyBytes = 1024

func FitHTTPExchanges(exchanges []HTTPExchange) []HTTPExchange {
	if len(exchanges) == 0 {
		return nil
	}
	bodyBudget := max(tracepayload.MaxBytes/(4*len(exchanges)), minExchangeBodyBytes)
	out := make([]HTTPExchange, len(exchanges))
	for i, x := range exchanges {
		var cutRequest, cutResponse bool
		x.RequestBody, cutRequest = tracepayload.Text(x.RequestBody, bodyBudget)
		x.ResponseBody, cutResponse = tracepayload.Text(x.ResponseBody, bodyBudget)
		x.Truncated = x.Truncated || cutRequest || cutResponse
		out[i] = x
	}
	return out
}

const MaxErrorChain = 16

func ErrorChain(err error) []string {
	if err == nil {
		return nil
	}
	chain := make([]string, 0, 4)
	var walk func(error)
	walk = func(e error) {
		for e != nil && len(chain) < MaxErrorChain {
			if msg := e.Error(); len(chain) == 0 || chain[len(chain)-1] != msg {
				chain = append(chain, msg)
			}
			if joined, ok := e.(interface{ Unwrap() []error }); ok {
				for _, inner := range joined.Unwrap() {
					walk(inner)
				}
				return
			}
			e = errors.Unwrap(e)
		}
	}
	walk(err)
	return chain
}

type ToolTrace struct {
	Exchanges    []HTTPExchange `json:"exchanges,omitempty"`
	MappedOutput interface{}    `json:"mappedOutput,omitempty"`
	ErrorKind    ErrorKind      `json:"errorKind,omitempty"`
	ErrorChain   []string       `json:"errorChain,omitempty"`
}
