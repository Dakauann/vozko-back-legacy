package businessphone_usecase

import (
	"errors"
	"log"
	"time"

	"github.com/google/uuid"

	businessphone "vozko/domain/whatsapp/business_phone"
	"vozko/domain/whatsapp/waba"
)

type onboardEmbeddedSignupUseCase struct {
	repo       businessphone.Repository
	wabaRepo   waba.Repository
	store      businessphone.OnboardingStore
	metaClient businessphone.MetaAPIService
	gate       businessphone.ProvisioningGate
}

func NewOnboardEmbeddedSignupUseCase(
	repo businessphone.Repository,
	wabaRepo waba.Repository,
	store businessphone.OnboardingStore,
	metaClient businessphone.MetaAPIService,
	gate businessphone.ProvisioningGate,
) businessphone.OnboardEmbeddedSignupUseCase {
	return &onboardEmbeddedSignupUseCase{
		repo:       repo,
		wabaRepo:   wabaRepo,
		store:      store,
		metaClient: metaClient,
		gate:       gate,
	}
}

// Authorize guards the Meta client-direct flow. The number belongs to the
// client's own WABA, so it consumes no provisioning slot; the workspace phone
// entitlement covers numbers we provision through 360dialog. What must still
// hold is tenancy: a workspace may only onboard a number no one else holds.
func (uc *onboardEmbeddedSignupUseCase) Authorize(workspaceID, metaPhoneNumberID string) error {
	existing, err := uc.repo.FindByMetaPhoneNumberID(metaPhoneNumberID)
	if errors.Is(err, businessphone.ErrPhoneNumberNotFound) || (err == nil && existing == nil) {
		return nil
	}
	if err != nil {
		log.Printf("[onboard-embedded-signup] ownership lookup failed for %s: %v (denying, fail-closed)", metaPhoneNumberID, err)
		return err
	}
	if !existing.BelongsToWorkspace(workspaceID) {
		return businessphone.ErrPhoneHeldByAnotherWorkspace
	}
	return nil
}

func (uc *onboardEmbeddedSignupUseCase) Execute(input businessphone.OnboardEmbeddedSignupInput) (*businessphone.OnboardEmbeddedSignupResult, error) {
	if input.PhoneNumberID == "" {
		return nil, businessphone.ErrMetaPhoneNumberIDRequired
	}
	if input.WABAId == "" {
		return nil, businessphone.ErrInvalidWABAId
	}
	if input.OwnerWorkspaceID == "" {
		return nil, businessphone.ErrOwnerWorkspaceIDRequired
	}
	if input.OwnerAssignedBy == "" {
		return nil, businessphone.ErrOwnerAssignedByRequired
	}
	if input.Provider.Normalized() == businessphone.ProviderDialog360 {
		if input.Dialog360APIKey == "" {
			return nil, businessphone.ErrInvalidAccessToken
		}
	} else if input.AccessToken == "" {
		return nil, businessphone.ErrInvalidAccessToken
	}

	if err := uc.Authorize(input.OwnerWorkspaceID, input.PhoneNumberID); err != nil {
		return nil, err
	}

	log.Printf("[onboard-embedded-signup] Onboarding phone %s for WABA %s (business_id=%s)", input.PhoneNumberID, input.WABAId, input.BusinessID)

	account, accountIsNew, err := uc.account(input)
	if err != nil {
		return nil, err
	}

	var metaPhone *businessphone.MetaPhoneNumberInfo
	if input.Provider.Normalized() != businessphone.ProviderDialog360 {
		metaPhone, err = uc.metaClient.GetPhoneNumber(input.PhoneNumberID, input.AccessToken)
		if err != nil {
			log.Printf("[onboard-embedded-signup] Could not fetch phone details from Meta: %v (will save with minimal data)", err)
		}
	}

	onboarding, err := uc.phone(input, metaPhone)
	if err != nil {
		return nil, err
	}
	onboarding.Account, onboarding.AccountIsNew = account, accountIsNew

	if err := uc.store.Save(onboarding); err != nil {
		log.Printf("[onboard-embedded-signup] Failed to save WABA %s with phone %s: %v", input.WABAId, input.PhoneNumberID, err)
		return nil, err
	}

	log.Printf("[onboard-embedded-signup] Saved phone %s (meta_id=%s, new=%v) with WABA %s", onboarding.Phone.ID, onboarding.Phone.MetaPhoneNumberID, onboarding.PhoneIsNew, input.WABAId)
	return &businessphone.OnboardEmbeddedSignupResult{
		Phone: onboarding.Phone,
		IsNew: onboarding.PhoneIsNew,
	}, nil
}

