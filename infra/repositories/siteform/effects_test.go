package siteform_repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"vozko/domain/site"
	"vozko/domain/sitesubmission"
)

var effectsAt = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

func pendingRun(sub sitesubmission.Submission) sitesubmission.EffectRun {
	plan := sitesubmission.EffectPlan{Settings: site.FormSettings{Notify: true}, Values: sitesubmission.EffectValues{Name: "Ana"}, SourceURL: "https://loja.example/"}
	return sitesubmission.NewEffectRun(sub, plan, []sitesubmission.Effect{sitesubmission.EffectNotice})
}

func TestASubmissionAndItsPendingEffectsAreStoredTogether(t *testing.T) {
	db, mock := mockDB(t)
	sub := sitesubmission.Submission{ID: "s1", WorkspaceID: "w1", FormKey: "k", IdempotencyKey: "idem-1", CreatedAt: effectsAt}
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "site_submissions" .* ON CONFLICT DO NOTHING`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(sqlPattern(insertEffectSQL)).
		WithArgs("s1", "w1", sqlmock.AnyArg(), `["notice"]`, "", "", "", 0, effectsAt, nil, "", effectsAt).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	got, created, err := NewSubmissionStore(db).Create(context.Background(), sub, pendingRun(sub))
	if err != nil || !created || got.ID != "s1" {
		t.Fatalf("got %+v %v %v", got, created, err)
	}
	done(t, mock)
}

func TestASubmissionWithNothingPendingStoresNoEffects(t *testing.T) {
	db, mock := mockDB(t)
	sub := sitesubmission.Submission{ID: "s1", WorkspaceID: "w1", FormKey: "k", IdempotencyKey: "idem-1", CreatedAt: effectsAt}
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "site_submissions" .* ON CONFLICT DO NOTHING`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if _, created, err := NewSubmissionStore(db).Create(context.Background(), sub, sitesubmission.NewEffectRun(sub, sitesubmission.EffectPlan{}, nil)); err != nil || !created {
		t.Fatalf("created %v err %v", created, err)
	}
	done(t, mock)
}

