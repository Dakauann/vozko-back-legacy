package container

import (
	"context"
	"log"
	"time"
	"vozko/domain/webhook"
	webhook_repository "vozko/infra/repositories/webhook"

	uwhttp "vozko/delivery/http/unofficial_whatsapp"
	conversation_domain "vozko/domain/conversation"
	"vozko/domain/shared"
	uw "vozko/domain/unofficial_whatsapp"
	uwrepo "vozko/infra/repositories/unofficial_whatsapp"
	"vozko/infra/uazapi"
	conversation_usecase "vozko/usecases/conversation"
	uwuc "vozko/usecases/unofficial_whatsapp"
	uwcuc "vozko/usecases/unofficial_whatsapp_campaign"
	uwguc "vozko/usecases/unofficial_whatsapp_group"
)

type unofficialWhatsAppBundle struct {
	LinkGuard     *uwuc.LinkGuard
	LineHandover  *uwuc.LineHandover
	Servers       uw.ServerRepository
	Instances     uw.InstanceRepository
	Contacts      uw.ContactRepository
	Conversations uw.ConversationRepository
	Groups        uw.GroupRepository

	Provider  uw.ProviderAPI
	History   uw.HistoryAPI
	Messaging uw.MessagingAPI
	GroupAPI  uw.GroupAPI
	Assets    uw.RemoteAssetFetcher

	StartConv *uwuc.StartConversationUseCase

	Entitlements *uwuc.InstanceEntitlementReader

	ProcessedEvts webhook.ProcessedEventRepository

	Handler        *uwhttp.Handler
	WebhookHandler *uwhttp.WebhookHandler
	GroupHandler   *uwhttp.GroupHandler
	GroupUseCases  *uwuc.GroupUseCases

	WorkspaceGroups       *uwguc.Service
	WorkspaceGroupHandler *uwhttp.WorkspaceGroupHandler

	Connect            *uwuc.ConnectInstanceUseCase
	HistorySync        *uwuc.HistorySyncUseCase
	Consume            *uwuc.ConsumeWebhookUseCase
	CheckHealth        *uwuc.CheckInstanceHealthUseCase
	ReconcileCapacity  *uwuc.ReconcileServerCapacityUseCase
	ProvisionInstances *uwuc.ProvisionInstanceUseCase

	SeedInboxPublisher *uwuc.SeedInboxPublisher
	ConsumeSeedInbox   *uwuc.ConsumeSeedInboxUseCase
}

