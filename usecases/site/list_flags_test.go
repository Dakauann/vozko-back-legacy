package site_usecase

import (
	"context"
	"errors"
	"testing"

	"vozko/domain/site"
	"vozko/domain/sitepublish"
)

func TestTheListSaysWhichPagesHaveUnpublishedChangesLikeTheDetail(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	draft := h.create(t, site.StackHTML, "blank", "Rascunho")
	current := h.create(t, site.StackHTML, "blank", "No ar")
	live := h.store.projects[current.Project.ID]
	live.Status, live.PublishedDeploymentID = StatusPublished, "dep-1"
	h.store.projects[current.Project.ID] = live
	h.published.revisions["dep-1"] = live.WorkingRevisionID

	items, _, err := h.svc.List(ctx, ListQuery{WorkspaceID: wsA})
	if err != nil {
		t.Fatal(err)
	}
	flags := map[string]bool{}
	for _, p := range items {
		flags[p.ID] = p.HasUnpublishedChanges
	}
	if !flags[draft.Project.ID] || flags[current.Project.ID] {
		t.Fatalf("got %v", flags)
	}
	detail, err := h.svc.Get(ctx, wsA, current.Project.ID)
	if err != nil || detail.HasUnpublishedChanges != flags[current.Project.ID] || detail.Project.HasUnpublishedChanges != detail.HasUnpublishedChanges {
		t.Fatalf("the detail and the list agree: %+v %v", detail, err)
	}
	h.published.err = errors.New("deployments down")
	if _, _, err := h.svc.List(ctx, ListQuery{WorkspaceID: wsA}); !errors.Is(err, h.published.err) {
		t.Fatalf("an unreadable deployment refuses the list: %v", err)
	}
}

func (h *harness) goLive(t *testing.T, v *View) {
	t.Helper()
	live := h.store.projects[v.Project.ID]
	live.Status, live.PublishedDeploymentID = StatusPublished, "dep-"+v.Project.ID
	h.store.projects[v.Project.ID] = live
	h.published.revisions[live.PublishedDeploymentID] = live.WorkingRevisionID
}

func TestEveryWriteAnswerCarriesThePublishState(t *testing.T) {
	ctx := context.Background()
	writes := map[string]func(h *harness, v *View) (*View, error){
		"save": func(h *harness, v *View) (*View, error) {
			return h.svc.SaveFiles(ctx, SaveFilesInput{WorkspaceID: wsA, ProjectID: v.Project.ID, ExpectedVersion: v.Project.Version, Author: PersonAuthor(userA), Files: map[string]string{"pages/index.html": "<main>outra</main>"}})
		},
		"unchanged save": func(h *harness, v *View) (*View, error) {
			return h.svc.SaveFiles(ctx, SaveFilesInput{WorkspaceID: wsA, ProjectID: v.Project.ID, ExpectedVersion: v.Project.Version, Author: PersonAuthor(userA), Files: map[string]string{"pages/index.html": v.Files["pages/index.html"].Content}})
		},
		"version": func(h *harness, v *View) (*View, error) {
			return h.svc.SaveVersion(ctx, SaveVersionInput{WorkspaceID: wsA, ProjectID: v.Project.ID, ExpectedVersion: v.Project.Version, Author: PersonAuthor(userA), Label: "Antes"})
		},
		"restore": func(h *harness, v *View) (*View, error) {
			return h.svc.Restore(ctx, RestoreInput{WorkspaceID: wsA, ProjectID: v.Project.ID, RevisionID: v.Revision.ID, ExpectedVersion: v.Project.Version, Author: PersonAuthor(userA)})
		},
	}
	for name, write := range writes {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			v := h.create(t, site.StackHTML, "blank", "No ar")
			h.goLive(t, v)
			v.Project = h.store.projects[v.Project.ID]
			got, err := write(h, v)
			if err != nil {
				t.Fatal(err)
			}
			changed := got.Project.WorkingRevisionID != v.Project.WorkingRevisionID
			if got.Project.HasUnpublishedChanges != changed || (changed && got.Project.PublishState() != PublishLiveWithChanges) || (!changed && got.Project.PublishState() != PublishLive) {
				t.Fatalf("changed %v: unpublished %v state %s", changed, got.Project.HasUnpublishedChanges, got.Project.PublishState())
			}
		})
	}
}

