package template_usecase

import (
	"errors"
	"testing"
	"time"

	"vozko/domain/conversation"
	"vozko/domain/whatsapp/template"
)

type accessRefusal struct{ lost bool }

func (r accessRefusal) Error() string           { return "provider refused" }
func (r accessRefusal) AccountAccessLost() bool { return r.lost }

type fakeAccountAccess struct {
	recorded []*time.Time
	err      error
}

func (f *fakeAccountAccess) RecordAccountAccess(_ string, lostAt *time.Time) (bool, error) {
	f.recorded = append(f.recorded, lostAt)
	return true, f.err
}

func syncWith(client *syncTemplatesClientMock, access *fakeAccountAccess) template.SyncTemplatesUseCase {
	uc := NewSyncTemplatesUseCase(newMockTemplateRepo(), &syncTemplatesFactoryMock{client: client, wabaID: "waba-1"}, templateQualityHistory{}, &fakeQualityRecorder{}, access)
	return uc
}

func TestAnAccountWeCanNoLongerReachIsRecordedAsLost(t *testing.T) {
	access := &fakeAccountAccess{}
	uc := syncWith(&syncTemplatesClientMock{err: accessRefusal{lost: true}}, access)

	_, err := uc.Execute(template.SyncTemplatesInput{BusinessPhoneID: "phone-1"})

	if !template.AccountAccessLost(err) {
		t.Fatalf("the sync must still report the refusal, got %v", err)
	}
	if len(access.recorded) != 1 || access.recorded[0] == nil {
		t.Fatalf("recorded = %v, want the moment access was lost", access.recorded)
	}
}

func TestAReachableAccountIsRecordedAsReachable(t *testing.T) {
	access := &fakeAccountAccess{}
	uc := syncWith(&syncTemplatesClientMock{outputs: []*conversation.ListTemplatesOutput{{}}}, access)

	if _, err := uc.Execute(template.SyncTemplatesInput{BusinessPhoneID: "phone-1"}); err != nil {
		t.Fatal(err)
	}

	if len(access.recorded) != 1 || access.recorded[0] != nil {
		t.Fatalf("recorded = %v, want the account reachable again", access.recorded)
	}
}

func TestOtherSyncFailuresSayNothingAboutAccess(t *testing.T) {
	access := &fakeAccountAccess{}
	uc := syncWith(&syncTemplatesClientMock{err: errors.New("timeout")}, access)

	if _, err := uc.Execute(template.SyncTemplatesInput{BusinessPhoneID: "phone-1"}); err == nil {
		t.Fatal("the timeout must reach the caller")
	}

	if len(access.recorded) != 0 {
		t.Fatalf("a timeout recorded access as %v", access.recorded)
	}
}

func TestAFailureToRecordLostAccessReachesTheCaller(t *testing.T) {
	access := &fakeAccountAccess{err: errors.New("database unavailable")}
	uc := syncWith(&syncTemplatesClientMock{err: accessRefusal{lost: true}}, access)

	_, err := uc.Execute(template.SyncTemplatesInput{BusinessPhoneID: "phone-1"})

	if err == nil || !errors.Is(err, access.err) || !template.AccountAccessLost(err) {
		t.Fatalf("err = %v, want both the refusal and the recording failure", err)
	}
}
