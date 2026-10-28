package sitepublish_repository

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"vozko/domain/sitepublish"
)

func TestPostgresHostnamePagesWalkEveryRowStillHoldingAProviderRecord(t *testing.T) {
	db := postgresDB(t)
	repo := NewHostnames(db)
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	rows := []struct {
		status sitepublish.HostnameStatus
		kind   sitepublish.HostnameKind
		ref    string
	}{
		{sitepublish.HostnameSuspended, sitepublish.HostnameCustom, "cf-1"},
		{sitepublish.HostnameFailed, sitepublish.HostnameCustom, "cf-2"},
		{sitepublish.HostnamePendingDNS, sitepublish.HostnameCustom, "cf-3"},
		{sitepublish.HostnameSuspended, sitepublish.HostnameCustom, ""},
		{sitepublish.HostnameFailed, sitepublish.HostnameCustom, ""},
		{sitepublish.HostnameActive, sitepublish.HostnameCustom, "cf-4"},
		{sitepublish.HostnamePendingSSL, sitepublish.HostnameCustom, "cf-5"},
		{sitepublish.HostnameRemoving, sitepublish.HostnameCustom, "cf-6"},
		{sitepublish.HostnameRemoving, sitepublish.HostnameCustom, ""},
		{sitepublish.HostnameRemoving, sitepublish.HostnamePlatform, ""},
	}
	ids := map[string]string{}
	for i, row := range rows {
		id := fmt.Sprintf("00000000-0000-4000-8000-%012d", 10-i)
		ids[row.ref+string(row.status)+string(row.kind)] = id
		rec := sitepublish.HostnameRecord{
			ID: id, WorkspaceID: workspaceID, ProjectID: projectID, Hostname: fmt.Sprintf("h%d.cliente.com.br", i),
			Kind: row.kind, Status: row.status, CloudflareRef: row.ref, CreatedAt: now, UpdatedAt: now.Add(-time.Duration(i) * time.Minute),
		}
		if err := repo.Create(ctx, rec); err != nil {
			t.Fatal(err)
		}
	}

	page := sitepublish.HostnamePage{Statuses: sitepublish.ProviderlessStatuses(), Kind: sitepublish.HostnameCustom, HoldingProviderRecord: true, Limit: 2}
	var walked []string
	for {
		listed, err := repo.ListPage(ctx, page)
		if err != nil {
			t.Fatal(err)
		}
		for _, rec := range listed {
			walked = append(walked, rec.CloudflareRef)
		}
		if len(listed) < page.Limit {
			break
		}
		page.AfterID = listed[len(listed)-1].ID
	}
	if want := []string{"cf-3", "cf-2", "cf-1"}; !slices.Equal(walked, want) {
		t.Fatalf("walked %v, want every leftover provider record once in id order %v", walked, want)
	}

	removing, err := repo.ListPage(ctx, sitepublish.HostnamePage{Statuses: []sitepublish.HostnameStatus{sitepublish.HostnameRemoving}, Kind: sitepublish.HostnameCustom, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(removing) != 2 || removing[0].ID != ids["removingcustom"] || removing[1].ID != ids["cf-6removingcustom"] {
		t.Fatalf("a page without the provider filter lists every row of the status and kind, got %+v", removing)
	}
}