func (uc *onboardEmbeddedSignupUseCase) NeedsRegistration(metaPhoneNumberID, wabaID string, metaStatus businessphone.Status, coexistence bool) (bool, error) {
	linked, err := uc.repo.FindByMetaPhoneNumberID(metaPhoneNumberID)
	if errors.Is(err, businessphone.ErrPhoneNumberNotFound) {
		return businessphone.RegistrationNeeded(nil, wabaID, metaStatus, coexistence), nil
	}
	if err != nil {
		return false, err
	}
	return businessphone.RegistrationNeeded(linked, wabaID, metaStatus, coexistence), nil
}

func (uc *onboardEmbeddedSignupUseCase) phone(
	input businessphone.OnboardEmbeddedSignupInput,
	metaPhone *businessphone.MetaPhoneNumberInfo,
) (businessphone.Onboarding, error) {
	existing, err := uc.repo.FindByMetaPhoneNumberID(input.PhoneNumberID)
	if err != nil && !errors.Is(err, businessphone.ErrPhoneNumberNotFound) {
		return businessphone.Onboarding{}, err
	}
	if existing != nil {
		return uc.updatedPhone(existing, input, metaPhone, false)
	}

	softDeleted, err := uc.repo.FindByMetaPhoneNumberIDUnscoped(input.PhoneNumberID)
	if err != nil && !errors.Is(err, businessphone.ErrPhoneNumberNotFound) {
		return businessphone.Onboarding{}, err
	}
	if softDeleted != nil {
		return uc.updatedPhone(softDeleted, input, metaPhone, true)
	}

	phone := &businessphone.WhatsAppBusinessPhoneNumber{
		ID:                         uuid.New().String(),
		Provider:                   input.Provider.Normalized(),
		MetaPhoneNumberID:          input.PhoneNumberID,
		WABAId:                     input.WABAId,
		BusinessPortfolioID:        input.BusinessID,
		AccessToken:                input.AccessToken,
		Dialog360ChannelID:         input.Dialog360ChannelID,
		Dialog360APIKey:            input.Dialog360APIKey,
		Status:                     businessphone.StatusConnected,
		WABAName:                   input.WABAName,
		AccountReviewStatus:        input.AccountReviewStatus,
		BusinessVerificationStatus: input.BusinessVerificationStatus,
		OwnershipType:              input.OwnershipType,
		MessagingLimitTier:         input.MessagingLimitTier,
	}

	if metaPhone != nil {
		phone.DisplayPhoneNumber = metaPhone.DisplayPhoneNumber
		phone.VerifiedName = metaPhone.VerifiedName
		phone.Status = businessphone.Status(metaPhone.Status)
		phone.QualityRating = businessphone.QualityRating(metaPhone.QualityRating)
		phone.NameStatus = businessphone.NameStatus(metaPhone.NameStatus)
		phone.CodeVerificationStatus = metaPhone.CodeVerificationStatus
		phone.IsOfficialBusiness = metaPhone.IsOfficialBusiness
	}

	if phone.Status == "" {
		phone.Status = businessphone.StatusConnected
	}

	if err := phone.AssignOwner(input.OwnerWorkspaceID, input.OwnerAssignedBy, time.Now().UTC()); err != nil {
		return businessphone.Onboarding{}, err
	}

	return businessphone.Onboarding{Phone: phone, PhoneIsNew: true}, nil
}