func (c *Container) initUnofficialWhatsApp() {
	bundle := &unofficialWhatsAppBundle{}
	c.unofficialWhatsApp = bundle

	if err := uw.ValidateWebhookBaseURL(c.cfg.UnofficialWhatsAppWebhookBaseURL); err != nil {
		log.Fatalf("[unofficial-whatsapp] %v", err)
	}

	provider := uazapi.NewClient(uazapi.Config{})
	bundle.Provider = provider
	bundle.History = provider
	bundle.Messaging = provider
	bundle.GroupAPI = provider
	bundle.Assets = provider
	bundle.Servers = uwrepo.NewServerRepository(c.db)
	bundle.Instances = uwrepo.NewInstanceRepository(c.db)
	bundle.Contacts = uwrepo.NewContactRepository(c.db)
	bundle.Conversations = uwrepo.NewConversationRepository(c.db)
	bundle.Groups = uwrepo.NewGroupRepository(c.db)
	bundle.ProcessedEvts = webhook_repository.NewProcessedEventRepository(c.db)

	startConv, err := uwuc.NewStartConversationUseCase(uwuc.StartConversationDeps{
		Instances:     bundle.Instances,
		Servers:       bundle.Servers,
		Contacts:      bundle.Contacts,
		Conversations: bundle.Conversations,
		Messaging:     bundle.Messaging,
		Leads:         uwrepo.NewLeadLinker(c.repositories.lead),
		Starts:        c.services.assignmentService,
	})
	if err != nil {
		log.Fatalf("[unofficial-whatsapp] %v", err)
	}
	bundle.StartConv = startConv

	bundle.Entitlements = uwuc.NewInstanceEntitlementReader(bundle.Instances, lateWorkspaceEntitlements{handle: c.uwEntitlementSourceHandle()})
	bundle.LinkGuard = uwuc.NewLinkGuard(bundle.Instances, bundle.Servers, bundle.Provider)
	bundle.LineHandover = uwuc.NewLineHandover(uwrepo.NewLineRepository(c.db))
	connections := lateConnectionListener{handle: c.uwConnectionListenerHandle()}

	c.seedUnofficialWhatsAppServer(bundle)

	provision := uwuc.NewProvisionInstanceUseCase(
		bundle.Servers, bundle.Instances, provider, c.cfg.UnofficialWhatsAppWebhookBaseURL, bundle.Entitlements)
	bundle.ProvisionInstances = provision

	bundle.Connect = uwuc.NewConnectInstanceUseCase(bundle.Instances, bundle.Servers, provider, bundle.LinkGuard, connections)

	bundle.Handler = uwhttp.NewHandler(uwhttp.HandlerDeps{
		Provision:   provision,
		Connect:     bundle.Connect,
		List:        uwuc.NewListInstancesUseCase(bundle.Instances),
		Get:         uwuc.NewGetInstanceUseCase(bundle.Instances),
		UpdateCfg:   uwuc.NewUpdateInstanceConfigUseCase(bundle.Instances),
		RotateToken: uwuc.NewRotateDeliveryTokenUseCase(bundle.Instances, bundle.Servers, provision),
		Remove:      uwuc.NewDeleteInstanceUseCase(bundle.Instances, bundle.Servers, provider),
		StartConv:   bundle.StartConv,
		Allowance:   uwuc.NewGetAllowanceUseCase(bundle.Entitlements),
		Departments: c.services.conversationAuthImpl,
	})

	bundle.GroupUseCases = uwuc.NewGroupUseCases(uwuc.GroupUseCaseDeps{
		Instances:     bundle.Instances,
		Servers:       bundle.Servers,
		Contacts:      bundle.Contacts,
		Conversations: bundle.Conversations,
		Groups:        bundle.Groups,
		GroupAPI:      bundle.GroupAPI,
		Messaging:     bundle.Messaging,
		Assets:        bundle.Assets,
		FileStorage:   c.services.fileStorage,
	})
	bundle.GroupHandler = uwhttp.NewGroupHandler(bundle.GroupUseCases, c.services.conversationAuthImpl)

	bundle.WebhookHandler = uwhttp.NewWebhookHandler(bundle.Instances, nil)

	bundle.CheckHealth = uwuc.NewCheckInstanceHealthUseCase(
		bundle.Instances, bundle.Servers, provider, c.cfg.UnofficialWhatsAppWebhookBaseURL,
		bundle.LinkGuard, connections, bundle.LineHandover)
	bundle.ReconcileCapacity = uwuc.NewReconcileServerCapacityUseCase(
		bundle.Servers, bundle.Instances, provider)

	log.Printf("[unofficial-whatsapp] channel enabled (webhook base=%s)",
		c.cfg.UnofficialWhatsAppWebhookBaseURL)
	log.Printf("[unofficial-whatsapp] webhook URL pattern: %s%s/{deliveryToken}",
		c.cfg.UnofficialWhatsAppWebhookBaseURL, uw.WebhookPathPrefix)
}

func (c *Container) seedUnofficialWhatsAppServer(bundle *unofficialWhatsAppBundle) {
	server, err := uwuc.NewEnsurePlatformServerUseCase(bundle.Servers).
		Execute(context.Background(), uwuc.PlatformServerInput{
			Name:       c.cfg.UnofficialWhatsAppServerName,
			BaseURL:    c.cfg.UnofficialWhatsAppServerURL,
			AdminToken: c.cfg.UnofficialWhatsAppAdminToken,
			Capacity:   c.cfg.UnofficialWhatsAppMaxSessions,
		})
	if err != nil {
		log.Fatalf("[unofficial-whatsapp] could not register the platform host: %v", err)
	}
	if server.Capacity <= 0 {
		log.Printf("[unofficial-whatsapp] host %s has no capacity configured "+
			"(UNOFFICIAL_WHATSAPP_MAX_SESSIONS); no number can be connected to it", server.BaseURL)
	}
}

