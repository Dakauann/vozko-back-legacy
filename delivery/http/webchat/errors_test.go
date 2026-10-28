package webchat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	wcdomain "vozko/domain/webchat"
)

func TestChallengeRefusalsKeepTheirMeaning(t *testing.T) {
	cases := map[error]struct {
		status int
		code   string
	}{
		wcdomain.ErrChallengeReused:                               {http.StatusBadRequest, "challenge_reused"},
		wcdomain.ErrChallengeUnavailable:                          {http.StatusServiceUnavailable, "challenge_unavailable"},
		fmt.Errorf("spend: %w", wcdomain.ErrChallengeUnavailable): {http.StatusServiceUnavailable, "challenge_unavailable"},
	}
	for err, want := range cases {
		rec := httptest.NewRecorder()
		writeError(rec, err)
		var body struct {
			Code string `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != want.status || body.Code != want.code {
			t.Fatalf("%v: got %d %q (%s), want %d %q", err, rec.Code, body.Code, rec.Body.String(), want.status, want.code)
		}
	}
}
