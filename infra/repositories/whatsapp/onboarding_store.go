package whatsapp_repository

import (
	"gorm.io/gorm"

	"vozko/domain/cache"
	businessphone "vozko/domain/whatsapp/business_phone"
)

type onboardingStore struct {
	db     *gorm.DB
	shared cache.SharedState
}

func NewOnboardingStore(db *gorm.DB, shared cache.SharedState) businessphone.OnboardingStore {
	return &onboardingStore{db: db, shared: shared}
}

func (s *onboardingStore) Save(onboarding businessphone.Onboarding) error {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := saveOnboardedAccount(tx, onboarding); err != nil {
			return err
		}
		return saveOnboardedPhone(tx, onboarding)
	})
	if err != nil {
		return err
	}
	dropCachedPhone(s.shared, onboarding.Phone.ID, onboarding.Phone.MetaPhoneNumberID)
	return nil
}

func saveOnboardedAccount(tx *gorm.DB, onboarding businessphone.Onboarding) error {
	accounts := NewWABARepository(tx)
	if onboarding.AccountIsNew {
		return accounts.Create(onboarding.Account)
	}
	return accounts.Update(onboarding.Account.ID, onboarding.Account)
}

func saveOnboardedPhone(tx *gorm.DB, onboarding businessphone.Onboarding) error {
	phones := NewBusinessPhoneRepository(tx)
	if onboarding.PhoneIsNew {
		return phones.Create(onboarding.Phone)
	}
	if onboarding.PhoneRestored {
		if err := phones.Restore(onboarding.Phone.ID); err != nil {
			return err
		}
	}
	return phones.Update(onboarding.Phone.ID, onboarding.Phone)
}
