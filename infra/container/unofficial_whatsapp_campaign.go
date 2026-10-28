package container

import (
	"context"
	"log"

	uwhttp "vozko/delivery/http/unofficial_whatsapp"
	wsdelivery "vozko/delivery/ws"
	"vozko/domain/shared"
	uw "vozko/domain/unofficial_whatsapp"
	uwc "vozko/domain/unofficial_whatsapp_campaign"
	workspace_department "vozko/domain/workspace/workspace_department"
	"vozko/infra/repositories/campaignstamp"
	uwrepo "vozko/infra/repositories/unofficial_whatsapp"
	uwc_repo "vozko/infra/repositories/unofficial_whatsapp_campaign"
	campaignaudience_usecase "vozko/usecases/campaignaudience"
	conversation_usecase "vozko/usecases/conversation"
	uwuc "vozko/usecases/unofficial_whatsapp"
	uwcuc "vozko/usecases/unofficial_whatsapp_campaign"
)

type unofficialWhatsAppCampaignBundle struct {
	Campaigns uwc.Repository
	Entries   uwc.EntryRepository

	Consumer     uwc.MessageConsumerUseCase
	Dispatch     uwc.DispatchCampaignUseCase
	Create       uwc.CreateCampaignUseCase
	Access       uwc.CampaignAccessUseCase
	Actions      uwc.CampaignActionUseCase
	Preview      uwc.ImportPreviewUseCase
	Instances    uwc.CampaignInstanceUseCase
	ScheduleJob  uwc.StartScheduleJob
	PauseForInst uwc.PauseCampaignsForInstanceUseCase
	Senders      uwcuc.GroupSenderDirectory
	Estimate     uwc.EstimateCampaignUseCase
	Preflight    *uwcuc.StartPreflight
	CreateFlow   uwc.CreateCampaignUseCase
	DraftLaunch  *uwcuc.DraftLaunch

	Handler *uwhttp.CampaignHandler
}

