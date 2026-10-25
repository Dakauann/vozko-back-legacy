package agenttool

import (
	"reflect"
	"testing"
)

func TestSecretNamesListsEveryReferenceOnceInOrder(t *testing.T) {
	got := SecretNames("{{secret.b}} x {{ secret.a }}", "{{secret.b}}", "{{var.c}}")
	if !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("got %v", got)
	}
}

func TestRenderSecretsReplacesOnlySecretReferences(t *testing.T) {
	out := RenderSecrets("Bearer {{secret.t}} {{var.x}} {{sys.date}}", map[string]string{"t": "v&1"}, nil)
	if out != "Bearer v&1 {{var.x}} {{sys.date}}" {
		t.Fatalf("out %q", out)
	}
	escaped := RenderSecrets("{{secret.t}}", map[string]string{"t": "v&1"}, func(s string) string { return "<" + s + ">" })
	if escaped != "<v&1>" {
		t.Fatalf("escaped %q", escaped)
	}
}

func TestRenderURLEscapesSecretsByPosition(t *testing.T) {
	out := RenderURL("https://api.example.com/{{secret.p}}/x?key={{secret.k}}&page=1#frag", map[string]string{"p": "a b", "k": "k&1"})
	if out != "https://api.example.com/a%20b/x?key=k%261&page=1#frag" {
		t.Fatalf("out %q", out)
	}
}
