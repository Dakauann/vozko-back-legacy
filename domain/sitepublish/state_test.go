package sitepublish

import (
	"errors"
	"testing"
)

func TestDeploymentTransitions(t *testing.T) {
	allowed := map[DeploymentStatus][]DeploymentStatus{
		DeploymentBuilding:   {DeploymentReady, DeploymentFailed},
		DeploymentReady:      {DeploymentActive},
		DeploymentActive:     {DeploymentSuperseded},
		DeploymentSuperseded: {DeploymentActive},
		DeploymentFailed:     {},
	}
	statuses := DeploymentStatuses()
	if len(statuses) != 5 {
		t.Fatalf("got %v", statuses)
	}
	for _, from := range statuses {
		for _, to := range statuses {
			want := false
			for _, ok := range allowed[from] {
				want = want || ok == to
			}
			err := CheckDeploymentTransition(from, to)
			if (err == nil) != want {
				t.Fatalf("%s to %s: got %v", from, to, err)
			}
			if err != nil && !errors.Is(err, ErrDeploymentTransition) {
				t.Fatalf("got %v", err)
			}
		}
	}
	if CheckDeploymentTransition("draft", DeploymentReady) == nil || DeploymentStatus("x").Valid() {
		t.Fatal("unknown statuses refuse")
	}
}

func TestDeploymentMovesThroughItsLife(t *testing.T) {
	d := newDeployment("dep_1", "prj_1", "rev_1")
	if d.Status != DeploymentBuilding || d.Activatable() {
		t.Fatalf("got %+v", d)
	}
	if err := d.Transition(DeploymentActive); !errors.Is(err, ErrDeploymentTransition) || d.Status != DeploymentBuilding {
		t.Fatalf("a building deployment never goes live: %v", err)
	}
	for _, step := range []DeploymentStatus{DeploymentReady, DeploymentActive, DeploymentSuperseded} {
		if err := d.Transition(step); err != nil {
			t.Fatal(err)
		}
	}
	if !d.Activatable() {
		t.Fatal("a superseded deployment is a rollback target")
	}
}

func TestHostnameTransitionsDependOnTheKind(t *testing.T) {
	custom := map[HostnameStatus][]HostnameStatus{
		HostnamePendingDNS: {HostnamePendingSSL, HostnameFailed, HostnameRemoving, HostnameSuspended},
		HostnamePendingSSL: {HostnameActive, HostnameFailed, HostnameRemoving, HostnameSuspended},
		HostnameActive:     {HostnameFailed, HostnameRemoving, HostnameSuspended},
		HostnameFailed:     {HostnamePendingDNS, HostnamePendingSSL, HostnameRemoving, HostnameSuspended},
		HostnameSuspended:  {HostnamePendingDNS, HostnamePendingSSL, HostnameRemoving},
		HostnameRemoving:   {},
	}
	platform := map[HostnameStatus][]HostnameStatus{
		HostnameActive: {HostnameRemoving},
	}
	for kind, table := range map[HostnameKind]map[HostnameStatus][]HostnameStatus{HostnameCustom: custom, HostnamePlatform: platform} {
		for _, from := range HostnameStatuses() {
			for _, to := range HostnameStatuses() {
				want := false
				for _, ok := range table[from] {
					want = want || ok == to
				}
				err := CheckHostnameTransition(kind, from, to)
				if (err == nil) != want {
					t.Fatalf("%s %s to %s: got %v", kind, from, to, err)
				}
			}
		}
	}
	if InitialHostnameStatus(HostnamePlatform) != HostnameActive || InitialHostnameStatus(HostnameCustom) != HostnamePendingDNS {
		t.Fatal("initial statuses")
	}
	if CheckHostnameTransition("subdomain", HostnameActive, HostnameRemoving) == nil {
		t.Fatal("unknown kind refuses")
	}
}

func TestOnlyActiveHostnamesServe(t *testing.T) {
	cases := map[HostnameStatus]error{
		HostnameActive:     nil,
		HostnamePendingDNS: ErrHostnameNotVerified,
		HostnamePendingSSL: ErrSSLPending,
		HostnameRemoving:   ErrHostnameUnavailable,
		HostnameFailed:     ErrHostnameUnavailable,
		HostnameSuspended:  ErrHostnameUnavailable,
	}
	for status, want := range cases {
		if err := status.serving(); !errors.Is(err, want) || (want == nil && err != nil) {
			t.Fatalf("%s: got %v", status, err)
		}
	}
}

func TestErrorCodes(t *testing.T) {
	cases := map[error]string{
		ErrDeploymentTransition: "deployment_transition_invalid",
		ErrHostnameTransition:   "hostname_transition_invalid",
		ErrHostnameInvalid:      "hostname_invalid",
		ErrHostnameForbidden:    "hostname_forbidden",
		ErrHostnameNotVerified:  "hostname_not_verified",
		ErrSSLPending:           "ssl_pending",
		ErrHostnameUnavailable:  "hostname_unavailable",
		ErrSlugInvalid:          "slug_invalid",
		ErrSlugReserved:         "slug_reserved",
		ErrServingInvalid:       "serving_invalid",
	}
	for err, code := range cases {
		if ErrorCode(err) != code {
			t.Fatalf("%v: got %q", err, ErrorCode(err))
		}
	}
	if ErrorCode(nil) != "" || ErrorCode(errors.New("x")) != "" {
		t.Fatal("fallback")
	}
}

func TestHostnameRecordsMoveByTheSameRules(t *testing.T) {
	rec := HostnameRecord{Kind: HostnameCustom, Status: HostnamePendingDNS}
	if err := rec.Transition(HostnameActive); !errors.Is(err, ErrHostnameTransition) || rec.Status != HostnamePendingDNS {
		t.Fatalf("got %v %s", err, rec.Status)
	}
	if err := rec.Transition(HostnamePendingSSL); err != nil || rec.Status != HostnamePendingSSL {
		t.Fatalf("got %v %s", err, rec.Status)
	}
}

func TestOnlyServingAndCertifyingDomainsMayHoldAProviderRecord(t *testing.T) {
	want := map[HostnameStatus]bool{HostnamePendingDNS: true, HostnameFailed: true, HostnameSuspended: true}
	for _, status := range HostnameStatuses() {
		if status.HoldsNoProviderRecord() != want[status] {
			t.Fatalf("%s: holds no provider record = %v", status, status.HoldsNoProviderRecord())
		}
	}
	listed := ProviderlessStatuses()
	if len(listed) != len(want) {
		t.Fatalf("got %v", listed)
	}
	for _, status := range listed {
		if !want[status] {
			t.Fatalf("%s is listed", status)
		}
	}
	listed[0] = HostnameActive
	if ProviderlessStatuses()[0] == HostnameActive {
		t.Fatal("the list is a copy")
	}
	if HostnameStatus("x").HoldsNoProviderRecord() {
		t.Fatal("an unknown status is never treated as safe to clean")
	}
}
