package container

import (
	"log"
	"context"

	export_domain "vozko/domain/export"
	facebook_repository "vozko/infra/repositories/facebook"
	instagram_repository "vozko/infra/repositories/instagram"
	mercadolivre_repository "vozko/infra/repositories/mercadolivre"
	telegram_repository "vozko/infra/repositories/telegram"
	unofficial_whatsapp_repository "vozko/infra/repositories/unofficial_whatsapp"
	webchat_repository "vozko/infra/repositories/webchat"
	wc_entry_repository "vozko/infra/repositories/whatsapp_campaign_entry"
	conversation_usecase "vozko/usecases/conversation"
	export_usecase "vozko/usecases/export"
)

type contactIdentityFuncs struct {
	byIDs            func(ctx context.Context, ids []string) (map[string]conversation_usecase.ContactDisplay, error)
	forConversation  func(ctx context.Context, conversationID string) (conversation_usecase.ContactDisplay, string, error)
	authorsByHandle  func(ctx context.Context, entryID string, handles []string) (map[string]conversation_usecase.ContactDisplay, error)
	authorsInAccount func(ctx context.Context, accountID string, handles []string) (map[string]conversation_usecase.ContactDisplay, error)
}

func (f contactIdentityFuncs) ContactsByIDs(ctx context.Context, ids []string) (map[string]conversation_usecase.ContactDisplay, error) {
	if f.byIDs == nil {
		return nil, nil
	}
	return f.byIDs(ctx, ids)
}

func (f contactIdentityFuncs) ContactForConversation(ctx context.Context, conversationID string) (conversation_usecase.ContactDisplay, string, error) {
	if f.forConversation == nil {
		return conversation_usecase.ContactDisplay{}, "", nil
	}
	return f.forConversation(ctx, conversationID)
}

func (f contactIdentityFuncs) AuthorsByHandle(ctx context.Context, entryID string, handles []string) (map[string]conversation_usecase.ContactDisplay, error) {
	if f.authorsByHandle == nil {
		return nil, nil
	}
	return f.authorsByHandle(ctx, entryID, handles)
}

func (f contactIdentityFuncs) AuthorsByAccount(ctx context.Context, handlesByAccount map[string][]string) (map[string]map[string]conversation_usecase.ContactDisplay, error) {
	if f.authorsInAccount == nil {
		return nil, nil
	}
	out := make(map[string]map[string]conversation_usecase.ContactDisplay, len(handlesByAccount))
	for accountID, handles := range handlesByAccount {
		if accountID == "" || len(handles) == 0 {
			continue
		}
		authors, err := f.authorsInAccount(ctx, accountID, handles)
		if err != nil {
			return nil, err
		}
		out[accountID] = authors
	}
	return out, nil
}

var _ conversation_usecase.ContactIdentityLookup = contactIdentityFuncs{}

func (c *Container) buildExportEntriesUseCase() export_domain.ExportEntriesUseCase {
	uc := export_usecase.NewExportEntriesUseCase(
		c.repositories.conversationAnalyses,
		c.repositories.stage,
	)

	setter, ok := uc.(interface {
		SetChannelEntryLister(export_domain.EntryType, export_domain.ChannelEntryLister)
	})
	if !ok {
		log.Fatalf("[export] the export use case cannot take per-channel entry listers")
	}
	setter.SetChannelEntryLister(export_domain.EntryTypeWhatsApp,
		wc_entry_repository.NewExportRepository(c.db))
	if c.instagram != nil {
		setter.SetChannelEntryLister(export_domain.EntryTypeInstagram, instagram_repository.NewExportRepository(c.db))
	}
	if c.telegram != nil {
		setter.SetChannelEntryLister(export_domain.EntryTypeTelegram, telegram_repository.NewExportRepository(c.db))
	}
	if c.unofficialWhatsApp != nil {
		setter.SetChannelEntryLister(export_domain.EntryTypeUnofficialWhatsApp,
			unofficial_whatsapp_repository.NewExportRepository(c.db))
	}
	if c.facebook != nil {
		setter.SetChannelEntryLister(export_domain.EntryTypeFacebook, facebook_repository.NewExportRepository(c.db))
	}
	if c.webchat != nil {
		setter.SetChannelEntryLister(export_domain.EntryTypeWebchat, webchat_repository.NewExportRepository(c.db))
	}
	setter.SetChannelEntryLister(export_domain.EntryTypeMercadoLivre, mercadolivre_repository.NewExportRepository(c.db))
	setter.SetChannelEntryLister(export_domain.EntryTypeTikTok, c.tiktokExportLister())
	setter.SetChannelEntryLister(export_domain.EntryTypeEmail, c.emailInboxExportLister())
	for entryType, lister := range c.carrierExportListers() {
		setter.SetChannelEntryLister(entryType, lister)
	}
	return uc
}
