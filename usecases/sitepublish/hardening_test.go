package sitepublish_usecase

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"vozko/domain/media"
	"vozko/domain/site"
	"vozko/domain/sitepublish"
	workspace_plan "vozko/domain/workspace/workspace_plan"
)

func addressed(r *rig) []string {
	return keys(r.hosting.deployments)
}

func TestASupersededDeploymentLosesItsImmutableAddress(t *testing.T) {
	r := newRig(t)
	first := publish(t, r)
	second := publish(t, r)
	if got := addressed(r); !slices.Equal(got, []string{second.ID}) {
		t.Fatalf("only the live deployment keeps an address, got %v (first %s)", got, first.ID)
	}
}

func TestRollbackRewritesOnlyTheActiveAddress(t *testing.T) {
	r := newRig(t)
	first := publish(t, r)
	publish(t, r)
	if _, err := r.svc.Rollback(context.Background(), actor, projectID, first.ID); err != nil {
		t.Fatal(err)
	}
	if got := addressed(r); !slices.Equal(got, []string{first.ID}) {
		t.Fatalf("got %v", got)
	}
}

func TestUnpublishDeletesTheAddressOfEveryDeployment(t *testing.T) {
	r := newRig(t)
	publish(t, r)
	publish(t, r)
	stale := sitepublish.DeploymentRecord{ID: "stale-ready", WorkspaceID: wsID, ProjectID: projectID, Status: sitepublish.DeploymentReady}
	r.deployments.rows[stale.ID] = stale
	r.deployments.seq = append(r.deployments.seq, stale.ID)
	r.hosting.deployments[stale.ID] = []byte("{}")
	if err := r.svc.Unpublish(context.Background(), actor, projectID); err != nil {
		t.Fatal(err)
	}
	if len(r.hosting.deployments) != 0 || len(r.hosting.hosts) != 0 {
		t.Fatalf("nothing stays served: deploy %v hosts %v", addressed(r), keys(r.hosting.hosts))
	}
	if !slices.Contains(r.hosting.dropped, stale.ID) {
		t.Fatal("a deployment that never went live is dropped too")
	}
}

func TestUnpublishChecksTheWorkspaceBeforeAnySideEffect(t *testing.T) {
	r := newRig(t)
	stranger := Actor{UserID: "user-2", WorkspaceID: "ws-other"}
	if err := r.svc.Unpublish(context.Background(), stranger, projectID); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("got %v", err)
	}
	if len(r.forms.deactivated) != 0 || len(r.projects.publications) != 0 || len(r.hosting.dropped) != 0 {
		t.Fatal("another workspace never touches the page")
	}
	if !slices.Contains(r.projects.described, "ws-other|"+projectID) {
		t.Fatal("ownership is read through the actor workspace")
	}
}

func TestOfflinePathsKeepOnlyTheActiveAddressSuspended(t *testing.T) {
	for name, cut := range map[string]func(r *rig) error{
		"admin suspension": func(r *rig) error {
			_, err := r.svc.AdminSuspend(context.Background(), "admin-1", projectID, "phishing")
			return err
		},
		"billing cut": func(r *rig) error {
			r.subs.err = workspace_plan.ErrSubscriptionNotActive
			return r.svc.OnAccessCut(context.Background(), wsID)
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			publish(t, r)
			live := publish(t, r)
			stale := sitepublish.DeploymentRecord{ID: "stale-superseded", WorkspaceID: wsID, ProjectID: projectID, Status: sitepublish.DeploymentSuperseded}
			r.deployments.rows[stale.ID] = stale
			r.deployments.seq = append(r.deployments.seq, stale.ID)
			r.hosting.deployments[stale.ID] = []byte("{}")
			if err := cut(r); err != nil {
				t.Fatal(err)
			}
			if got := addressed(r); !slices.Equal(got, []string{live.ID}) {
				t.Fatalf("got %v", got)
			}
			if s, _ := sitepublish.DecodeServing(r.hosting.deployments[live.ID]); !s.Suspended {
				t.Fatal("the live address is suspended")
			}
		})
	}
}

func TestRetireDropsEveryAddressOfADeletedPage(t *testing.T) {
	r := newRig(t)
	first := publish(t, r)
	if err := r.svc.Unpublish(context.Background(), actor, projectID); err != nil {
		t.Fatal(err)
	}
	r.hosting.deployments[first.ID] = []byte("{}")
	if err := r.svc.Retire(context.Background(), wsID, projectID); err != nil {
		t.Fatal(err)
	}
	if len(r.hosting.deployments) != 0 {
		t.Fatalf("got %v", addressed(r))
	}
	if err := r.svc.Retire(context.Background(), "ws-other", projectID); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("another workspace cannot retire the page, got %v", err)
	}
}

