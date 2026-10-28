package googlesheet

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"vozko/domain/googlesheet"
	"vozko/domain/shared"
	gsuc "vozko/usecases/googlesheet"
)

func TestListedRowsCarryTheirOriginLinks(t *testing.T) {
	at := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	stub := &stubSheets{rows: []gsuc.RowView{
		{
			Row: &googlesheet.Row{ID: "row-1", Status: googlesheet.RowWritten, Cells: googlesheet.Cells{"valor": 10}, Origin: googlesheet.Origin{Kind: googlesheet.OriginWorkflow}, CreatedAt: at},
			Origin: gsuc.OriginView{
				Kind:         googlesheet.OriginWorkflow,
				Conversation: &shared.EntryRef{EntryID: "conv-1", EntryType: shared.EntryTypeWhatsApp},
				Workflow:     &gsuc.NamedRef{ID: "wf-1", Name: "Cobrança"},
				RunID:        "run-1",
			},
		},
		{
			Row:    &googlesheet.Row{ID: "row-2", Status: googlesheet.RowPending, Origin: googlesheet.Origin{Kind: googlesheet.OriginManual}, CreatedAt: at},
			Origin: gsuc.OriginView{Kind: googlesheet.OriginManual, Person: &gsuc.NamedRef{ID: "user-1", Name: "Ana"}},
		},
	}}

	rec := serve(NewHandler(stub, &stubRows{}), http.MethodGet, "/google-sheets/s1/rows?page=1", "", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Items []struct {
			ID           string         `json:"id"`
			Origin       string         `json:"origin"`
			OriginDetail map[string]any `json:"originDetail"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 2 {
		t.Fatalf("items = %+v", body.Items)
	}
	run, _ := json.Marshal(body.Items[0].OriginDetail)
	if body.Items[0].Origin != "workflow" || string(run) != `{"conversation":{"entryId":"conv-1","entryType":"whatsapp"},"kind":"workflow","runId":"run-1","workflow":{"id":"wf-1","name":"Cobrança"}}` {
		t.Fatalf("workflow row = %s", run)
	}
	manual, _ := json.Marshal(body.Items[1].OriginDetail)
	if string(manual) != `{"kind":"manual","person":{"id":"user-1","name":"Ana"}}` {
		t.Fatalf("manual row = %s", manual)
	}
}
