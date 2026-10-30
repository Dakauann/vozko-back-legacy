package whatsapp_repository

import (
	"testing"
	"time"

	businessphone "vozko/domain/whatsapp/business_phone"
	"vozko/infra/database/schema"
	"vozko/infra/repositories/repotest"
)

func TestANumberRemembersLosingItsAccountUntilItIsReachableAgainOnPostgres(t *testing.T) {
	db := repotest.IsolatedDB(t, "accountaccess", &schema.WhatsAppBusinessPhoneNumber{})
	phones := NewBusinessPhoneRepository(db)
	phone := onboardingPhone(t, "meta-1", "waba-1")
	if err := phones.Create(phone); err != nil {
		t.Fatal(err)
	}
	recorder := phones.(businessphone.AccountAccessRecorder)
	lostAt := time.Date(2026, 10, 29, 18, 0, 0, 0, time.UTC)

	if _, err := recorder.RecordAccountAccess(phone.ID, &lostAt); err != nil {
		t.Fatal(err)
	}
	later := lostAt.Add(time.Hour)
	if _, err := recorder.RecordAccountAccess(phone.ID, &later); err != nil {
		t.Fatal(err)
	}
	lost, err := phones.FindByID(phone.ID)
	if err != nil || !lost.NeedsReconnect() || !lost.AccountAccessLostAt.Equal(lostAt) {
		t.Fatalf("after losing access: %+v, %v", lost.AccountAccessLostAt, err)
	}

	if _, err := recorder.RecordAccountAccess(phone.ID, nil); err != nil {
		t.Fatal(err)
	}
	reachable, err := phones.FindByID(phone.ID)
	if err != nil || reachable.NeedsReconnect() {
		t.Fatalf("after reaching it again: %+v, %v", reachable.AccountAccessLostAt, err)
	}
}

func TestReconnectingANumberClearsItsLostAccessOnPostgres(t *testing.T) {
	db := repotest.IsolatedDB(t, "accountaccess", &schema.WhatsAppBusinessAccount{}, &schema.WhatsAppBusinessPhoneNumber{})
	phones := NewBusinessPhoneRepository(db)
	phone := onboardingPhone(t, "meta-1", "waba-1")
	lostAt := time.Date(2026, 10, 29, 18, 0, 0, 0, time.UTC)
	phone.AccountAccessLostAt = &lostAt
	if err := phones.Create(phone); err != nil {
		t.Fatal(err)
	}
	phone.AccountAccessLostAt = nil

	if err := phones.Update(phone.ID, phone); err != nil {
		t.Fatal(err)
	}

	saved, err := phones.FindByID(phone.ID)
	if err != nil || saved.NeedsReconnect() {
		t.Fatalf("reconnected number still asks to reconnect: %+v, %v", saved.AccountAccessLostAt, err)
	}
}
