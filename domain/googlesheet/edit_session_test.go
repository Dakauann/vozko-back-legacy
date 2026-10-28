package googlesheet

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var editStart = time.Date(2026, 10, 27, 14, 2, 0, 0, time.UTC)

func startedSession(t *testing.T) *EditSession {
	t.Helper()
	s, err := NewEditSession("ws-1", "sp-1", "user-1", editStart)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestANewEditSessionHoldsTheFileForOneLeaseFromItsStart(t *testing.T) {
	s := startedSession(t)
	if !s.ExpiresAt.Equal(editStart.Add(EditSessionTTL)) || s.Settled() || s.Ended() {
		t.Fatalf("session = %+v", s)
	}
	cases := []struct {
		name  string
		at    time.Time
		holds bool
	}{
		{"at the start", editStart, true},
		{"just before the lease runs out", editStart.Add(EditSessionTTL - time.Second), true},
		{"when the lease runs out", editStart.Add(EditSessionTTL), false},
		{"long after", editStart.Add(time.Hour), false},
	}
	for _, c := range cases {
		if got := s.HoldsAt(c.at); got != c.holds {
			t.Errorf("%s: HoldsAt = %v, want %v", c.name, got, c.holds)
		}
	}
}

func TestAnEditSessionNeedsTheWorkspaceTheFileAndThePerson(t *testing.T) {
	for _, in := range [][3]string{{"", "sp", "u"}, {"ws", "", "u"}, {"ws", "sp", ""}, {"ws", " ", "u"}} {
		if _, err := NewEditSession(in[0], in[1], in[2], editStart); err == nil {
			t.Errorf("NewEditSession(%q) accepted a session without an owner", in)
		}
	}
}

func TestAHeartbeatExtendsTheLeaseButNeverPastTheMaximumLength(t *testing.T) {
	s := startedSession(t)
	beat := editStart.Add(30 * time.Second)
	if err := s.Extend(beat); err != nil || !s.ExpiresAt.Equal(beat.Add(EditSessionTTL)) {
		t.Fatalf("extend = %v, expires %v", err, s.ExpiresAt)
	}
	late := editStart.Add(EditSessionMaxLength - 30*time.Second)
	for at := beat; at.Before(late); at = at.Add(time.Minute) {
		if err := s.Extend(at); err != nil {
			t.Fatalf("extend at %v: %v", at, err)
		}
	}
	if err := s.Extend(late); err != nil || !s.ExpiresAt.Equal(s.Deadline()) {
		t.Fatalf("near the end the lease stops at the deadline: err=%v expires=%v deadline=%v", err, s.ExpiresAt, s.Deadline())
	}
}

func TestAnExpiredOrEndedSessionIsNeverRevived(t *testing.T) {
	expired := startedSession(t)
	if err := expired.Extend(editStart.Add(EditSessionTTL)); !errors.Is(err, ErrEditSessionEnded) {
		t.Fatalf("an expired session was extended: %v", err)
	}
	ended := startedSession(t)
	if err := ended.End(EditEnd{Reason: EditFinished, By: "user-1", At: editStart.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := ended.Extend(editStart.Add(90 * time.Second)); !errors.Is(err, ErrEditSessionEnded) {
		t.Fatalf("an ended session was extended: %v", err)
	}
	if ended.HoldsAt(editStart.Add(90 * time.Second)) {
		t.Fatal("an ended session still holds the file")
	}
}

func TestOnlyTheHolderFinishesOrLeavesAndOnlyAnotherPersonEndsItAsAdmin(t *testing.T) {
	at := editStart.Add(time.Minute)
	cases := []struct {
		name string
		end  EditEnd
		want error
	}{
		{"the holder finishes", EditEnd{Reason: EditFinished, By: "user-1", At: at}, nil},
		{"the holder leaves the page", EditEnd{Reason: EditLeft, By: "user-1", At: at}, nil},
		{"someone else cannot finish it", EditEnd{Reason: EditFinished, By: "user-2", At: at}, ErrEditSessionNotYours},
		{"an admin ends someone else's", EditEnd{Reason: EditEndedByAdmin, By: "user-2", At: at}, nil},
		{"an admin end needs the admin", EditEnd{Reason: EditEndedByAdmin, At: at}, ErrEditSessionNotYours},
		{"the system expires it", EditEnd{Reason: EditExpired, At: at}, nil},
		{"an unknown reason", EditEnd{Reason: "bored", By: "user-1", At: at}, ErrEditEndReasonUnknown},
	}
	for _, c := range cases {
		s := startedSession(t)
		err := s.End(c.end)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
			continue
		}
		if err == nil && (!s.Ended() || s.EndReason != c.end.Reason || s.EndedBy != c.end.By || !s.EndedAt.Equal(at)) {
			t.Errorf("%s: session = %+v", c.name, s)
		}
		if err != nil && s.Ended() {
			t.Errorf("%s: a refused end still ended the session", c.name)
		}
	}
}

func TestEndingTwiceKeepsTheFirstEnd(t *testing.T) {
	s := startedSession(t)
	first := editStart.Add(time.Minute)
	if err := s.End(EditEnd{Reason: EditFinished, By: "user-1", At: first}); err != nil {
		t.Fatal(err)
	}
	if err := s.End(EditEnd{Reason: EditLeft, By: "user-1", At: first.Add(time.Second)}); !errors.Is(err, ErrEditSessionEnded) {
		t.Fatalf("second end = %v", err)
	}
	if s.EndReason != EditFinished || !s.EndedAt.Equal(first) {
		t.Fatalf("the first end was overwritten: %+v", s)
	}
}

func TestTheClientMayOnlyAskToFinishOrToLeave(t *testing.T) {
	for _, ok := range []string{"finished", "left"} {
		if _, err := ParseClientEndReason(ok); err != nil {
			t.Errorf("%q refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "expired", "ended_by_admin", "FINISHED"} {
		if _, err := ParseClientEndReason(bad); !errors.Is(err, ErrEditEndReasonUnknown) {
			t.Errorf("%q accepted: %v", bad, err)
		}
	}
}

func TestTheTakenErrorNamesTheHolderAndUnwrapsToBeingEdited(t *testing.T) {
	s := startedSession(t)
	err := error(&EditTakenError{Session: s})
	if !errors.Is(err, ErrFileBeingEdited) || ErrorCode(err) != CodeFileBeingEdited {
		t.Fatalf("err = %v code = %s", err, ErrorCode(err))
	}
	if !strings.Contains(err.Error(), "user-1") {
		t.Fatalf("message %q does not name the holder", err.Error())
	}
}

func TestTheEmbeddedEditorUsesTheOneMinimalModeAndTheTab(t *testing.T) {
	s := &Sheet{SpreadsheetID: "abc123", TabID: 42}
	if got := s.EmbedURL(); got != "https://docs.google.com/spreadsheets/d/abc123/edit?"+EmbeddedEditorMode+"#gid=42" {
		t.Fatalf("EmbedURL = %s", got)
	}
	if EmbeddedEditorMode != "rm=minimal" {
		t.Fatalf("mode = %s", EmbeddedEditorMode)
	}
	if s.URL() != "https://docs.google.com/spreadsheets/d/abc123/edit#gid=42" {
		t.Fatalf("the full editor link changed: %s", s.URL())
	}
}

func TestRefWarningsListCellsThatAreNotARefOrRepeatOne(t *testing.T) {
	a, b := "0b8f5f3e-4a63-4c1e-9b0e-2f1d6f5d1a11", "7c1e2d4f-1b2a-4c3d-8e9f-0a1b2c3d4e5f"
	snapshot := TabSnapshot{Refs: [][]any{
		{RefHeader},
		{a},
		{},
		{"anotação"},
		{b},
		{a},
		{" "},
		{12.0},
	}}
	got := snapshot.RefWarnings()
	want := []int{4, 6, 8}
	if len(got) != len(want) {
		t.Fatalf("RefWarnings = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("RefWarnings = %v, want %v", got, want)
		}
	}
	if clean := (TabSnapshot{Refs: [][]any{{RefHeader}, {a}, {}, {b}}}).RefWarnings(); len(clean) != 0 {
		t.Fatalf("a clean column warned: %v", clean)
	}
}

func TestTheCheckIsBrokenWhenATabNeedsAttentionAndUncheckedWhenGoogleCouldNotBeRead(t *testing.T) {
	fine := TabCheck{SheetID: "s1", TabName: "Pagamentos"}
	renamed := TabCheck{SheetID: "s2", TabName: "Pagos", RenamedFrom: "Pagamentos"}
	warned := TabCheck{SheetID: "s3", TabName: "Negociações", RefWarnings: []int{7}}
	broken := TabCheck{SheetID: "s4", TabName: "Leads", Reason: ReasonHeaderMismatch, Detail: "coluna C"}
	cases := []struct {
		name  string
		check EditCheck
		want  CheckStatus
	}{
		{"all fine", NewEditCheck([]TabCheck{fine, renamed, warned}), CheckOK},
		{"one broken", NewEditCheck([]TabCheck{fine, broken}), CheckBroken},
		{"google unreadable", UncheckedEdit("o Google não respondeu"), CheckUnchecked},
		{"a tab unreadable", NewEditCheck([]TabCheck{fine, {SheetID: "s5", Unchecked: "limite"}}), CheckUnchecked},
		{"broken wins over unreadable", NewEditCheck([]TabCheck{broken, {SheetID: "s5", Unchecked: "limite"}}), CheckBroken},
	}
	for _, c := range cases {
		if c.check.Status != c.want {
			t.Errorf("%s: status = %s, want %s", c.name, c.check.Status, c.want)
		}
	}
	if !broken.Repairable() || fine.Repairable() || (TabCheck{Reason: ReasonSpreadsheetGone}).Repairable() {
		t.Fatal("only header reasons are repairable")
	}
}
