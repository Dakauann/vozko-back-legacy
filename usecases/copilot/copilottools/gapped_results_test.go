package copilottools

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAResultEndsWithTheTrailingField(t *testing.T) {
	raw, err := JSONEndingWith(map[string]interface{}{"b": 1, "a": "x"}, "open_gaps", []string{"alt"})
	if err != nil || string(raw) != `{"a":"x","b":1,"open_gaps":["alt"]}` {
		t.Fatalf("got %s %v", raw, err)
	}
	raw, err = JSONEndingWith(nil, "open_gaps", []string{})
	if err != nil || string(raw) != `{"open_gaps":[]}` {
		t.Fatalf("got %s %v", raw, err)
	}
	var back map[string]interface{}
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if _, err := JSONEndingWith(map[string]interface{}{"bad": make(chan int)}, "open_gaps", nil); err == nil {
		t.Fatal("an unencodable body refuses")
	}
}

func TestARepliesMentionsMatchWordStartsOrWholePhrases(t *testing.T) {
	cases := []struct {
		reply string
		keys  []string
		want  bool
	}{
		{"Falta o texto alternativo da foto.", []string{"alt"}, true},
		{"Falta só o link.", []string{"alt"}, false},
		{"O botão Agendar consulta está sem destino.", []string{"Agendar Consulta"}, true},
		{"Veja o alt_missing.", []string{"alt_missing"}, true},
		{"Pronto.", []string{"", "link"}, false},
	}
	for _, c := range cases {
		if got := ReplyMentions(c.reply, c.keys); got != c.want {
			t.Errorf("%q %v: got %v", c.reply, c.keys, got)
		}
	}
}

func TestTheClosingNoteListsWhatIsLeftAndWhatNeverToInvent(t *testing.T) {
	note := ClosingNote("o e-mail ainda tem lacunas que impedem salvar e enviar", []string{"cta: href_missing", "settings: subject_empty"}, "Nunca invente links.")
	want := "Antes de encerrar: o e-mail ainda tem lacunas que impedem salvar e enviar, e a sua resposta não fala delas:\n- cta: href_missing\n- settings: subject_empty\n" +
		"Escreva de novo a resposta final, completa: diga ao usuário o que falta e peça esses dados numa só pergunta, ou corrija o que depende só de você. Nunca invente links."
	if note != want {
		t.Fatalf("got %q", note)
	}
	if got := Bullets([]string{"a", "b"}); got != "- a\n- b\n" || !strings.HasSuffix(note, "Nunca invente links.") {
		t.Fatalf("got %q", got)
	}
}
