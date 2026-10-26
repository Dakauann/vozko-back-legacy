package unofficial_whatsapp

import (
	"fmt"
	"reflect"
	"testing"

	"vozko/domain/conversation"
)

func menuOptions(count int) []conversation.InteractiveOption {
	options := make([]conversation.InteractiveOption, 0, count)
	for i := 1; i <= count; i++ {
		options = append(options, conversation.InteractiveOption{ID: fmt.Sprintf("op%d", i), Title: fmt.Sprintf("Opção %d", i), Section: "Assuntos"})
	}
	return options
}

func TestAMenuShowsWhatTheLineCanRender(t *testing.T) {
	menu := ShowMenu(conversation.SendInteractiveRequest{
		Header: "JURIDICO SINDCOP", Body: "MENU INICIAL {{nome}}", Footer: "Atendimento",
		Options: menuOptions(MaxListOptions + 2), Style: InteractiveStyleButtons, ListButton: "Ver assuntos",
	})

	if menu.Style != InteractiveStyleList || len(menu.Options) != MaxListOptions {
		t.Fatalf("style %q with %d options, want a list cut at %d", menu.Style, len(menu.Options), MaxListOptions)
	}
	if menu.Request.Body != "JURIDICO SINDCOP\n\nMENU INICIAL {​{nome}}" || menu.Request.Header != "" {
		t.Fatalf("body %q header %q, want the header folded into a sanitized body", menu.Request.Body, menu.Request.Header)
	}
	content := menu.Content()
	if content.Kind != conversation.InteractiveKindList || content.ButtonLabel != "Ver assuntos" || content.Footer != "Atendimento" {
		t.Fatalf("content %+v, want the list as sent", content)
	}
	if rows := content.Sections[0].Rows; len(rows) != MaxListOptions || rows[0].ID != "op1" {
		t.Fatalf("rows %+v, want the %d shown options", rows, MaxListOptions)
	}
}

func TestTheStoredMenuIsTheOneTheLineShowed(t *testing.T) {
	req := conversation.SendInteractiveRequest{Body: "Deseja continuar?", Options: menuOptions(2), Style: InteractiveStyleButtons}

	menu := ShowMenu(req)
	want := menu.Request.ShowingAs(menu.Style, menu.Options)
	if got := ShownInteractive(req); !reflect.DeepEqual(got, want) {
		t.Fatalf("ShownInteractive = %+v, want %+v", got, want)
	}
	if got := ShownInteractive(req); got.Kind != conversation.InteractiveKindButtons || len(got.Buttons) != 2 {
		t.Fatalf("content %+v, want two reply buttons", got)
	}
}
