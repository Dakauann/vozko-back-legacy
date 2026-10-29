package site_repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"vozko/domain/site"
	"vozko/domain/sitepublish"
	site_usecase "vozko/usecases/site"
)

type noBuilder struct{}

func (noBuilder) Execute(context.Context, site_usecase.ExecuteRequest) (site_usecase.ExecuteResult, error) {
	return site_usecase.ExecuteResult{}, errors.New("unused")
}

func (noBuilder) Extract(context.Context, site_usecase.ExtractRequest) (site_usecase.ExtractResult, error) {
	return site_usecase.ExtractResult{}, errors.New("unused")
}

func (noBuilder) Locate(context.Context, string, []string) (map[string]string, error) {
	return map[string]string{}, nil
}

func (noBuilder) Preview(context.Context, site_usecase.PreviewRequest) (site_usecase.PreviewResult, error) {
	return site_usecase.PreviewResult{}, errors.New("unused")
}

type anyReference struct{}

func (anyReference) Check(context.Context, string, string, site.FormSettings) error { return nil }

func (anyReference) CheckWebchat(context.Context, string, string) error { return nil }

type noForms struct{}

func (noForms) Deactivate(context.Context, string) error { return nil }

func (noForms) PublishedRevision(context.Context, string, string) (string, error) { return "", nil }

func (noForms) Retire(context.Context, string, string) error { return nil }

func (noForms) NumberOf(context.Context, string, string) (string, error) {
	return "", errors.New("unused")
}

func (noForms) ListByProjects(context.Context, string, []string) ([]sitepublish.HostnameRecord, error) {
	return nil, nil
}

type memoryPreviews map[string]string

func (m memoryPreviews) GetString(key string) (string, error) { return m[key], nil }

func (m memoryPreviews) SetString(key, value string, _ time.Duration) error {
	m[key] = value
	return nil
}

func TestAStarterPageSurvivesSavesAndARestoreOnPostgres(t *testing.T) {
	repo, _ := isolated(t, "site_service")
	svc, err := site_usecase.NewService(site_usecase.Deps{Store: repo, Executor: noBuilder{}, Previewer: noBuilder{}, References: anyReference{}, Forms: noForms{}, Retirement: noForms{}, Published: noForms{}, Media: noBuilder{},
		Numbers: noForms{}, Hostnames: noForms{}, Previews: memoryPreviews{}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ws, user := uuid.NewString(), uuid.NewString()
	created, err := svc.Create(ctx, site_usecase.CreateInput{WorkspaceID: ws, UserID: user, Name: "Clínica Sorriso", Stack: site.StackReact, Starter: "captura-de-leads"})
	if err != nil {
		t.Fatal(err)
	}
	page := "src/routes/index.jsx"
	saved, err := svc.SaveFiles(ctx, site_usecase.SaveFilesInput{WorkspaceID: ws, ProjectID: created.Project.ID, ExpectedVersion: 1,
		Author: site_usecase.PersonAuthor(user), Files: map[string]string{page: "export default function Home() {\n  return <h1>Oi</h1>;\n}\n"}})
	if err != nil {
		t.Fatal(err)
	}
	restored, err := svc.Restore(ctx, site_usecase.RestoreInput{WorkspaceID: ws, ProjectID: created.Project.ID, RevisionID: created.Revision.ID,
		ExpectedVersion: saved.Project.Version, Author: site_usecase.PersonAuthor(user)})
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(ctx, ws, created.Project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Files[page].Content != created.Files[page].Content || got.Files[page].Rev != 3 || got.Revision.ID != restored.Revision.ID || got.Project.Version != 3 {
		t.Fatalf("got %+v version %d", got.Files[page], got.Project.Version)
	}
	revisions, total, err := svc.ListRevisions(ctx, ws, created.Project.ID, 10, 0)
	if err != nil || total != 3 || revisions[0].Kind != site_usecase.RevisionRestore {
		t.Fatalf("got %+v %d %v", revisions, total, err)
	}
}
