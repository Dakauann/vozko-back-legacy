package instagram

import (
	"errors"
	"testing"

	"vozko/domain/shared/deps"
)

func TestModerationRequiresTheCommentAnalysis(t *testing.T) {
	uc, err := NewModerateCommentUseCase(accountRepoFor(connectedAccount()), &fakeCommentService{}, &fakeCommentRepo{}, nil)
	if uc != nil || !errors.Is(err, deps.ErrMissing) {
		t.Fatalf("uc = %v err = %v", uc, err)
	}
}

func TestPrivateRepliesRequireTheMessageHistory(t *testing.T) {
	uc, err := NewSendPrivateReplyUseCase(accountRepoFor(connectedAccount()), &fakeMessagingService{}, &fakeCommentService{}, &fakeCommentRepo{}, nil, &fakeContactRepo{}, &fakeConversationRepo{}, nil)
	var missing *deps.MissingError
	if uc != nil || !errors.As(err, &missing) || len(missing.Missing) != 2 {
		t.Fatalf("uc = %v err = %v, want the sender and history refused", uc, err)
	}
}