func (c *Container) initUnofficialWhatsAppRuntime(history conversation_domain.MessageHistoryManager) {
	bundle := c.unofficialWhatsApp
	if bundle == nil {
		return
	}
	if c.useCases == nil {
		log.Fatalf("[unofficial-whatsapp] runtime wiring ran before useCases were built")
	}

	if err := c.uwEntitlementSourceHandle().Bind(c.useCases.getWorkspaceEntitlements); err != nil {
		log.Fatalf("[unofficial-whatsapp] %v", err)
	}
	c.initWorkspaceGroups(bundle)

	bundle.WebhookHandler = uwhttp.NewWebhookHandler(bundle.Instances, c.useCases.publishWebhook)

	historyWindow := time.Duration(c.cfg.UnofficialWhatsAppHistoryDays) * 24 * time.Hour
	handler, err := uwuc.NewHandleWebhookUseCase(uwuc.HandleWebhookDeps{
		Instances:          bundle.Instances,
		Servers:            bundle.Servers,
		Contacts:           bundle.Contacts,
		Conversations:      bundle.Conversations,
		Groups:             bundle.Groups,
		Messaging:          bundle.Messaging,
		GroupAPI:           bundle.GroupAPI,
		Assets:             bundle.Assets,
		History:            history,
		Messages:           c.repositories.conversation,
		Media:              c.mediaStore(),
		FileStorage:        c.services.fileStorage,
		Broadcaster:        c.services.conversationHub,
		Assignments:        c.services.assignmentService,
		AIReply:            c.mustChannelAIReply(),
		Workflows:          c.useCases.triggerEvaluator,
		Leads:              uwrepo.NewLeadLinker(c.repositories.lead),
		Analysis:           c.services.analysisScheduler,
		AutomationGate:     c.services.automationGate,
		Choices:            c.choiceStatsRecorder(),
		LinkGate:           bundle.LinkGuard,
		Listener:           lateConnectionListener{handle: c.uwConnectionListenerHandle()},
		Handover:           bundle.LineHandover,
		HistoryWindow:      historyWindow,
		GroupWatcher:       bundle.WorkspaceGroups,
		CampaignSink:       uwcuc.NewDeliverySink(c.unofficialWhatsAppCampaigns.Entries),
		CampaignAutomation: uwcuc.NewAutomationSource(bundle.Conversations, c.unofficialWhatsAppCampaigns.Campaigns),
		ExitStep:           c.leadExitStep(),
	})
	if err != nil {
		log.Fatalf("[unofficial-whatsapp] %v", err)
	}

	c.wireUnofficialWhatsAppHistory(bundle, handler, historyWindow)

	bundle.SeedInboxPublisher = uwuc.NewSeedInboxPublisher(c.services.uwSeedQueuePub)
	bundle.ConsumeSeedInbox = uwuc.NewConsumeSeedInboxUseCase(
		c.services.uwSeedQueueSub,
		uwuc.NewSeedInboxUseCase(
			bundle.Instances,
			bundle.Contacts,
			bundle.Conversations,
			uwrepo.NewLeadLinker(c.repositories.lead),
			c.repositories.conversation,
			uwuc.NewConversationScripter(c.services.ai, c.cfg.OpenRouterDefaultModel),
			c.services.cachedBalanceChecker,
		).WithAttachments(c.repositories.media, c.repositories.conversationMedia),
	)
	if err := bundle.ConsumeSeedInbox.Start(); err != nil {
		log.Fatalf("[unofficial-whatsapp] the inbox seed consumer failed to start: %v", err)
	}

	logChannelCapabilities("unofficial-whatsapp", map[string]bool{
		"ai-reply":             c.services.channelAIReply != nil,
		"workflows":            c.useCases.triggerEvaluator != nil,
		"assignment":           c.services.assignmentService != nil,
		"broadcaster":          c.services.conversationHub != nil,
		"campaigns":            c.unofficialWhatsAppCampaigns != nil,
		"media":                c.services.fileStorage != nil,
		"avatars":              c.services.fileStorage != nil && bundle.Assets != nil,
		"groups":               bundle.Groups != nil && bundle.GroupAPI != nil,
		"inbox-seeding":        c.services.uwSeedQueuePub != nil && c.services.uwSeedQueueSub != nil,
		"inbox-seed-scripting": c.services.ai != nil,
		"department-scope":     c.services.conversationAuthImpl != nil,
	})

	bundle.Consume = uwuc.NewConsumeWebhookUseCase(
		c.services.webhookQueueSub,
		c.services.webhookQueuePub,
		c.redisProvider.SharedState(),
		bundle.ProcessedEvts,
		handler,
	)
}

