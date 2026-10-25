package tools

import (
	"context"

	"testing"
	"vozko/domain/calltrace"
)

func TestCallIDTravelsWithTheContext(t *testing.T) {
	if got := CallIDFrom(context.Background()); got != "" {
		t.Fatalf("empty context = %q", got)
	}
	ctx := WithCallID(context.Background(), " call_9 ")
	if got := CallIDFrom(ctx); got != "call_9" {
		t.Fatalf("call id = %q", got)
	}
	if got := CallIDFrom(WithCallID(ctx, "")); got != "call_9" {
		t.Fatalf("an empty id must not erase the known one: %q", got)
	}
}

func TestTheOfferedDefinitionGivesTheHandlerAndInstance(t *testing.T) {
	defs := []Definition{{Name: "consultar_pedido", HandlerName: "http_request", Instance: &InstanceRef{ID: "t1", Type: "http_request", Revision: 2}}, {Name: "finish_conversation"}}
	def, ok := Offered(defs, "CONSULTAR_PEDIDO")
	if !ok || def.Handler() != "http_request" {
		t.Fatalf("def %+v ok %v", def, ok)
	}
	if plain, _ := Offered(defs, "finish_conversation"); plain.Handler() != "finish_conversation" {
		t.Fatalf("plain %+v", plain)
	}
	ctx := WithInstance(context.Background(), def.Instance)
	if ref, ok := InstanceFrom(ctx); !ok || ref.ID != "t1" || ref.Revision != 2 {
		t.Fatalf("ref %+v", ref)
	}
	if _, ok := InstanceFrom(WithInstance(context.Background(), nil)); ok {
		t.Fatal("no instance without a ref")
	}
}

func TestTheModelPayloadCarriesResultErrorAndContextOnly(t *testing.T) {
	res := &ExecutionResult{Result: map[string]interface{}{"status": "ok"}, IsError: true, ContextUpdateText: " nota ", RunID: "r1", Trace: &calltrace.ToolTrace{ErrorKind: "x"}}
	if got := res.ModelPayload(); got != `{"context_update":"nota","is_error":true,"result":{"status":"ok"}}` {
		t.Fatalf("payload %s", got)
	}
	var nothing *ExecutionResult
	if nothing.ModelPayload() != "{}" {
		t.Fatal("nil result")
	}
}