func (c *Container) initUnofficialWhatsAppCampaigns(
	sender *conversation_usecase.MessageSenderService,
	departments workspace_department.CreationDepartmentResolver,
) {
	bundle := &unofficialWhatsAppCampaignBundle{}
	c.unofficialWhatsAppCampaigns = bundle

	if c.unofficialWhatsApp == nil {
		return
	}

	bundle.Campaigns = uwc_repo.NewRepository(c.db)
	bundle.Entries = uwc_repo.NewEntryRepository(c.db)
	summary := uwc_repo.NewSummaryRepository(c.db)

	gateway := &unofficialCampaignGateway{
		instances: c.unofficialWhatsApp.Instances,
		servers:   c.unofficialWhatsApp.Servers,
		messaging: c.unofficialWhatsApp.Messaging,
		provider:  c.unofficialWhatsApp.Provider,
		groups:    c.unofficialWhatsApp.Groups,
		resolver: uwuc.NewConversationResolver(
			c.unofficialWhatsApp.Contacts,
			c.unofficialWhatsApp.Conversations,
			uwrepo.NewLeadLinker(c.repositories.lead),
		),
	}

	spam := c.campaignSpamGuard()
	eligibility := c.campaignEligibility()

	budget := uwcuc.NewSendBudget(c.redisProvider.SharedState())

	consumer, err := uwcuc.NewMessageConsumerUseCase(uwcuc.ConsumerDeps{
		QueueSub:    c.services.wcQueueSub,
		QueuePub:    c.services.wcQueuePub,
		Shared:      c.redisProvider.SharedState(),
		Campaigns:   bundle.Campaigns,
		Entries:     bundle.Entries,
		Instances:   gateway,
		Sender:      sender,
		Budget:      budget,
		Spam:        spam,
		Eligibility: eligibility,
		Assignments: c.services.assignmentService,
		Broadcaster: &campaignEntryBroadcaster{hub: c.services.conversationHub},
		Groups:      gateway,
		Choices:     c.choiceStatsRecorder(),
	})
	if err != nil {
		log.Fatalf("[unofficial-whatsapp-campaigns] %v", err)
	}
	bundle.Consumer = consumer
	bundle.PauseForInst = uwcuc.NewPauseCampaignsForInstanceUseCase(bundle.Campaigns, consumer)

	bundle.Dispatch = uwcuc.NewDispatchCampaignUseCase(
		bundle.Campaigns, bundle.Entries, gateway, consumer,
		c.services.wcQueuePub, c.redisProvider.SharedState(),
	)
	bundle.ScheduleJob = uwcuc.NewScheduleStartJob(bundle.Campaigns, bundle.Dispatch, unofficialSchedules(bundle.Campaigns), c.unofficialScheduleReporter(bundle.Campaigns))
	keyed, ok := bundle.Campaigns.(uwcuc.IdempotentCampaigns)
	if !ok {
		log.Fatalf("[unofficial-whatsapp-campaigns] the campaign repository cannot read idempotency keys")
	}
	create, err := uwcuc.NewCreateCampaignUseCase(uwcuc.CreateCampaignDeps{
		Campaigns:    bundle.Campaigns,
		Entries:      bundle.Entries,
		Leads:        c.repositories.lead,
		Instances:    gateway,
		Eligibility:  eligibility,
		Departments:  departments,
		LeadsByID:    c.repositories.lead,
		Automation:   c.campaignAutomation().WithoutSheets().OnChannel(shared.EntryTypeUnofficialWhatsApp),
		Keyed:        keyed,
		GroupSenders: gateway,
	})
	if err != nil {
		log.Fatalf("[unofficial-whatsapp-campaigns] %v", err)
	}
	bundle.Create = create
	update, err := uwcuc.NewUpdateCampaignUseCase(bundle.Campaigns, bundle.Entries, gateway, c.campaignAutomation().WithoutSheets().OnChannel(shared.EntryTypeUnofficialWhatsApp))
	if err != nil {
		log.Fatalf("[unofficial-whatsapp-campaigns] %v", err)
	}
	addEntries, err := uwcuc.NewAddEntriesUseCase(bundle.Campaigns, bundle.Entries, c.repositories.lead, c.campaignAutomation().WithoutSheets().OnChannel(shared.EntryTypeUnofficialWhatsApp))
	if err != nil {
		log.Fatalf("[unofficial-whatsapp-campaigns] %v", err)
	}
	quickSend, err := uwcuc.NewQuickSendUseCase(bundle.Campaigns, bundle.Entries, c.repositories.lead, bundle.Dispatch, c.redisProvider.SharedState(), c.campaignAutomation().WithoutSheets().OnChannel(shared.EntryTypeUnofficialWhatsApp))
	if err != nil {
		log.Fatalf("[unofficial-whatsapp-campaigns] %v", err)
	}
	bundle.Access = uwcuc.NewCampaignAccessUseCase(uwcuc.NewGetCampaignUseCase(bundle.Campaigns, bundle.Entries, gateway))
	bundle.Actions = uwcuc.NewCampaignActionUseCase(bundle.Access, bundle.Dispatch, c.unofficialScheduleReporter(bundle.Campaigns))
	bundle.Preview = uwcuc.NewImportPreviewUseCase(c.useCases.readMedia)
	bundle.Instances = uwcuc.NewCampaignInstanceUseCase(gateway)
	c.addUnofficialAudience(bundle.Instances)
	bundle.Senders = gateway
	bundle.Estimate = uwcuc.NewEstimateCampaignUseCase(bundle.Entries, gateway, gateway)
	bundle.Preflight = uwcuc.NewStartPreflight(gateway)
	bundle.CreateFlow = campaignaudience_usecase.UnofficialCreate(bundle.Create, c.campaignAudience())

	logChannelCapabilities("unofficial-whatsapp-campaigns", map[string]bool{
		"sender":         sender != nil,
		"assignment":     c.services.assignmentService != nil,
		"broadcaster":    c.services.conversationHub != nil,
		"departments":    departments != nil,
		"spam-cooldown":  c.repositories.leadCampaignSend != nil,
		"department-acl": c.services.conversationAuthImpl != nil,
	})

	bundle.DraftLaunch = uwcuc.NewDraftLaunch(uwcuc.DraftLaunchDeps{
		Create:    bundle.CreateFlow,
		Actions:   bundle.Actions,
		Estimate:  bundle.Estimate,
		Audience:  campaignaudience_usecase.NewUnofficialCounter(c.campaignAudience()),
		Preflight: bundle.Preflight,
	})
	live := c.campaignLive("unofficial_whatsapp_campaign", bundle.Entries, campaignstamp.UnofficialEntries)
	handler, err := uwhttp.NewCampaignHandler(uwhttp.CampaignHandlerDeps{
		Create:      bundle.CreateFlow,
		Update:      update,
		Get:         uwcuc.NewGetCampaignUseCase(bundle.Campaigns, bundle.Entries, gateway),
		Access:      bundle.Access,
		Actions:     bundle.Actions,
		List:        uwcuc.NewListCampaignsUseCase(bundle.Campaigns, bundle.Entries, gateway),
		Delete:      uwcuc.NewDeleteCampaignUseCase(bundle.Campaigns, bundle.Entries),
		AssignDep:   uwcuc.NewAssignDepartmentUseCase(bundle.Campaigns, departments),
		Summary:     uwcuc.NewGetSummaryUseCase(summary),
		Entries:     uwcuc.NewListEntriesUseCase(bundle.Entries, gateway),
		Reset:       uwcuc.NewResetCampaignUseCase(bundle.Campaigns, bundle.Entries),
		Clear:       uwcuc.NewClearHistoryUseCase(bundle.Campaigns, bundle.Entries, c.repositories.conversation),
		AddEntry:    addEntries,
		UpdateEntry: uwcuc.NewUpdateEntryUseCase(bundle.Campaigns, bundle.Entries, c.repositories.lead),
		DeleteEntry: uwcuc.NewDeleteEntryUseCase(bundle.Campaigns, bundle.Entries),
		QuickSend:   quickSend,
		Validate:    uwcuc.NewValidateTargetsUseCase(bundle.Campaigns, bundle.Entries, gateway),
		Estimate:    bundle.Estimate,
		Departments: c.services.conversationAuthImpl,
		Permissions: c.useCases.checkWsAccess,
		Progress:    live.progress,
		Timeline:    live.timeline,
		Scheduled:   bundle.DraftLaunch,
	})
	if err != nil {
		log.Fatalf("[unofficial-whatsapp-campaigns] %v", err)
	}
	bundle.Handler = handler
}

