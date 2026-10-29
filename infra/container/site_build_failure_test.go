package container

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/site"
	"vozko/domain/sitepublish"
	site_usecase "vozko/usecases/site"
	"vozko/usecases/sitebuild"
	sitepublish_usecase "vozko/usecases/sitepublish"
)

type brokenSiteBuilder struct {
	*replayBuilder
}

var brokenBuildError = sitebuild.BuildError{Path: "src/App.tsx", Line: 4, Column: 9, Message: "dynamic import", Code: "import_dynamic_forbidden"}

func (brokenSiteBuilder) Build(context.Context, sitebuild.BuildRequest) (sitebuild.BuildResult, error) {
	return sitebuild.BuildResult{Errors: []sitebuild.BuildError{brokenBuildError}}, nil
}

func brokenBuildWorld(t *testing.T) (*siteWorld, *sitebuild.Service, *site_usecase.View) {
	t.Helper()
	w := newSiteWorld(t)
	c := &Container{siteFormsWiring: w.forms}
	service, err := sitebuild.NewService(sitebuild.Config{
		Addresses: sitepublish.Addresses{Domain: e2ePublic}, SigningKey: e2eSigningKey, APIOrigin: e2eAPI, Runtime: siteRuntimeFiles(),
	}, sitebuild.Deps{
		Builder: brokenSiteBuilder{w.builder}, Cache: &e2eCache{items: map[string]any{}}, Media: e2eNoMedia{}, Sections: w.sections,
		WhatsApp: c.siteFormsForPublish(), Widgets: e2eNoWidgets{}, Workspaces: e2eWorkspaces{e2eWorkspace: "Clínica Sorriso"}, Now: w.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := w.storage.Create(context.Background(), site_usecase.CreateInput{WorkspaceID: e2eWorkspace, UserID: e2eUser, Name: "Clínica Sorriso", Stack: site.StackReact, Starter: "captura-de-leads"})
	if err != nil {
		t.Fatal(err)
	}
	return w, service, created
}

func TestAPreviewThatDoesNotCompileKeepsTheBuilderErrorCode(t *testing.T) {
	_, service, created := brokenBuildWorld(t)

	_, err := siteBuildEditor{service: service}.Preview(context.Background(), site_usecase.PreviewRequest{
		WorkspaceID: e2eWorkspace, ProjectID: created.Project.ID, RevisionID: created.Revision.ID, Project: created.Source(), Settings: created.Settings.Settings,
	})

	var failed *site_usecase.BuildFailedError
	if !errors.As(err, &failed) {
		t.Fatalf("err = %v, want a build failure", err)
	}
	want := site_usecase.BuildIssue{Path: brokenBuildError.Path, Line: brokenBuildError.Line, Column: brokenBuildError.Column, Message: brokenBuildError.Message, Code: brokenBuildError.Code}
	if len(failed.Errors) != 1 || failed.Errors[0] != want {
		t.Fatalf("errors = %+v, want %+v", failed.Errors, want)
	}
}

func TestAPublishCandidateThatDoesNotCompileKeepsTheBuilderErrorCode(t *testing.T) {
	_, service, created := brokenBuildWorld(t)

	candidate, err := siteCandidateBuilder{service: service}.Candidate(context.Background(), sitepublish_usecase.CandidateRequest{
		WorkspaceID: e2eWorkspace, ProjectID: created.Project.ID, Project: created.Source(), Settings: created.Settings.Settings, FormKeys: map[string]string{"contato": "fk_contato"},
	})

	if err != nil {
		t.Fatalf("err = %v, want the failure carried on the candidate", err)
	}
	want := sitepublish_usecase.BuildError{Path: brokenBuildError.Path, Line: brokenBuildError.Line, Column: brokenBuildError.Column, Message: brokenBuildError.Message, Code: brokenBuildError.Code}
	if len(candidate.Errors) != 1 || candidate.Errors[0] != want {
		t.Fatalf("errors = %+v, want %+v", candidate.Errors, want)
	}
}

func TestAnUnreachableBuilderIsNotReportedAsACompileFailure(t *testing.T) {
	err := siteEditorBuildError(sitebuild.ErrBuilderUnavailable)
	var failed *site_usecase.BuildFailedError
	if errors.As(err, &failed) || !errors.Is(err, sitebuild.ErrBuilderUnavailable) {
		t.Fatalf("err = %v", err)
	}
}
