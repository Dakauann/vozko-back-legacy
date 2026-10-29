package sitenext_usecase

import (
	"context"
	"errors"
	"slices"
	"testing"

	"vozko/domain/site"
	"vozko/domain/sitenext"
	"vozko/domain/sitepublish"
	site_usecase "vozko/usecases/site"
	"vozko/usecases/siteinspect"
)

type fakeProjects struct {
	view *site_usecase.View
	err  error
}

func (f *fakeProjects) Get(_ context.Context, workspaceID, id string) (*site_usecase.View, error) {
	if f.err != nil {
		return nil, f.err
	}
	if workspaceID != "ws-1" || id != "p-1" {
		return nil, site_usecase.ErrProjectNotFound
	}
	return f.view, nil
}

type fakeOutlines struct {
	report siteinspect.Report
}

func (f *fakeOutlines) Outline(_ context.Context, _ *site_usecase.View) siteinspect.Report {
	return f.report
}

type fakeHostnames struct {
	recs  []sitepublish.HostnameRecord
	err   error
	calls int
}

func (f *fakeHostnames) ListByProject(_ context.Context, workspaceID, projectID string) ([]sitepublish.HostnameRecord, error) {
	f.calls++
	if workspaceID != "ws-1" || projectID != "p-1" {
		return nil, errors.New("wrong scope")
	}
	return f.recs, f.err
}

func view(status site_usecase.Status, changes bool, content string, settings *site_usecase.StoredSettings) *site_usecase.View {
	return &site_usecase.View{
		Project:  site_usecase.Project{ID: "p-1", WorkspaceID: "ws-1", Stack: site.StackHTML, Status: status, HasUnpublishedChanges: changes},
		Files:    map[string]site.File{"pages/index.html": {Content: content}},
		Settings: settings,
	}
}

func service(t *testing.T, projects *fakeProjects, outlines *fakeOutlines, hostnames *fakeHostnames) *Service {
	t.Helper()
	svc, err := New(Deps{Projects: projects, Outlines: outlines, Hostnames: hostnames})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func codes(steps []sitenext.Step) []string {
	out := []string{}
	for _, step := range steps {
		out = append(out, step.Code)
	}
	return out
}

func TestNewRefusesMissingDependencies(t *testing.T) {
	if _, err := New(Deps{}); err == nil {
		t.Fatal("expected a missing dependency error")
	}
}

func TestNextReadsTheWorkingVersionSettingsAndOutline(t *testing.T) {
	stored := &site_usecase.StoredSettings{Settings: site.Settings{Site: site.Identity{FaviconMediaID: "m-1"}}}
	projects := &fakeProjects{view: view(site_usecase.StatusDraft, false, "<p>Agende sua avaliação</p>", stored)}
	outlines := &fakeOutlines{report: siteinspect.Report{Known: true, Forms: []string{"contato"}, Gaps: []site.Issue{}}}
	hostnames := &fakeHostnames{}
	got, err := service(t, projects, outlines, hostnames).Next(context.Background(), "ws-1", "p-1")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{sitenext.CodeFormDestination, sitenext.CodeAddBooking, sitenext.CodePublish}
	if !got.Checked || !slices.Equal(codes(got.Steps), want) {
		t.Fatalf("got %+v want %v", got, want)
	}
	if hostnames.calls != 0 {
		t.Fatalf("a draft never reads hostnames, read %d", hostnames.calls)
	}
}

func TestNextSuggestsACustomDomainOnlyForALivePageWithoutOne(t *testing.T) {
	stored := &site_usecase.StoredSettings{Settings: site.Settings{Site: site.Identity{FaviconMediaID: "m-1", WebchatWidget: "w-1"}}}
	projects := &fakeProjects{view: view(site_usecase.StatusPublished, false, "<p>Olá</p>", stored)}
	outlines := &fakeOutlines{report: siteinspect.Report{Known: true}}
	hostnames := &fakeHostnames{recs: []sitepublish.HostnameRecord{{Kind: sitepublish.HostnamePlatform, Status: sitepublish.HostnameActive}}}
	svc := service(t, projects, outlines, hostnames)
	got, err := svc.Next(context.Background(), "ws-1", "p-1")
	if err != nil || !slices.Equal(codes(got.Steps), []string{sitenext.CodeCustomDomain}) {
		t.Fatalf("got %+v %v", got, err)
	}
	hostnames.recs = append(hostnames.recs, sitepublish.HostnameRecord{Kind: sitepublish.HostnameCustom, Status: sitepublish.HostnameActive})
	got, err = svc.Next(context.Background(), "ws-1", "p-1")
	if err != nil || len(got.Steps) != 0 || !got.Checked {
		t.Fatalf("got %+v %v", got, err)
	}
}

func TestNextMapsThePublishState(t *testing.T) {
	cases := []struct {
		status  site_usecase.Status
		changes bool
		want    sitenext.Stage
	}{
		{site_usecase.StatusDraft, false, sitenext.StageDraft},
		{site_usecase.StatusPublished, false, sitenext.StageLive},
		{site_usecase.StatusPublished, true, sitenext.StageLiveWithChanges},
		{site_usecase.StatusSuspended, false, sitenext.StageClosed},
		{site_usecase.StatusArchived, false, sitenext.StageClosed},
	}
	for _, tc := range cases {
		if got := stageOf(site_usecase.Project{Status: tc.status, HasUnpublishedChanges: tc.changes}); got != tc.want {
			t.Fatalf("%s %v got %s", tc.status, tc.changes, got)
		}
	}
}

func TestNextFailsClosed(t *testing.T) {
	t.Run("an unread outline gives no steps", func(t *testing.T) {
		projects := &fakeProjects{view: view(site_usecase.StatusDraft, false, "<p>Olá</p>", nil)}
		outlines := &fakeOutlines{report: siteinspect.Report{ReasonCode: siteinspect.CodeNotInspected}}
		got, err := service(t, projects, outlines, &fakeHostnames{}).Next(context.Background(), "ws-1", "p-1")
		if err != nil || got.Checked || got.Steps == nil || len(got.Steps) != 0 {
			t.Fatalf("got %+v %v", got, err)
		}
	})
	t.Run("an unreadable hostname list is an error, never a guess", func(t *testing.T) {
		projects := &fakeProjects{view: view(site_usecase.StatusPublished, false, "<p>Olá</p>", nil)}
		outlines := &fakeOutlines{report: siteinspect.Report{Known: true}}
		_, err := service(t, projects, outlines, &fakeHostnames{err: errors.New("db down")}).Next(context.Background(), "ws-1", "p-1")
		if !errors.Is(err, ErrUnavailable) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("a missing page passes its error through", func(t *testing.T) {
		_, err := service(t, &fakeProjects{view: view(site_usecase.StatusDraft, false, "", nil)}, &fakeOutlines{}, &fakeHostnames{}).Next(context.Background(), "ws-1", "other")
		if !errors.Is(err, site_usecase.ErrProjectNotFound) {
			t.Fatalf("got %v", err)
		}
	})
}
