package conversationad

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/domain/sitepublish"
)

type projectShelf struct {
	rows  map[string]conversation.PageProject
	err   error
	asked []string
}

func (s *projectShelf) PageProject(_ context.Context, workspaceID, projectID string) (*conversation.PageProject, error) {
	s.asked = append(s.asked, workspaceID+"|"+projectID)
	if s.err != nil {
		return nil, s.err
	}
	if row, ok := s.rows[workspaceID+"|"+projectID]; ok {
		return &row, nil
	}
	return nil, nil
}

type hostShelf struct {
	rows  []sitepublish.HostnameRecord
	err   error
	asked []string
}

func (s *hostShelf) ListByProject(_ context.Context, workspaceID, projectID string) ([]sitepublish.HostnameRecord, error) {
	s.asked = append(s.asked, workspaceID+"|"+projectID)
	return s.rows, s.err
}

func storedPageOrigin() *pageBook {
	return &pageBook{rows: map[string]conversation.PageOrigin{
		"e1" + string(shared.EntryTypeWhatsApp): {
			EntryID: "e1", EntryType: shared.EntryTypeWhatsApp, WorkspaceID: "w1",
			ProjectID: "p1", Route: "/oferta", Reference: "LP7K2", ArrivedAt: pageNow,
		},
	}}
}

func liveHosts() *hostShelf {
	return &hostShelf{rows: []sitepublish.HostnameRecord{
		{Hostname: "loja.vozko.site", Kind: sitepublish.HostnamePlatform, Status: sitepublish.HostnameActive},
	}}
}

func pageReader(book *pageBook, projects *projectShelf, hosts *hostShelf) *PageReader {
	return NewPageReader(book, entryWorkspaces{"e1": "w1"}, projects, hosts, sitepublish.Origins{})
}

func TestPageReader_NamesTheLivePageWithItsAddress(t *testing.T) {
	projects := &projectShelf{rows: map[string]conversation.PageProject{"w1|p1": {Name: "Loja de outubro", Live: true}}}
	view, err := pageReader(storedPageOrigin(), projects, liveHosts()).PageOrigin(context.Background(), "e1", shared.EntryTypeWhatsApp)
	if err != nil || view == nil {
		t.Fatalf("view=%+v err=%v", view, err)
	}
	want := conversation.PageOriginView{
		ProjectID: "p1", ProjectName: "Loja de outubro", Route: "/oferta",
		URL: "https://loja.vozko.site/oferta", Reference: "LP7K2", ArrivedAt: pageNow,
	}
	if *view != want {
		t.Fatalf("view = %+v, want %+v", *view, want)
	}
	if projects.asked[0] != "w1|p1" {
		t.Fatalf("project asked as %v", projects.asked)
	}
}

func TestPageReader_AnOfflinePageHasNoAddress(t *testing.T) {
	projects := &projectShelf{rows: map[string]conversation.PageProject{"w1|p1": {Name: "Loja", Live: false}}}
	hosts := liveHosts()
	view, err := pageReader(storedPageOrigin(), projects, hosts).PageOrigin(context.Background(), "e1", shared.EntryTypeWhatsApp)
	if err != nil || view == nil || view.URL != "" || view.ProjectName != "Loja" {
		t.Fatalf("view=%+v err=%v", view, err)
	}
	if len(hosts.asked) != 0 {
		t.Fatal("an offline page must not look up hostnames")
	}
}

func TestPageReader_ALivePageWithoutAnActiveHostnameHasNoAddress(t *testing.T) {
	projects := &projectShelf{rows: map[string]conversation.PageProject{"w1|p1": {Name: "Loja", Live: true}}}
	hosts := &hostShelf{rows: []sitepublish.HostnameRecord{{Hostname: "loja.vozko.site", Kind: sitepublish.HostnamePlatform, Status: sitepublish.HostnamePendingDNS}}}
	view, err := pageReader(storedPageOrigin(), projects, hosts).PageOrigin(context.Background(), "e1", shared.EntryTypeWhatsApp)
	if err != nil || view == nil || view.URL != "" {
		t.Fatalf("view=%+v err=%v", view, err)
	}
}

func TestPageReader_ADeletedPageStillShowsTheOriginWithoutAName(t *testing.T) {
	view, err := pageReader(storedPageOrigin(), &projectShelf{}, liveHosts()).PageOrigin(context.Background(), "e1", shared.EntryTypeWhatsApp)
	if err != nil || view == nil || view.ProjectName != "" || view.URL != "" || view.Route != "/oferta" {
		t.Fatalf("view=%+v err=%v", view, err)
	}
}

func TestPageReader_NoOriginReadsNothingElse(t *testing.T) {
	projects := &projectShelf{}
	view, err := pageReader(&pageBook{}, projects, liveHosts()).PageOrigin(context.Background(), "e1", shared.EntryTypeWhatsApp)
	if err != nil || view != nil || len(projects.asked) != 0 {
		t.Fatalf("view=%+v err=%v asked=%v", view, err, projects.asked)
	}
}

func TestPageReader_RefusesAnOriginFromAnotherWorkspace(t *testing.T) {
	book := storedPageOrigin()
	projects := &projectShelf{rows: map[string]conversation.PageProject{"w1|p1": {Name: "Loja", Live: true}}}
	reader := NewPageReader(book, entryWorkspaces{"e1": "w2"}, projects, liveHosts(), sitepublish.Origins{})
	view, err := reader.PageOrigin(context.Background(), "e1", shared.EntryTypeWhatsApp)
	if !errors.Is(err, ErrPageOriginForeign) || view != nil || len(projects.asked) != 0 {
		t.Fatalf("view=%+v err=%v asked=%v", view, err, projects.asked)
	}
}

func TestPageReader_FailsClosedWhenAnyReadFails(t *testing.T) {
	ctx := context.Background()
	live := map[string]conversation.PageProject{"w1|p1": {Name: "Loja", Live: true}}
	cases := map[string]*PageReader{
		"origin":    pageReader(&pageBook{getErr: errors.New("db down")}, &projectShelf{rows: live}, liveHosts()),
		"workspace": NewPageReader(storedPageOrigin(), entryWorkspaces{}, &projectShelf{rows: live}, liveHosts(), sitepublish.Origins{}),
		"project":   pageReader(storedPageOrigin(), &projectShelf{err: errors.New("db down")}, liveHosts()),
		"hostnames": pageReader(storedPageOrigin(), &projectShelf{rows: live}, &hostShelf{err: errors.New("db down")}),
	}
	for name, reader := range cases {
		view, err := reader.PageOrigin(ctx, "e1", shared.EntryTypeWhatsApp)
		if err == nil || view != nil {
			t.Fatalf("%s: view=%+v err=%v", name, view, err)
		}
	}
}
