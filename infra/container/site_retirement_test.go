package container

import (
	"errors"
	"fmt"
	"testing"

	site_usecase "vozko/usecases/site"
	sitepublish_usecase "vozko/usecases/sitepublish"
)

func TestARetirementBlockedByABusyDomainIsAPageConflict(t *testing.T) {
	if err := retirementError(fmt.Errorf("remove: %w", sitepublish_usecase.ErrDomainBusy)); !errors.Is(err, site_usecase.ErrProjectDomainsBusy) {
		t.Fatalf("got %v", err)
	}
	other := errors.New("db down")
	if err := retirementError(other); !errors.Is(err, other) || errors.Is(err, site_usecase.ErrProjectDomainsBusy) {
		t.Fatalf("got %v", err)
	}
	if retirementError(nil) != nil {
		t.Fatal("no error stays no error")
	}
}
