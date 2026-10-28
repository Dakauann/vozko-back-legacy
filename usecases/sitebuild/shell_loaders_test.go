package sitebuild

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"vozko/domain/site"
)

func TestTheShellCarriesTheExactLoaderOfEveryEnabledIntegration(t *testing.T) {
	f := newFixture(t)
	in := previewInput()
	in.Settings.Integrations = site.Integrations{GA4: "G-AB12CD34", Clarity: "abc123def"}
	_, err := f.svc.Candidate(context.Background(), CandidateInput{
		WorkspaceID: in.WorkspaceID, ProjectID: in.ProjectID, Project: in.Project, Settings: in.Settings, PrimaryOrigin: "https://sorriso.paginas.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	shell := f.builder.builds[0].Shell
	ga4, _ := site.IntegrationLoader(site.IntegrationGA4)
	clarity, _ := site.IntegrationLoader(site.IntegrationClarity)
	if len(shell.Loaders) != 2 || shell.Loaders["ga4"] != ga4 || shell.Loaders["clarity"] != clarity {
		t.Fatalf("got %v", shell.Loaders)
	}
	raw, err := json.Marshal(shell)
	if err != nil || !strings.Contains(string(raw), `"loaders":{`) {
		t.Fatalf("the builder reads loaders: %s %v", raw, err)
	}
}
