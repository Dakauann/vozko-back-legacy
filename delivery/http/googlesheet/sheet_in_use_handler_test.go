package googlesheet

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"

	"vozko/domain/googlesheet"
)

func TestDeletingATabARunningCampaignWritesToNamesTheCampaigns(t *testing.T) {
	h := NewHandler(&stubSheets{err: &googlesheet.InUseError{Campaigns: []string{"Cobrança Centro"}}}, &stubRows{})
	router := mux.NewRouter()
	router.HandleFunc("/google-sheets/{id}", h.Delete).Methods(http.MethodDelete)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/google-sheets/s1", nil))
	code, details := codeOf(t, rec)
	campaigns, _ := details["campaigns"].([]any)
	if rec.Code != http.StatusConflict || code != googlesheet.CodeSheetInUse || len(campaigns) != 1 || campaigns[0] != "Cobrança Centro" {
		t.Fatalf("status = %d code = %s details = %v", rec.Code, code, details)
	}
}
