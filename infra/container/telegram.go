package container

import (
	"context"
	"log"
	"strconv"
	"vozko/domain/webhook"
	webhook_repository "vozko/infra/repositories/webhook"

	telegramhttp "vozko/delivery/http/telegram"
	conversation_domain "vozko/domain/conversation"
	"vozko/domain/shared"
	tgdomain "vozko/domain/telegram"
	telegram_repository "vozko/infra/repositories/telegram"
	tginfra "vozko/infra/telegram"
	conversation_usecase "vozko/usecases/conversation"
	tguc "vozko/usecases/telegram"
)

type telegramBundle struct {
	Accounts      tgdomain.AccountRepository
	Contacts      tgdomain.ContactRepository
	Conversations tgdomain.ConversationRepository
	DeepLinks     tgdomain.DeepLinkRepository
	Files         tgdomain.FileCacheRepository
	ProcessedEvts webhook.ProcessedEventRepository

	API tgdomain.BotAPI

	Connect *tguc.ConnectAccountUseCase
	List    *tguc.ListAccountsUseCase

	Handler        *telegramhttp.Handler
	WebhookHandler *telegramhttp.WebhookHandler

	Consume     *tguc.ConsumeWebhookUseCase
	CheckHealth *tguc.CheckWebhookHealthUseCase
}

func (c *Container) initTelegram() {
	bundle := &telegramBundle{}
	c.telegram = bundle

	if err := tgdomain.ValidateWebhookBaseURL(c.cfg.TelegramWebhookBaseURL); err != nil {
		log.Fatalf("[telegram] %v", err)
	}

	api, err := tginfra.NewThrottled(
		tginfra.NewClient(tginfra.Config{BaseURL: c.cfg.TelegramBotAPIBaseURL}),
		c.redisProvider.RateLimiterFactory(),
	)
	if err != nil {
		log.Fatalf("[telegram] %v", err)
	}
	bundle.API = api

	bundle.Accounts = telegram_repository.NewAccountRepository(c.db)
	bundle.Contacts = telegram_repository.NewContactRepository(c.db)
	bundle.Conversations = telegram_repository.NewConversationRepository(c.db)
	bundle.DeepLinks = telegram_repository.NewDeepLinkRepository(c.db)
	bundle.Files = telegram_repository.NewFileCacheRepository(c.db)
	bundle.ProcessedEvts = webhook_repository.NewProcessedEventRepository(c.db)

	bundle.Connect = tguc.NewConnectAccountUseCase(bundle.Accounts, api, c.cfg.TelegramWebhookBaseURL)
	bundle.List = tguc.NewListAccountsUseCase(bundle.Accounts)
	bundle.Handler = telegramhttp.NewHandler(telegramhttp.HandlerDeps{
		Connect:    bundle.Connect,
		Reregister: tguc.NewReregisterWebhookUseCase(bundle.Accounts, api, c.cfg.TelegramWebhookBaseURL),
		List:       bundle.List,
		Get:        tguc.NewGetAccountUseCase(bundle.Accounts),
		UpdateCfg:  tguc.NewUpdateAccountConfigUseCase(bundle.Accounts),
		Disconnect: tguc.NewDisconnectAccountUseCase(bundle.Accounts, api),
		CreateLink: tguc.NewCreateDeepLinkUseCase(bundle.Accounts, bundle.DeepLinks),
		ListLinks:  tguc.NewListDeepLinksUseCase(bundle.Accounts, bundle.DeepLinks),
		DeleteLink: tguc.NewDeleteDeepLinkUseCase(bundle.Accounts, bundle.DeepLinks),
	})
	bundle.WebhookHandler = telegramhttp.NewWebhookHandler(bundle.Accounts, nil)

	bundle.CheckHealth = tguc.NewCheckWebhookHealthUseCase(bundle.Accounts, api, c.cfg.TelegramWebhookBaseURL)

	log.Printf("[telegram] channel enabled (webhook base=%s)", c.cfg.TelegramWebhookBaseURL)
	log.Printf("[telegram] webhook URL pattern: %s%s/{accountId}",
		c.cfg.TelegramWebhookBaseURL, tgdomain.WebhookPathPrefix)
}

