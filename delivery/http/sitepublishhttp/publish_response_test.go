package sitepublishhttp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"vozko/domain/sitepublish"
	sitepublish_usecase "vozko/usecases/sitepublish"
)

func TestPublishAnswersThePublishedRevisionAndWhetherEditsAreLeft(t *testing.T) {
	pub := &fakePublisher{}
	r, _ := router(NewHandler(pub, fakeAnalytics{}, sitepublish.Addresses{Domain: "paginas.test"}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, request(http.MethodPost, "/sites/projects/p1/deployments", ""))
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["revisionId"] != "rev-mark" || out["hasUnpublishedChanges"] != true || out["projectVersion"] != float64(14) {
		t.Fatalf("got %s", w.Body.String())
	}
}

func TestConcurrentAndStalePublishesAreConflicts(t *testing.T) {
	for code, err := range map[string]error{
		"publish_in_progress": sitepublish_usecase.ErrPublishInProgress,
		"publish_stale":       fmt.Errorf("x: %w", sitepublish_usecase.ErrPublishStale),
	} {
		w := httptest.NewRecorder()
		writeError(w, err)
		if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), `"`+code+`"`) {
			t.Fatalf("%s: got %d %s", code, w.Code, w.Body.String())
		}
	}
}
