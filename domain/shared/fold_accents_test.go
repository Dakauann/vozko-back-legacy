package shared

import "testing"

func TestFoldAccentsDropsMarksAndKeepsLetters(t *testing.T) {
	if got := FoldAccents("Matrícula ação Ç"); got != "Matricula acao C" {
		t.Fatalf("got %q", got)
	}
}
