package crmtools

import (
	"context"
	"strings"
	"testing"

	"vozko/domain/copilot"
	"vozko/domain/googlesheet"
)

func TestEloIsRefusedWithTheRepeatCodeWhenTheRecordExists(t *testing.T) {
	sheets := eloSheet(t)
	sheets.repeat = &googlesheet.RepeatError{Code: googlesheet.CodeAlreadyRecordedColumnsMonth, RowID: "row-1", Headers: []string{"Matrícula"}}
	args := map[string]interface{}{"sheet_id": "sheet-1", "values": []interface{}{map[string]interface{}{"column": "nome", "value": "Ana"}}}
	res := NewAddGoogleSheetRowTool(sheets).Execute(context.Background(), eloContext, args)
	if res.Status != copilot.StatusError || !strings.HasPrefix(res.Message, googlesheet.CodeAlreadyRecordedColumnsMonth+":") || !strings.Contains(res.Message, "Matrícula") {
		t.Fatalf("result = %+v", res)
	}
}

func TestListGoogleSheetsShowsTheUniqueRule(t *testing.T) {
	sheets := eloSheet(t)
	if err := sheets.sheet.ChangeUnique(googlesheet.UniqueRule{Mode: googlesheet.UniqueColumns, Columns: []string{"nome"}}); err != nil {
		t.Fatal(err)
	}
	res := NewListGoogleSheetsTool(sheets).Execute(context.Background(), eloContext, map[string]interface{}{})
	listed := res.Data.(map[string]interface{})["sheets"].([]map[string]interface{})[0]
	unique, _ := listed["unique"].(map[string]interface{})
	if unique["mode"] != googlesheet.UniqueColumns || len(unique["columns"].([]string)) != 1 {
		t.Fatalf("unique = %+v", listed["unique"])
	}
}