func (c *Container) wireUnofficialWhatsAppConversationStack() {
	bundle := c.unofficialWhatsApp
	if bundle == nil {
		return
	}

	adapter := uwuc.NewChannelAdapter(
		bundle.Instances,
		bundle.Servers,
		bundle.Contacts,
		bundle.Conversations,
		bundle.Messaging,
		uazapi.NewVoiceTranscoder(mediaDownloads()),
	)

	c.registerConversationChannel(conversationChannel{
		EntryType:     shared.EntryTypeUnofficialWhatsApp,
		Adapter:       adapter,
		Conversations: bundle.Conversations,
		Automation: func(ctx context.Context, entryID string) (*bool, error) {
			conv, err := bundle.Conversations.FindByID(ctx, entryID)
			if err != nil {
				return nil, err
			}
			return conv.AutomationEnabled, nil
		},
		Contacts: unofficialWhatsAppContactIdentity(bundle),
	})
}

func unofficialWhatsAppContactIdentity(bundle *unofficialWhatsAppBundle) conversation_usecase.ContactIdentityLookup {
	contacts, conversations := bundle.Contacts, bundle.Conversations

	display := func(c *uw.Contact) conversation_usecase.ContactDisplay {
		leadID := ""
		if c.LeadID != nil {
			leadID = *c.LeadID
		}
		return conversation_usecase.ContactDisplay{
			ContactID:  c.ID,
			LeadID:     leadID,
			Ref:        c.JID,
			Handle:     c.Handle(),
			Name:       c.DisplayName(),
			PictureURL: c.PictureURL,
			IsGroup:    c.IsGroup,
		}
	}

	authorsInInstance := func(ctx context.Context, instanceID string, handles []string) (map[string]conversation_usecase.ContactDisplay, error) {
		found, err := contacts.FindByHandles(ctx, instanceID, handles)
		if err != nil {
			return nil, err
		}
		out := make(map[string]conversation_usecase.ContactDisplay, len(found))
		for _, contact := range found {
			if contact == nil {
				continue
			}
			out[contact.Handle()] = display(contact)
		}
		return out, nil
	}

	return contactIdentityFuncs{
		byIDs: func(ctx context.Context, ids []string) (map[string]conversation_usecase.ContactDisplay, error) {
			found, err := contacts.FindByIDs(ctx, ids)
			if err != nil {
				return nil, err
			}
			out := make(map[string]conversation_usecase.ContactDisplay, len(found))
			for _, contact := range found {
				if contact == nil {
					continue
				}
				d := display(contact)
				out[contact.ID] = d
				if d.LeadID != "" {
					out[d.LeadID] = d
				}
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
		authorsByHandle: func(ctx context.Context, entryID string, handles []string) (map[string]conversation_usecase.ContactDisplay, error) {
			conv, err := conversations.FindByID(ctx, entryID)
			if err != nil {
				return nil, err
			}
			if !conv.IsGroup {
				return nil, nil
			}
			return authorsInInstance(ctx, conv.InstanceID, handles)
		},
		authorsInAccount: authorsInInstance,
	}
}

func unofficialWhatsAppHandler(c *Container) *uwhttp.Handler {
	if c.unofficialWhatsApp == nil {
		return nil
	}
	return c.unofficialWhatsApp.Handler
}

func unofficialWhatsAppGroupHandler(c *Container) *uwhttp.GroupHandler {
	if c.unofficialWhatsApp == nil {
		return nil
	}
	return c.unofficialWhatsApp.GroupHandler
}

func unofficialWhatsAppWebhookHandler(c *Container) *uwhttp.WebhookHandler {
	if c.unofficialWhatsApp == nil {
		return nil
	}
	return c.unofficialWhatsApp.WebhookHandler
}

func (c *Container) wireUnofficialWhatsAppHistory(bundle *unofficialWhatsAppBundle, handler *uwuc.HandleWebhookUseCase, window time.Duration) {
	historySync, err := uwuc.NewHistorySyncUseCase(uwuc.HistorySyncDeps{
		Runs:      uwrepo.NewHistorySyncRepository(c.db),
		Instances: bundle.Instances,
		Servers:   bundle.Servers,
		Source:    bundle.History,
		Importer:  handler,
		Handover:  bundle.LineHandover,
		Window:    window,
	})
	if err != nil {
		log.Fatalf("[unofficial-whatsapp] history import: %v", err)
	}
	if err := c.uwConnectionListenerHandle().Bind(historySync); err != nil {
		log.Fatalf("[unofficial-whatsapp] %v", err)
	}
	bundle.Handler.SetHistorySync(historySync)
	bundle.HistorySync = historySync

	log.Printf("[unofficial-whatsapp] history import: window %d days, sweep every minute",
		c.cfg.UnofficialWhatsAppHistoryDays)
}