func (c *Container) startUnofficialWhatsAppCampaigns() {
	if err := c.unofficialWhatsAppCampaigns.Consumer.Start(); err != nil {
		log.Fatalf("[unofficial-whatsapp-campaign] consumer failed to start: %v", err)
	}
}

type unofficialCampaignGateway struct {
	instances uw.InstanceRepository
	servers   uw.ServerRepository
	messaging uw.MessagingAPI
	provider  uw.ProviderAPI
	groups    uw.GroupRepository
	resolver  *uwuc.ConversationResolver
}

func (g *unofficialCampaignGateway) Seats(ctx context.Context, workspaceID, groupJID string) ([]*uw.Group, error) {
	seats, err := g.SeatsOf(ctx, workspaceID, []string{groupJID})
	if err != nil {
		return nil, err
	}
	return seats[groupJID], nil
}

func (g *unofficialCampaignGateway) SeatsOf(ctx context.Context, workspaceID string, groupJIDs []string) (map[string][]*uw.Group, error) {
	return g.groups.ListByJIDs(ctx, workspaceID, groupJIDs)
}

func (g *unofficialCampaignGateway) WorkspaceInstances(ctx context.Context, workspaceID string) ([]*uw.Instance, error) {
	return uwuc.AllWorkspaceInstances(ctx, g.instances, workspaceID)
}

func (g *unofficialCampaignGateway) Conversation(ctx context.Context, instance *uw.Instance, groupJID, subject string) (*uwuc.Resolved, error) {
	return g.resolver.ResolveGroup(ctx, instance, groupJID, subject)
}

func (g *unofficialCampaignGateway) Instance(ctx context.Context, instanceID string) (*uw.Instance, error) {
	return g.instances.FindByID(ctx, instanceID)
}

func (g *unofficialCampaignGateway) Ref(ctx context.Context, instance *uw.Instance) (uw.InstanceRef, error) {
	server, err := g.servers.FindByID(ctx, instance.ServerID)
	if err != nil {
		return uw.InstanceRef{}, err
	}
	return uw.RefFor(server, instance), nil
}

func (g *unofficialCampaignGateway) CheckNumbers(ctx context.Context, ref uw.InstanceRef, numbers []string) ([]uw.NumberCheck, error) {
	return g.messaging.CheckNumbers(ctx, ref, numbers)
}

func (g *unofficialCampaignGateway) MessagingLimits(ctx context.Context, ref uw.InstanceRef) (*uw.Restriction, error) {
	return g.provider.MessagingLimits(ctx, ref)
}

func (g *unofficialCampaignGateway) CacheRestriction(ctx context.Context, instanceID string, r uw.Restriction) error {
	return g.instances.UpdateRestriction(ctx, instanceID, r)
}

func (g *unofficialCampaignGateway) Resolve(ctx context.Context, instance *uw.Instance, in uwuc.ResolveInput) (*uwuc.Resolved, error) {
	return g.resolver.Resolve(ctx, instance, in)
}

type campaignEntryBroadcaster struct{ hub *wsdelivery.ConversationHub }

func (b *campaignEntryBroadcaster) BroadcastEntryUpdate(entryID, entryType string, _ interface{}) {
	if b.hub == nil {
		return
	}
	b.hub.BroadcastEntryUpdate(entryID, entryType, nil)
}

func unofficialWhatsAppCampaignHandler(c *Container) *uwhttp.CampaignHandler {
	if c.unofficialWhatsAppCampaigns == nil {
		return nil
	}
	return c.unofficialWhatsAppCampaigns.Handler
}
