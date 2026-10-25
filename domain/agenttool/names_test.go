package agenttool

import (
	"strings"
	"testing"
)

func TestValidNamesAreLowercaseSnakeCaseUpTo64WithoutTheMCPSeparator(t *testing.T) {
	cases := map[string]bool{
		"consultar_pedido":      true,
		"a":                     true,
		strings.Repeat("a", 64): true,
		strings.Repeat("a", 65): false,
		"":                      false,
		"1pedido":               false,
		"_pedido":               false,
		"Consultar":             false,
		"consultar-pedido":      false,
		"mcp__tool":             false,
		"ação":                  false,
	}
	for name, want := range cases {
		if got := ValidName(name); got != want {
			t.Errorf("%q: got %v want %v", name, got, want)
		}
	}
}

func TestProposeNameUsesTheFirstWordsOfTheDescription(t *testing.T) {
	none := func(string) bool { return false }
	if got := ProposeName("http_request", "Consultar status do pedido pelo número informado pelo cliente", none); got != "consultar_status_do_pedido_pelo" {
		t.Fatalf("got %q", got)
	}
	if got := ProposeName("http_request", "", none); got != "http_request" {
		t.Fatalf("got %q", got)
	}
	if got := ProposeName("http_request", "123 itens!", none); got != "tool_123_itens" {
		t.Fatalf("got %q", got)
	}
}

func TestProposeNameAddsASuffixUntilTheNameIsFree(t *testing.T) {
	taken := map[string]bool{"criar_ticket": true, "criar_ticket_2": true}
	if got := ProposeName("http_request", "Criar ticket", func(n string) bool { return taken[n] }); got != "criar_ticket_3" {
		t.Fatalf("got %q", got)
	}
}
