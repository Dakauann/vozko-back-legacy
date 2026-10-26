package unofficial_whatsapp

import (
	"errors"
	"testing"

	"vozko/domain/shared/deps"
)

func TestNewStartConversationUseCaseNamesEveryMissingDependency(t *testing.T) {
	_, err := NewStartConversationUseCase(StartConversationDeps{})

	var missing *deps.MissingError
	if !errors.As(err, &missing) {
		t.Fatalf("err = %v, want the missing dependencies named", err)
	}
	want := []string{"contact repository", "conversation repository", "instance repository", "messaging api", "server repository", "starter rule"}
	if len(missing.Missing) != len(want) {
		t.Fatalf("missing = %v, want %v", missing.Missing, want)
	}
	for i, name := range want {
		if missing.Missing[i] != name {
			t.Fatalf("missing = %v, want %v", missing.Missing, want)
		}
	}
}
