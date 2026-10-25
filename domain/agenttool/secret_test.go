package agenttool

import (
	"strings"
	"testing"
)

func TestCredentialHeadersAreRecognisedByName(t *testing.T) {
	cases := map[string]bool{
		"Authorization":             true,
		"X-API-Key":                 true,
		"x-auth-token":              true,
		"Ocp-Apim-Subscription-Key": true,
		"Cookie":                    true,
		"X-Signature":               true,
		"Proxy-Authorization":       true,
		"Content-Type":              false,
		"Accept":                    false,
		"X-Request-Source":          false,
		"User-Agent":                false,
	}
	for name, want := range cases {
		if got := IsCredentialName(name); got != want {
			t.Errorf("%s: got %v want %v", name, got, want)
		}
	}
}

func TestAValueIsSealedOnlyWhenEveryCredentialPartIsAReference(t *testing.T) {
	cases := map[string]bool{
		"":                               true,
		"{{secret.api_key}}":             true,
		"Bearer {{secret.api_key}}":      true,
		"Basic {{ secret.basic }}":       true,
		"Bearer abc123":                  false,
		"abc {{secret.api_key}}":         false,
		"plain-token":                    false,
		"{{var.client}}":                 false,
		"Token {{secret.a}}{{secret.b}}": true,
	}
	for value, want := range cases {
		if got := IsSealed(value); got != want {
			t.Errorf("%q: got %v want %v", value, got, want)
		}
	}
}

func TestGeneratedSecretNamesAreValidStableAndShort(t *testing.T) {
	name := SecretNameFor("3F2A9C11-aaaa-bbbb-cccc-000000000000", "consultar_pedido", "X-API-Key")
	if name != "agent_3f2a9c11_consultar_pedido_x_api_key" {
		t.Fatalf("name %q", name)
	}
	long := SecretNameFor("3f2a9c11-0000", strings.Repeat("ferramenta_", 10), "Authorization")
	if len(long) > 64 || !strings.HasPrefix(long, "agent_3f2a9c11_") {
		t.Fatalf("long %q (%d)", long, len(long))
	}
	if odd := SecretNameFor("", "Ação Pedido!", "Token"); odd != "agent_acao_pedido_token" {
		t.Fatalf("odd %q", odd)
	}
}

func TestReferenceWrapsTheNameInTheTemplateSyntax(t *testing.T) {
	if got := Reference("api_key"); got != "{{secret.api_key}}" {
		t.Fatalf("got %q", got)
	}
}

func TestWithSuffixKeepsTheNameWithinTheLimit(t *testing.T) {
	base := strings.Repeat("a", 64)
	got := WithSuffix(base, 12)
	if len(got) != 64 || !strings.HasSuffix(got, "_12") {
		t.Fatalf("got %q", got)
	}
	if got := WithSuffix("agent_x", 2); got != "agent_x_2" {
		t.Fatalf("got %q", got)
	}
}