func (c *Container) initTelegramRuntime(history conversation_domain.MessageHistoryManager) {
	bundle := c.telegram
	if bundle == nil {
		return
	}
	if c.useCases == nil {
		log.Fatalf("[telegram] runtime wiring ran before useCases were built")
	}

	bundle.WebhookHandler = telegramhttp.NewWebhookHandler(bundle.Accounts, c.useCases.publishWebhook)

	handler := tguc.NewHandleWebhookUseCase(tguc.HandleWebhookDeps{
		Accounts:      bundle.Accounts,
		Contacts:      bundle.Contacts,
		Conversations: bundle.Conversations,
		DeepLinks:     bundle.DeepLinks,
		API:           bundle.API,
		History:       history,
		Messages:      c.repositories.conversation,
		Media:         c.mediaStore(),
		FileStorage:   c.services.fileStorage,
		Broadcaster:   c.services.conversationHub,
		Assignments:   c.services.assignmentService,
		AIReply:       c.mustChannelAIReply(),
		Workflows:     c.useCases.triggerEvaluator,
		Leads:         telegram_repository.NewLeadLinker(c.repositories.lead),
		Analysis:      c.services.analysisScheduler,
		Delegations:   c.services.delegations,
	})

	bundle.Consume = tguc.NewConsumeWebhookUseCase(
		c.services.webhookQueueSub,
		c.services.webhookQueuePub,
		c.redisProvider.SharedState(),
		bundle.ProcessedEvts,
		handler,
	)
}

func (c *Container) wireTelegramConversationStack() {
	bundle := c.telegram
	if bundle == nil {
		return
	}

	adapter := tguc.NewChannelAdapter(
		bundle.Accounts,
		bundle.Contacts,
		bundle.Conversations,
		bundle.Files,
		bundle.API,
	)

	c.registerConversationChannel(conversationChannel{
		EntryType:     shared.EntryTypeTelegram,
		Adapter:       adapter,
		Conversations: bundle.Conversations,
		Automation: func(ctx context.Context, entryID string) (*bool, error) {
			conv, err := bundle.Conversations.FindByID(ctx, entryID)
			if err != nil {
				return nil, err
			}
			return conv.AutomationEnabled, nil
		},
		Contacts: telegramContactIdentity(bundle),
	})
}

func telegramContactIdentity(bundle *telegramBundle) conversation_usecase.ContactIdentityLookup {
	contacts, conversations := bundle.Contacts, bundle.Conversations

	display := func(c *tgdomain.Contact) conversation_usecase.ContactDisplay {
		return conversation_usecase.ContactDisplay{
			ContactID:  c.ID,
			Ref:        strconv.FormatInt(c.TGUserID, 10),
			Handle:     c.Handle(),
			Name:       c.DisplayName(),
			PictureURL: c.PhotoURL,
		}
	}

	return contactIdentityFuncs{
		byIDs: func(ctx context.Context, ids []string) (map[string]conversation_usecase.ContactDisplay, error) {
			found, err := contacts.FindByIDs(ctx, ids)
			if err != nil {
				return nil, err
			}
			out := make(map[string]conversation_usecase.ContactDisplay, len(found))
			for _, c := range found {
				if c == nil {
					continue
				}
				out[c.ID] = display(c)
			}
			return out, nil
		},
		forConversation: func(ctx context.Context, conversationID string) (conversation_usecase.ContactDisplay, string, error) {
			conv, err := conversations.FindByID(ctx, conversationID)
			if err != nil {
				return conversation_usecase.ContactDisplay{}, "", err
			}
			contact, err := contacts.FindByID(ctx, conv.ContactID)
			if err != nil {
				return conversation_usecase.ContactDisplay{}, conv.WorkspaceID, err
			}
			return display(contact), conv.WorkspaceID, nil
		},
	}
}

func telegramHandler(c *Container) *telegramhttp.Handler {
	if c.telegram == nil {
		return nil
	}
	return c.telegram.Handler
}

func telegramWebhookHandler(c *Container) *telegramhttp.WebhookHandler {
	if c.telegram == nil {
		return nil
	}
	return c.telegram.WebhookHandler
}
