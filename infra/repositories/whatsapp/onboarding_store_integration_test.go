package whatsapp_repository

import (
	"testing"
	"time"

	"github.com/google/uuid"

	businessphone "vozko/domain/whatsapp/business_phone"
	"vozko/domain/whatsapp/waba"
	"vozko/infra/database/schema"
	"vozko/infra/repositories/repotest"
)

func onboardingAccount(t *testing.T, metaWABAId string) *waba.WhatsAppBusinessAccount {
	t.Helper()
	account, err := waba.NewWhatsAppBusinessAccount(metaWABAId, "Conta "+metaWABAId)
	if err != nil {
		t.Fatal(err)
	}
	return account
}

func onboardingPhone(t *testing.T, metaPhoneID, wabaID string) *businessphone.WhatsAppBusinessPhoneNumber {
	t.Helper()
	phone := &businessphone.WhatsAppBusinessPhoneNumber{
		ID: uuid.NewString(), MetaPhoneNumberID: metaPhoneID, WABAId: wabaID,
		Provider: businessphone.ProviderMeta, Status: businessphone.StatusConnected, AccessToken: "token",
	}
	if err := phone.AssignOwner(uuid.NewString(), uuid.NewString(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	return phone
}

func TestOnboardingSavesTheAccountAndRelinksTheNumberTogetherOnPostgres(t *testing.T) {
	db := repotest.IsolatedDB(t, "onboarding", &schema.WhatsAppBusinessAccount{}, &schema.WhatsAppBusinessPhoneNumber{})
	phones := NewBusinessPhoneRepository(db)
	moving := onboardingPhone(t, "meta-1", "waba-old")
	if err := phones.Create(moving); err != nil {
		t.Fatal(err)
	}
	moving.WABAId = "waba-new"

	err := NewOnboardingStore(db, nil).Save(businessphone.Onboarding{
		Account: onboardingAccount(t, "waba-new"), AccountIsNew: true, Phone: moving,
	})

	if err != nil {
		t.Fatal(err)
	}
	saved, err := phones.FindByMetaPhoneNumberID("meta-1")
	if err != nil || saved.WABAId != "waba-new" {
		t.Fatalf("phone = %+v, %v", saved, err)
	}
	if _, err := NewWABARepository(db).FindByMetaWABAId("waba-new"); err != nil {
		t.Fatalf("account not saved: %v", err)
	}
}

func TestOnboardingKeepsNoAccountWhenTheNumberCannotBeSavedOnPostgres(t *testing.T) {
	db := repotest.IsolatedDB(t, "onboarding", &schema.WhatsAppBusinessAccount{}, &schema.WhatsAppBusinessPhoneNumber{})
	if err := NewBusinessPhoneRepository(db).Create(onboardingPhone(t, "meta-1", "waba-old")); err != nil {
		t.Fatal(err)
	}

	err := NewOnboardingStore(db, nil).Save(businessphone.Onboarding{
		Account: onboardingAccount(t, "waba-new"), AccountIsNew: true,
		Phone: onboardingPhone(t, "meta-1", "waba-new"), PhoneIsNew: true,
	})

	if err == nil {
		t.Fatal("a duplicate number must fail the onboarding")
	}
	if _, err := NewWABARepository(db).FindByMetaWABAId("waba-new"); err != waba.ErrWABANotFound {
		t.Fatalf("the account outlived the failed onboarding: %v", err)
	}
}
