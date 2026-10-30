package businessphone_usecase

import (
	"errors"
	"testing"
	"time"

	businessphone "vozko/domain/whatsapp/business_phone"
)

type mockOnboardingStore struct {
	phones *mockRepository
	wabas  *mockWABARepository
	err    error
	saved  []businessphone.Onboarding
}

func (s *mockOnboardingStore) Save(onboarding businessphone.Onboarding) error {
	if s.err != nil {
		return s.err
	}
	s.saved = append(s.saved, onboarding)
	s.wabas.accounts[onboarding.Account.ID] = onboarding.Account
	if onboarding.PhoneRestored {
		delete(s.phones.deletedPhoneNumbers, onboarding.Phone.ID)
	}
	s.phones.phoneNumbers[onboarding.Phone.ID] = onboarding.Phone
	return nil
}

func newOnboarding(repo *mockRepository, wabaRepo *mockWABARepository, gate businessphone.ProvisioningGate) (businessphone.OnboardEmbeddedSignupUseCase, *mockOnboardingStore) {
	store := &mockOnboardingStore{phones: repo, wabas: wabaRepo}
	return NewOnboardEmbeddedSignupUseCase(repo, wabaRepo, store, newMockMetaAPI(), gate), store
}

func TestAReconnectionMovesTheNumberToTheAccountJustConnected(t *testing.T) {
	repo := newMockRepo()
	moving := ownedPhone("phone-1", "meta-1", "ws-1", businessphone.StatusConnected)
	moving.WABAId = "waba-old"
	repo.phoneNumbers[moving.ID] = moving
	uc, store := newOnboarding(repo, newMockWABARepo(), &stubProvisioningGate{})

	input := quotaInput("ws-1", "meta-1")
	input.WABAId = "waba-new"
	if _, err := uc.Execute(input); err != nil {
		t.Fatal(err)
	}

	if len(store.saved) != 1 {
		t.Fatalf("saved %d onboardings, want the account and number together once", len(store.saved))
	}
	saved := store.saved[0]
	if saved.Account.MetaWABAId != "waba-new" || !saved.AccountIsNew || saved.Phone.WABAId != "waba-new" || saved.PhoneIsNew {
		t.Fatalf("saved = %+v / %+v", saved.Account, saved.Phone)
	}
}

func TestAFailedSaveLeavesNeitherTheAccountNorTheNumberBehind(t *testing.T) {
	repo, wabaRepo := newMockRepo(), newMockWABARepo()
	uc, store := newOnboarding(repo, wabaRepo, &stubProvisioningGate{})
	store.err = errors.New("database unavailable")

	_, err := uc.Execute(quotaInput("ws-1", "meta-1"))

	if err == nil {
		t.Fatal("a failed save must reach the caller")
	}
	if len(wabaRepo.accounts) != 0 || len(repo.phoneNumbers) != 0 {
		t.Fatalf("left accounts=%d phones=%d behind", len(wabaRepo.accounts), len(repo.phoneNumbers))
	}
}

func TestAnAccountThatCannotBeReadStopsTheOnboarding(t *testing.T) {
	wabaRepo := newMockWABARepo()
	wabaRepo.findErr = errors.New("database unavailable")
	uc, store := newOnboarding(newMockRepo(), wabaRepo, &stubProvisioningGate{})

	if _, err := uc.Execute(quotaInput("ws-1", "meta-1")); err == nil || len(store.saved) != 0 {
		t.Fatalf("err = %v, saved = %d", err, len(store.saved))
	}
}

func TestRegistrationFollowsWhereOurRecordsLinkTheNumber(t *testing.T) {
	repo := newMockRepo()
	linked := ownedPhone("phone-1", "meta-1", "ws-1", businessphone.StatusConnected)
	linked.WABAId = "waba-old"
	repo.phoneNumbers[linked.ID] = linked
	uc, _ := newOnboarding(repo, newMockWABARepo(), &stubProvisioningGate{})

	moving, err := uc.NeedsRegistration("meta-1", "waba-new", businessphone.StatusConnected, false)
	if err != nil || !moving {
		t.Fatalf("a number moving accounts needs registration, got %v, %v", moving, err)
	}
	staying, err := uc.NeedsRegistration("meta-1", "waba-old", businessphone.StatusConnected, false)
	if err != nil || staying {
		t.Fatalf("a number already in this account needs none, got %v, %v", staying, err)
	}
	fresh, err := uc.NeedsRegistration("meta-unknown", "waba-new", businessphone.StatusConnected, false)
	if err != nil || fresh {
		t.Fatalf("a connected number new to us needs none, got %v, %v", fresh, err)
	}
}

func TestRegistrationIsNotDecidedWhenTheNumberCannotBeRead(t *testing.T) {
	repo := newMockRepo()
	repo.findErr = errors.New("database unavailable")
	uc, _ := newOnboarding(repo, newMockWABARepo(), &stubProvisioningGate{})

	if _, err := uc.NeedsRegistration("meta-1", "waba-new", businessphone.StatusConnected, false); err == nil {
		t.Fatal("an unreadable number must not be guessed")
	}
}

func TestReconnectingANumberStopsAskingForAReconnection(t *testing.T) {
	repo := newMockRepo()
	lost := ownedPhone("phone-1", "meta-1", "ws-1", businessphone.StatusConnected)
	lostAt := time.Now().UTC()
	lost.AccountAccessLostAt = &lostAt
	repo.phoneNumbers[lost.ID] = lost
	uc, store := newOnboarding(repo, newMockWABARepo(), &stubProvisioningGate{})

	if _, err := uc.Execute(quotaInput("ws-1", "meta-1")); err != nil {
		t.Fatal(err)
	}

	if store.saved[0].Phone.NeedsReconnect() {
		t.Fatal("a reconnected number still asks to reconnect")
	}
}
