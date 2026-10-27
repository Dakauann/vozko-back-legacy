package googlesheet

import (
	"testing"

	"vozko/domain/googlesheet"
)

func TestASheetResponseSaysWhetherItTakesMessageExports(t *testing.T) {
	timeline := &googlesheet.Sheet{Columns: []googlesheet.Column{{Key: "mensagem", Source: googlesheet.SourceMessageText}}}
	plain := &googlesheet.Sheet{Columns: []googlesheet.Column{{Key: "nome", Source: googlesheet.SourceInput}}}
	if !toSheetResponse(timeline).TakesMessages || toSheetResponse(plain).TakesMessages {
		t.Fatal("the editor warns from this flag, so it must follow the sheet's message columns")
	}
}