func TestAFailedActivationDropsTheNewAddressAndRestoresThePrevious(t *testing.T) {
	r := newRig(t)
	first := publish(t, r)
	r.deployments.activateErr = errors.New("database down")
	if _, err := r.svc.Publish(context.Background(), actor, projectID); err == nil {
		t.Fatal("the publish fails")
	}
	if got := addressed(r); !slices.Equal(got, []string{first.ID}) {
		t.Fatalf("got %v", got)
	}
	if r.hosting.serving(t, liveHost).DeploymentID != first.ID {
		t.Fatal("the hosts point back at the live deployment")
	}
}

func redirecting(r *rig, to string) {
	r.projects.snap.Settings.Redirects = []site.Redirect{{From: "/promo", To: to, Status: 302}}
}

func TestPublishRecordsTheRedirectTargetsAsOutbound(t *testing.T) {
	r := newRig(t)
	redirecting(r, "https://loja.example.net/promo")
	res, err := r.svc.Publish(context.Background(), actor, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(r.deployments.rows[res.Deployment.ID].Outbound, "https://loja.example.net/promo") {
		t.Fatalf("outbound %v", r.deployments.rows[res.Deployment.ID].Outbound)
	}
}

func TestPublishRecordsTheWhatsAppSuccessLinkAsOutbound(t *testing.T) {
	r := newRig(t)
	r.builder.candidate = htmlCandidate(formHTML)
	r.projects.snap.Settings.Forms = map[string]site.FormSettings{"contato": {Success: &site.Success{Kind: site.SuccessWhatsApp, Channel: "num-1"}}}
	res, err := r.svc.Publish(context.Background(), actor, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(r.deployments.rows[res.Deployment.ID].Outbound, "https://vzk.to/num-1") {
		t.Fatalf("outbound %v", r.deployments.rows[res.Deployment.ID].Outbound)
	}
}

func TestPublishHoldsARedirectToATrustedName(t *testing.T) {
	r := newRig(t)
	redirecting(r, "https://itau-seguro.example/login")
	res, err := r.svc.Publish(context.Background(), actor, projectID)
	if err != nil || !res.Held {
		t.Fatalf("got %+v %v", res, err)
	}
	if len(r.hosting.hosts) != 0 || len(r.hosting.deployments) != 0 {
		t.Fatal("a held deployment is never addressed")
	}
}

func TestPublishRefusesAFrameOutsideTheEmbedList(t *testing.T) {
	r := newRig(t)
	r.builder.candidate = htmlCandidate(strings.Replace(homeHTML, "</body>", `<iframe src="https://evil.example/login" title="x"></iframe></body>`, 1))
	_, err := r.svc.Publish(context.Background(), actor, projectID)
	var issues *IssuesFailure
	if !errors.As(err, &issues) || issues.Issues[0].Code != site.CodeFrameSourceForbidden {
		t.Fatalf("got %v", err)
	}
}

func TestPublishChecksTheLiveDOMLikeTheStaticPage(t *testing.T) {
	t.Run("credential field", func(t *testing.T) {
		r := newRig(t)
		r.renderer.live = map[string]string{"/": `<!doctype html><html lang="pt-BR"><head><title>Loja</title></head><body><h1>Bolos</h1><input type="password" name="senha"></body></html>`}
		_, err := r.svc.Publish(context.Background(), actor, projectID)
		var issues *IssuesFailure
		if !errors.Is(err, ErrPublishBlocked) || !errors.As(err, &issues) || issues.Issues[0].Code != site.CodeCredentialFieldForbidden {
			t.Fatalf("got %v", err)
		}
		if len(r.renderer.scans) != 1 || r.renderer.scans[0] != "https://cand.preview.test/" {
			t.Fatalf("the candidate build is scanned: %v", r.renderer.scans)
		}
		if len(r.hosting.hosts) != 0 {
			t.Fatal("nothing goes live")
		}
	})
	t.Run("external form action", func(t *testing.T) {
		r := newRig(t)
		r.renderer.live = map[string]string{"/": `<!doctype html><html lang="pt-BR"><head><title>Loja</title></head><body><h1>Bolos</h1><form action="https://evil.example/collect"><input name="q"></form></body></html>`}
		if _, err := r.svc.Publish(context.Background(), actor, projectID); !errors.Is(err, ErrPublishBlocked) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("impersonation", func(t *testing.T) {
		r := newRig(t)
		r.renderer.live = map[string]string{"/": `<!doctype html><html lang="pt-BR"><head><title>Atualize seu cadastro Itaú</title></head><body><h1>Itaú</h1><input name="lead.phone"></body></html>`}
		res, err := r.svc.Publish(context.Background(), actor, projectID)
		if err != nil || !res.Held {
			t.Fatalf("got %+v %v", res, err)
		}
		if len(r.abuse.rows) != 1 || r.abuse.rows[0].Status != sitepublish.AbuseHeld {
			t.Fatalf("cases %+v", r.abuse.rows)
		}
	})
	t.Run("scan failure", func(t *testing.T) {
		r := newRig(t)
		r.renderer.liveErr = errors.New("renderer down")
		if _, err := r.svc.Publish(context.Background(), actor, projectID); !errors.Is(err, ErrLiveScanFailed) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("missing route", func(t *testing.T) {
		r := newRig(t)
		r.renderer.live = map[string]string{}
		if _, err := r.svc.Publish(context.Background(), actor, projectID); !errors.Is(err, ErrLiveScanFailed) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestPublishWritesTheContentPolicyIntoTheManifest(t *testing.T) {
	r := newRig(t)
	r.projects.snap.Settings.Integrations.GA4 = "G-ABC1234"
	r.projects.snap.Settings.Consent.Banner = true
	dep := publish(t, r)
	manifest, err := sitepublish.DecodeDeploymentManifest(r.hosting.manifests[dep.ID])
	if err != nil {
		t.Fatal(err)
	}
	want := site.ContentPolicyFor([]site.IntegrationKind{site.IntegrationGA4}, "https://api.vozko.test", false)
	if !slices.Equal(manifest.Policy.Script, want.Script) || !slices.Equal(manifest.Policy.Frame, site.EmbedFrames()) {
		t.Fatalf("policy %+v", manifest.Policy)
	}
}

func TestPublishReportsAPublishThatWasNotRecorded(t *testing.T) {
	r := newRig(t)
	r.projects.recordErr = errors.New("database down")
	if _, err := r.svc.Publish(context.Background(), actor, projectID); !errors.Is(err, ErrPublishUnrecorded) {
		t.Fatalf("got %v", err)
	}
	r = newRig(t)
	r.deployments.recordErr = errors.New("database down")
	if _, err := r.svc.Publish(context.Background(), actor, projectID); !errors.Is(err, ErrPublishUnrecorded) {
		t.Fatalf("got %v", err)
	}
}

func TestPublishCopiesOnlyThisWorkspacesPublicMedia(t *testing.T) {
	for name, row := range map[string]*media.Media{
		"another workspace": {ID: "m-2", WorkspaceID: "ws-other", URL: "https://cdn.vozko.test/media/bolo.webp", Type: media.MediaTypeProductImage},
		"private import":    {ID: "m-3", WorkspaceID: wsID, URL: "https://cdn.vozko.test/media/bolo.webp", Type: media.MediaTypeLeadImport},
		"unknown object":    nil,
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			delete(r.media.rows, "https://cdn.vozko.test/media/bolo.webp")
			if row != nil {
				r.media.rows["https://cdn.vozko.test/media/bolo.webp"] = *row
			}
			if _, err := r.svc.Publish(context.Background(), actor, projectID); !errors.Is(err, ErrMediaRefused) {
				t.Fatalf("got %v", err)
			}
			if len(r.hosting.hosts) != 0 {
				t.Fatal("nothing goes live")
			}
		})
	}
}

func TestReservedWordsAreTheBrandAndTheTargetLabel(t *testing.T) {
	got := ReservedWords([]string{"Vozko", " ", "Elo"}, "sites.paginas.test")
	if !slices.Equal(got, []string{"Vozko", "Elo", "sites"}) {
		t.Fatalf("got %v", got)
	}
	r := newRig(t)
	if !slices.Equal(r.svc.reserved, []string{"Vozko", "sites"}) {
		t.Fatalf("the service reserves the brand and the target label: %v", r.svc.reserved)
	}
}
func TestPublicationStatesAreKnown(t *testing.T) {
	for _, p := range []Publication{PublicationLive, PublicationSuspended, PublicationOffline} {
		if !p.Valid() {
			t.Fatalf("%s", p)
		}
	}
	if Publication("archived").Valid() {
		t.Fatal("unknown publication states refuse")
	}
}

func TestPublishRecordsLinksAddedByScriptsAsOutbound(t *testing.T) {
	r := newRig(t)
	r.renderer.live = map[string]string{"/": `<!doctype html><html lang="pt-BR"><head><title>Loja</title></head><body><h1>Bolos</h1><a href="https://loja.example.net/pix">Pague</a></body></html>`}
	res, err := r.svc.Publish(context.Background(), actor, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(r.deployments.rows[res.Deployment.ID].Outbound, "https://loja.example.net/pix") {
		t.Fatalf("outbound %v", r.deployments.rows[res.Deployment.ID].Outbound)
	}
}
