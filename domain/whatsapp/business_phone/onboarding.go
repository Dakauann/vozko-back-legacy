package businessphone

import "vozko/domain/whatsapp/waba"

type Onboarding struct {
	Account       *waba.WhatsAppBusinessAccount
	AccountIsNew  bool
	Phone         *WhatsAppBusinessPhoneNumber
	PhoneIsNew    bool
	PhoneRestored bool
}

type OnboardingStore interface {
	Save(onboarding Onboarding) error
}
