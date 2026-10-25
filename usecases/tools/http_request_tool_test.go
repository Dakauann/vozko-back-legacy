package tools_usecase

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"vozko/domain/calltrace"
	"vozko/domain/tools"
	httpcall_usecase "vozko/usecases/httpcall"
)

func clientEgress(c *http.Client) httpcall_usecase.Egress {
	return httpcall_usecase.Egress{Client: c}
}

func TestHTTPRequestToolDeclaresTheCanonicalConfigAndManyInstances(t *testing.T) {
	def := NewHTTPRequestToolUseCase(clientEgress(http.DefaultClient), nil).Definition()
	if !def.RequiresConfig || def.Instances != tools.Many() {
		t.Fatalf("definition %+v", def)
	}
	for _, key := range []string{"url", "method", "fields", "body_template", "auth", "output", "timeout_seconds", "idempotency_header"} {
		if _, ok := def.ConfigSchema[key]; !ok {
			t.Errorf("config key %s missing", key)
		}
	}
	if err := def.ValidateConfig(map[string]interface{}{"url": "https://a.test", "method": "GET", "fields": []interface{}{}, "output": map[string]interface{}{}}); err != nil {
		t.Fatalf("canonical config refused: %v", err)
	}
}

func TestTheModelSeesOneTypedArgumentPerModelField(t *testing.T) {
	tool := NewHTTPRequestToolUseCase(clientEgress(http.DefaultClient), nil).(tools.ContextualHandler)
	def := tool.DefinitionWithContext(tools.ToolContext{Config: map[string]interface{}{
		"url": "https://a.test/pedidos/{pedido}", "method": "GET",
		"fields": []interface{}{
			map[string]interface{}{"name": "pedido", "location": "path", "source": "model", "type": "string", "required": true},
			map[string]interface{}{"name": "Accept", "location": "header", "source": "fixed", "value": "json"},
		},
	}})
	if len(def.Parameters) != 1 || def.Parameters["pedido"].Type != "string" || len(def.Required) != 1 {
		t.Fatalf("parameters %+v required %v", def.Parameters, def.Required)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestHTTPRequestToolValuesFromTheModelCannotReshapeTheURL(t *testing.T) {
	var requested string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requested = r.URL.String()
		return &http.Response{StatusCode: 200, Body: http.NoBody, Header: http.Header{}, Request: r}, nil
	})}
	res, err := NewHTTPRequestToolUseCase(clientEgress(client), nil).ExecuteWithConfig(context.Background(), map[string]interface{}{
		"url": "https://api.example.com/leads/{lead_id}", "method": "GET",
		"fields": []interface{}{
			map[string]interface{}{"name": "lead_id", "location": "path", "source": "model"},
			map[string]interface{}{"name": "q", "location": "query", "source": "model"},
		},
	}, map[string]interface{}{"lead_id": "../admin?x=1", "q": "a&role=admin"})
	if err != nil || res.IsError {
		t.Fatalf("res %+v err %v", res, err)
	}
	if requested != "https://api.example.com/leads/..%2Fadmin%3Fx=1?q=a%26role%3Dadmin" {
		t.Fatalf("requested %q", requested)
	}
}

func TestHTTPRequestToolReportsAnEgressRefusalWithoutRetrying(t *testing.T) {
	calls := 0
	refused := errors.New("refused by the guard")
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, refused
	})}
	tool := NewHTTPRequestToolUseCase(clientEgress(client), nil).(*httpRequestTool)
	tool.sleep = func(context.Context, time.Duration) bool { return true }
	res, _ := tool.ExecuteWithConfig(context.Background(), map[string]interface{}{"url": "https://169.254.169.254/latest", "method": "GET"}, nil)
	if !res.IsError || res.Trace == nil || res.Trace.ErrorKind != calltrace.ErrorKindTransport {
		t.Fatalf("res %+v", res)
	}
	if calls != 1+maxIdempotentRetries {
		t.Fatalf("a transport error on GET is retried twice, calls %d", calls)
	}
}

type flaky struct {
	mu      sync.Mutex
	status  []int
	calls   int
	keys    []string
	methods []string
}

func (f *flaky) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.keys = append(f.keys, r.Header.Get("Idempotency-Key"))
	f.methods = append(f.methods, r.Method)
	status := f.status[min(f.calls, len(f.status)-1)]
	f.calls++
	if status == http.StatusTooManyRequests {
		w.Header().Set("Retry-After", "0")
	}
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func runAgainst(t *testing.T, srv *httptest.Server, method string) tools.ExecutionResult {
	t.Helper()
	tool := NewHTTPRequestToolUseCase(clientEgress(srv.Client()), nil).(*httpRequestTool)
	tool.sleep = func(context.Context, time.Duration) bool { return true }
	ctx := tools.WithInstance(tools.WithCallID(tools.WithRunID(context.Background(), "run-1"), "call-1"), &tools.InstanceRef{ID: "t1"})
	res, err := tool.ExecuteWithConfig(ctx, map[string]interface{}{"url": srv.URL + "/x", "method": method}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestIdempotentMethodsRetryOn503And429ButPostNeverDoes(t *testing.T) {
	for method, wantCalls := range map[string]int{"GET": 3, "PUT": 3, "DELETE": 3, "POST": 1, "PATCH": 1} {
		t.Run(method, func(t *testing.T) {
			f := &flaky{status: []int{503, 429, 200}}
			if method == "POST" || method == "PATCH" {
				f.status = []int{503}
			}
			srv := httptest.NewServer(f)
			defer srv.Close()
			res := runAgainst(t, srv, method)
			if f.calls != wantCalls {
				t.Fatalf("calls %d want %d (res %+v)", f.calls, wantCalls, res)
			}
			if wantCalls == 3 && (res.IsError || len(res.Trace.Exchanges) != 3 || res.Trace.Exchanges[2].Attempt != 3) {
				t.Fatalf("res %+v", res)
			}
		})
	}
}

func TestPostAndPatchCarryAStableIdempotencyKey(t *testing.T) {
	f := &flaky{status: []int{200}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	runAgainst(t, srv, "POST")
	runAgainst(t, srv, "POST")
	runAgainst(t, srv, "GET")
	if f.keys[0] == "" || f.keys[0] != f.keys[1] || f.keys[2] != "" {
		t.Fatalf("keys %q", f.keys)
	}
}

func TestANonSuccessStatusReachesTheModelAsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"erro":"pedido não encontrado"}`))
	}))
	defer srv.Close()
	res := runAgainst(t, srv, "POST")
	result, _ := json.Marshal(res.Result)
	if !res.IsError || res.Trace.ErrorKind != calltrace.ErrorKindHTTPStatus || !strings.Contains(string(result), "404") || !strings.Contains(string(result), "pedido não encontrado") {
		t.Fatalf("res %+v", res)
	}
}

func TestOnlyMappedOutputReachesTheModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"status":"enviado","cpf":"52998224725"}}`))
	}))
	defer srv.Close()
	res, err := NewHTTPRequestToolUseCase(clientEgress(srv.Client()), nil).ExecuteWithConfig(context.Background(), map[string]interface{}{
		"url": srv.URL, "method": "GET", "output": map[string]interface{}{"fields": []interface{}{map[string]interface{}{"name": "status", "path": "data.status"}}},
	}, nil)
	if err != nil || res.IsError {
		t.Fatalf("res %+v err %v", res, err)
	}
	out, _ := json.Marshal(res.Result)
	if string(out) != `{"status":"enviado"}` || res.Trace.MappedOutput == nil {
		t.Fatalf("result %s", out)
	}
}