func TestEditAnswersCarryThePublishState(t *testing.T) {
	h := newHarness(t)
	v := h.create(t, site.StackHTML, "blank", "No ar")
	h.goLive(t, v)
	page := "pages/index.html"
	h.executor.steps = func(req ExecuteRequest) []site.StepPatch {
		return []site.StepPatch{{Op: site.OpSetText, Files: map[string]string{page: req.Project.Files[page].Content + "\n"}}}
	}
	res, err := h.svc.Edit(context.Background(), EditInput{WorkspaceID: wsA, ProjectID: v.Project.ID, ExpectedVersion: h.store.projects[v.Project.ID].Version, Author: PersonAuthor(userA),
		Batch: setTextBatch(page+"#"+offsetOf(v.Files[page].Content, "<h1"), 1, "Oi")})
	if err != nil {
		t.Fatal(err)
	}
	if !res.View.Project.HasUnpublishedChanges || res.View.Project.PublishState() != PublishLiveWithChanges {
		t.Fatalf("got %+v", res.View.Project)
	}
}

func TestAWriteRefusesBeforeCommittingWhenThePublicationIsUnreadable(t *testing.T) {
	h := newHarness(t)
	v := h.create(t, site.StackHTML, "blank", "No ar")
	h.goLive(t, v)
	before := h.store.projects[v.Project.ID]
	h.published.err = errors.New("deployments down")
	_, err := h.svc.SaveFiles(context.Background(), SaveFilesInput{WorkspaceID: wsA, ProjectID: v.Project.ID, ExpectedVersion: before.Version, Author: PersonAuthor(userA), Files: map[string]string{"pages/index.html": "<main>outra</main>"}})
	if !errors.Is(err, h.published.err) || h.store.projects[v.Project.ID].Version != before.Version {
		t.Fatalf("got %v, version %d", err, h.store.projects[v.Project.ID].Version)
	}
}

func TestUpdateAnswersCarryThePublishState(t *testing.T) {
	h := newHarness(t)
	v := h.create(t, site.StackHTML, "blank", "No ar")
	h.goLive(t, v)
	name := "Outro nome"
	p, err := h.svc.Update(context.Background(), wsA, v.Project.ID, h.store.projects[v.Project.ID].Version, Change{Name: &name})
	if err != nil || p.HasUnpublishedChanges || p.PublishState() != PublishLive {
		t.Fatalf("got %+v %v", p, err)
	}
	draft := h.create(t, site.StackHTML, "blank", "Rascunho")
	if !draft.Project.HasUnpublishedChanges {
		t.Fatalf("a new page has unpublished changes: %+v", draft.Project)
	}
}

func TestTheListCarriesEachPrimaryURLFromOneHostnameRead(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	first := h.create(t, site.StackHTML, "blank", "Primeira")
	second := h.create(t, site.StackHTML, "blank", "Segunda")
	h.create(t, site.StackHTML, "blank", "Sem endereço")
	h.hostnames.list = []sitepublish.HostnameRecord{
		{ProjectID: first.Project.ID, Hostname: "primeira.paginas.test", Kind: sitepublish.HostnamePlatform, Status: sitepublish.HostnameActive},
		{ProjectID: first.Project.ID, Hostname: "www.primeira.com.br", Kind: sitepublish.HostnameCustom, Status: sitepublish.HostnameActive},
		{ProjectID: second.Project.ID, Hostname: "segunda.paginas.test", Kind: sitepublish.HostnamePlatform, Status: sitepublish.HostnameActive},
		{ProjectID: second.Project.ID, Hostname: "www.segunda.com.br", Kind: sitepublish.HostnameCustom, Status: sitepublish.HostnamePendingDNS},
	}
	h.hostnames.calls = 0
	items, _, err := h.svc.List(ctx, ListQuery{WorkspaceID: wsA})
	if err != nil {
		t.Fatal(err)
	}
	urls := map[string]string{}
	for _, p := range items {
		urls[p.Name] = p.PrimaryURL
	}
	if urls["Primeira"] != "https://www.primeira.com.br" || urls["Segunda"] != "https://segunda.paginas.test" || urls["Sem endereço"] != "" || h.hostnames.calls != 1 {
		t.Fatalf("got %v after %d reads", urls, h.hostnames.calls)
	}
	h.hostnames.err = errors.New("db down")
	if _, _, err := h.svc.List(ctx, ListQuery{WorkspaceID: wsA}); !errors.Is(err, h.hostnames.err) {
		t.Fatalf("an unreadable hostname list refuses the list: %v", err)
	}
}