func TestAFailedEffectsInsertRollsTheSubmissionBack(t *testing.T) {
	db, mock := mockDB(t)
	sub := sitesubmission.Submission{ID: "s1", WorkspaceID: "w1", FormKey: "k", IdempotencyKey: "idem-1", CreatedAt: effectsAt}
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "site_submissions" .* ON CONFLICT DO NOTHING`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(sqlPattern(insertEffectSQL)).WillReturnError(errors.New("down"))
	mock.ExpectRollback()
	if _, created, err := NewSubmissionStore(db).Create(context.Background(), sub, pendingRun(sub)); err == nil || created {
		t.Fatalf("a submission without its effects must not be kept: %v %v", created, err)
	}
	done(t, mock)
}

func effectColumns() []string {
	return []string{"submission_id", "workspace_id", "plan", "pending", "owner_id", "entry_id", "entry_type", "attempts", "next_attempt_at", "locked_until", "finished_at", "last_error", "created_at"}
}

func TestClaimingAnEffectRunLocksItAndReadsItsSubmission(t *testing.T) {
	db, mock := mockDB(t)
	lockedUntil := effectsAt.Add(3 * time.Minute)
	mock.ExpectQuery(sqlPattern(claimEffectSQL)).WithArgs(lockedUntil, "s1", effectsAt, effectsAt).
		WillReturnRows(sqlmock.NewRows(effectColumns()).AddRow("s1", "w1", []byte(`{"settings":{"notify":true},"values":{"name":"Ana"},"source_url":"https://loja.example/"}`),
			[]byte(`["owner","notice"]`), "user-7", "", "", 2, effectsAt, lockedUntil, nil, "notices down", effectsAt))
	mock.ExpectQuery(`SELECT \* FROM "site_submissions" WHERE id = \$1 LIMIT \$2`).WithArgs("s1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "workspace_id", "form_key", "form_id", "lead_id", "fields", "utm", "consent", "ip_hash"}).
			AddRow("s1", "w1", "k", "contato", "lead-1", []byte(`{"lead.name":"Ana"}`), []byte(`{}`), []byte(`{}`), "hash"))
	run, err := NewEffectStore(db).Claim(context.Background(), "s1", effectsAt, 3*time.Minute)
	if err != nil || run == nil {
		t.Fatalf("run %+v err %v", run, err)
	}
	if run.WorkspaceID != "w1" || run.OwnerID != "user-7" || run.Attempts != 2 || len(run.Pending) != 2 || run.Pending[0] != sitesubmission.EffectOwner ||
		!run.Plan.Settings.Notify || run.Plan.Values.Name != "Ana" || run.Plan.SourceURL != "https://loja.example/" || run.LastError != "notices down" {
		t.Fatalf("run %+v", run)
	}
	if run.Submission.LeadID != "lead-1" || run.Submission.FormID != "contato" || run.Submission.Fields["lead.name"] != "Ana" || run.Submission.IPHash != "hash" {
		t.Fatalf("submission %+v", run.Submission)
	}
	done(t, mock)
}

func TestARunThatIsLockedDoneOrNotDueIsNotClaimed(t *testing.T) {
	db, mock := mockDB(t)
	mock.ExpectQuery(sqlPattern(claimEffectSQL)).WillReturnRows(sqlmock.NewRows(effectColumns()))
	if run, err := NewEffectStore(db).Claim(context.Background(), "s1", effectsAt, time.Minute); err != nil || run != nil {
		t.Fatalf("run %+v err %v", run, err)
	}
	mock.ExpectQuery(sqlPattern(claimEffectSQL)).WillReturnError(errors.New("down"))
	if run, err := NewEffectStore(db).Claim(context.Background(), "s1", effectsAt, time.Minute); err == nil || run != nil {
		t.Fatalf("a store failure must refuse: %+v %v", run, err)
	}
	mock.ExpectQuery(sqlPattern(claimEffectSQL)).
		WillReturnRows(sqlmock.NewRows(effectColumns()).AddRow("s1", "w1", []byte(`{}`), []byte(`not json`), "", "", "", 0, effectsAt, effectsAt, nil, "", effectsAt))
	if _, err := NewEffectStore(db).Claim(context.Background(), "s1", effectsAt, time.Minute); !errors.Is(err, ErrStoredFormInvalid) {
		t.Fatalf("an unreadable run refuses: %v", err)
	}
	done(t, mock)
}

func TestDueRunsAreListedOldestFirst(t *testing.T) {
	db, mock := mockDB(t)
	mock.ExpectQuery(sqlPattern(dueEffectsSQL)).WithArgs(effectsAt, effectsAt, 50).
		WillReturnRows(sqlmock.NewRows([]string{"submission_id"}).AddRow("s1").AddRow("s2"))
	ids, err := NewEffectStore(db).Due(context.Background(), effectsAt, 50)
	if err != nil || len(ids) != 2 || ids[0] != "s1" {
		t.Fatalf("ids %v err %v", ids, err)
	}
	done(t, mock)
}

func TestProgressAndSettleWriteTheRunState(t *testing.T) {
	db, mock := mockDB(t)
	run := sitesubmission.EffectRun{SubmissionID: "s1", Pending: []sitesubmission.Effect{sitesubmission.EffectTrigger}, OwnerID: "user-7", EntryID: "e1", EntryType: "whatsapp", Attempts: 1, NextAttemptAt: effectsAt, LastError: "x"}
	mock.ExpectExec(sqlPattern(progressEffectSQL)).WithArgs(`["trigger"]`, "user-7", "e1", "whatsapp", "s1").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := NewEffectStore(db).Progress(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	finished := effectsAt.Add(time.Minute)
	run.Pending, run.FinishedAt = nil, &finished
	mock.ExpectExec(sqlPattern(settleEffectSQL)).WithArgs(`[]`, "user-7", "e1", "whatsapp", 1, effectsAt, finished, "x", "s1").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := NewEffectStore(db).Settle(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(sqlPattern(progressEffectSQL)).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := NewEffectStore(db).Progress(context.Background(), run); !errors.Is(err, ErrEffectRunMissing) {
		t.Fatalf("a run that is gone refuses: %v", err)
	}
	done(t, mock)
}