func (uc *onboardEmbeddedSignupUseCase) updatedPhone(
	existing *businessphone.WhatsAppBusinessPhoneNumber,
	input businessphone.OnboardEmbeddedSignupInput,
	metaPhone *businessphone.MetaPhoneNumberInfo,
	restored bool,
) (businessphone.Onboarding, error) {
	existing.WABAId = input.WABAId
	existing.Reconnected()
	existing.BusinessPortfolioID = input.BusinessID
	existing.AccessToken = input.AccessToken
	if input.Provider.Normalized() != businessphone.ProviderMeta {
		existing.Provider = input.Provider.Normalized()
	}
	if input.Dialog360ChannelID != "" {
		existing.Dialog360ChannelID = input.Dialog360ChannelID
	}
	if input.Dialog360APIKey != "" {
		existing.Dialog360APIKey = input.Dialog360APIKey
	}
	existing.WABAName = input.WABAName
	existing.AccountReviewStatus = input.AccountReviewStatus
	existing.BusinessVerificationStatus = input.BusinessVerificationStatus
	existing.OwnershipType = input.OwnershipType
	existing.MessagingLimitTier = input.MessagingLimitTier

	if metaPhone != nil {
		existing.DisplayPhoneNumber = metaPhone.DisplayPhoneNumber
		existing.VerifiedName = metaPhone.VerifiedName
		existing.Status = businessphone.Status(metaPhone.Status)
		existing.QualityRating = businessphone.QualityRating(metaPhone.QualityRating)
		existing.NameStatus = businessphone.NameStatus(metaPhone.NameStatus)
		existing.CodeVerificationStatus = metaPhone.CodeVerificationStatus
		existing.IsOfficialBusiness = metaPhone.IsOfficialBusiness
	}

	if existing.Status == "" {
		existing.Status = businessphone.StatusConnected
	}

	if !existing.BelongsToWorkspace(input.OwnerWorkspaceID) || existing.OwnerAssignedAt == nil || existing.OwnerAssignedBy == "" {
		if err := existing.AssignOwner(input.OwnerWorkspaceID, input.OwnerAssignedBy, time.Now().UTC()); err != nil {
			return businessphone.Onboarding{}, err
		}
	}

	return businessphone.Onboarding{Phone: existing, PhoneRestored: restored}, nil
}

func (uc *onboardEmbeddedSignupUseCase) account(input businessphone.OnboardEmbeddedSignupInput) (*waba.WhatsAppBusinessAccount, bool, error) {
	existing, err := uc.wabaRepo.FindByMetaWABAId(input.WABAId)
	if err != nil && !errors.Is(err, waba.ErrWABANotFound) {
		return nil, false, err
	}
	if existing != nil {
		existing.Name = input.WABAName
		existing.BusinessPortfolioID = input.BusinessID
		existing.AccessToken = input.AccessToken
		existing.AccountReviewStatus = input.AccountReviewStatus
		existing.BusinessVerificationStatus = input.BusinessVerificationStatus
		existing.OwnershipType = input.OwnershipType
		existing.MessagingLimitTier = input.MessagingLimitTier
		if input.Provider.Normalized() != businessphone.ProviderMeta {
			existing.Provider = string(input.Provider.Normalized())
		}
		if input.Dialog360ClientID != "" {
			existing.Dialog360ClientID = input.Dialog360ClientID
		}
		return existing, false, nil
	}

	account, err := waba.NewWhatsAppBusinessAccount(input.WABAId, input.WABAName)
	if err != nil {
		return nil, false, err
	}
	account.Provider = string(input.Provider.Normalized())
	account.BusinessPortfolioID = input.BusinessID
	account.AccessToken = input.AccessToken
	account.Dialog360ClientID = input.Dialog360ClientID
	account.AccountReviewStatus = input.AccountReviewStatus
	account.BusinessVerificationStatus = input.BusinessVerificationStatus
	account.OwnershipType = input.OwnershipType
	account.MessagingLimitTier = input.MessagingLimitTier
	return account, true, nil
}
