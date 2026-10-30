package template

import (
	"errors"
	"fmt"
	"testing"
)

type providerRefusal struct{ lost bool }

func (r providerRefusal) Error() string           { return "provider refused" }
func (r providerRefusal) AccountAccessLost() bool { return r.lost }

func TestOnlyAnErrorThatSaysAccessWasLostCountsAsLostAccess(t *testing.T) {
	if !AccountAccessLost(fmt.Errorf("list templates: %w", providerRefusal{lost: true})) {
		t.Fatal("a wrapped loss of access must be recognised")
	}
	for _, err := range []error{nil, errors.New("timeout"), providerRefusal{lost: false}} {
		if AccountAccessLost(err) {
			t.Fatalf("%v is not a loss of access", err)
		}
	}
}
