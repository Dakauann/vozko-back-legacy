package sitepublishhttp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"vozko/domain/sitepublish"
	sitepublish_usecase "vozko/usecases/sitepublish"
)

func TestWriteErrorAnswersEveryFailure(t *testing.T) {
	cases := map[string]struct {
		err    error
		status int
		code   string
	}{
		"canceled":      {fmt.Errorf("wrapped: %w", context.Canceled), http.StatusRequestTimeout, "request_canceled"},
		"live scan":     {fmt.Errorf("x: %w", sitepublish_usecase.ErrLiveScanFailed), http.StatusBadGateway, "live_scan_failed"},
		"unrecorded":    {fmt.Errorf("x: %w", sitepublish_usecase.ErrPublishUnrecorded), http.StatusInternalServerError, "publish_unrecorded"},
		"media refused": {fmt.Errorf("x: %w", sitepublish_usecase.ErrMediaRefused), http.StatusUnprocessableEntity, "media_refused"},
		"media down":    {fmt.Errorf("x: %w", sitepublish_usecase.ErrMediaUnavailable), http.StatusServiceUnavailable, "media_unavailable"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeError(w, c.err)
			if w.Code != c.status || !strings.Contains(w.Body.String(), `"`+c.code+`"`) {
				t.Fatalf("got %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestOnlyTheActiveDeploymentHasAnImmutableAddress(t *testing.T) {
	for status, want := range map[sitepublish.DeploymentStatus]string{
		sitepublish.DeploymentActive:     "https://deploy--dep-1.paginas.test/",
		sitepublish.DeploymentSuperseded: "",
		sitepublish.DeploymentReady:      "",
		sitepublish.DeploymentFailed:     "",
	} {
		if got := deploymentDTO(sitepublish.DeploymentRecord{ID: "dep-1", Status: status}, sitepublish.Addresses{Domain: "paginas.test"}).Address; got != want {
			t.Errorf("%s: got %q", status, got)
		}
	}
}
