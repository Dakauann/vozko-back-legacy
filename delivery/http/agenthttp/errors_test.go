package agenthttp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"vozko/delivery/http/response"
	"vozko/delivery/http/response/responsetest"
	"vozko/domain/agenttool"
	"vozko/domain/lint"
	"vozko/pkg/codegolden"
)

const agentErrorCodesAdvice = "update testdata/error_codes.golden, add the code to vozko-front src/lib/agents/error-codes.ts and translate agents.errors.<code> in the pt, en, es and de catalogs"

func TestEveryAgentErrorWritesItsStableCode(t *testing.T) {
	responsetest.RequireEveryErrorWritesItsCode(t, Refusals, WriteError)
	responsetest.RequireStableCodes(t, RequestRefusals)
}

func TestAnUnexpectedAgentErrorStillWritesACode(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, errors.New("db down"))
	if rec.Code != http.StatusInternalServerError || responsetest.CodeOf(t, rec) != RefusalInternal.Code {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestAgentErrorCodesMatchTheGoldenSoTheFrontTranslatesThem(t *testing.T) {
	codes := response.RefusalCodes(response.ErrorRefusalsOf(Refusals), RequestRefusals)
	codegolden.Compare(t, filepath.Join("testdata", "error_codes.golden"), codes, agentErrorCodesAdvice)
}

func TestBlockingIssuesAnswer422WithTheIssueList(t *testing.T) {
	report := agenttool.Report{Issues: []agenttool.Issue{{Code: agenttool.CodeHTTPURLInvalid, Severity: lint.SeverityBlocking, ToolID: "t1", Message: "x"}}}
	for kind, code := range map[error]string{agenttool.ErrNotActivatable: "agent_not_activatable", agenttool.ErrNotPublishable: "agent_not_publishable"} {
		rec := httptest.NewRecorder()
		WriteError(rec, agenttool.BlockedBy(kind, report))
		if rec.Code != http.StatusUnprocessableEntity || responsetest.CodeOf(t, rec) != code || !strings.Contains(rec.Body.String(), `"toolId":"t1"`) {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}
	}
}
