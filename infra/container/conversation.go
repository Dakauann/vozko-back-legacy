package container

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	wsdelivery "vozko/delivery/ws"
	conversation_domain "vozko/domain/conversation"
	"vozko/domain/shared"
	"vozko/domain/whatsapp/medialimits"
	conversation_infra "vozko/infra/conversation"
	whatsapp_infra "vozko/infra/conversation/whatsapp"
	"vozko/infra/conversation/whatsapp/media"
	"vozko/infra/natdiscovery"
	ah_repository "vozko/infra/repositories/assignment_history"
	conversation_repository "vozko/infra/repositories/conversation"
	ia_repo "vozko/infra/repositories/inbox_assignment"
	livedecision_repository "vozko/infra/repositories/livedecision"
	wc_entry_repository "vozko/infra/repositories/whatsapp_campaign_entry"
	workflow_repository "vozko/infra/repositories/workflow"
	"vozko/usecases/agentturn"
	aa_usecase "vozko/usecases/ai_attendance"
	cauc "vozko/usecases/audience"
	"vozko/usecases/channelmedia"
	conversation_usecase "vozko/usecases/conversation"
	ce_usecase "vozko/usecases/conversation_event"
	crm_telemetry_usecase "vozko/usecases/crm_telemetry"
	ia_usecase "vozko/usecases/inbox_assignment"
	label_usecase "vozko/usecases/label"
	livedecisions_usecase "vozko/usecases/livedecisions"
	"vozko/usecases/messageaction"
	"vozko/usecases/messagestate"
	scheduled_message_usecase "vozko/usecases/scheduled_message"
	stage_usecase "vozko/usecases/stage"
	workflow_usecase "vozko/usecases/workflow"
)

func (c *Container) wireConversationHub() {
	workspaceResolver := conversation_usecase.NewCampaignWorkspaceResolver(
		c.repositories.wcCampaign,
		c.repositories.wcEntry,
		c.repositories.departmentRoutes,
	)

	conversationAuthorizer := conversation_infra.NewAuthorizer(
		c.repositories.wcEntry,
		wc_entry_repository.NewPlacementDirectory(c.db),
		c.repositories.workspace,
		c.cachedDepartmentScope(),
		c.repositories.inboxAssignment,
		workspaceResolver,
		c.redisProvider.SharedState(),
	)
	c.services.conversationAuth = conversationAuthorizer
	if impl, ok := conversationAuthorizer.(*conversation_infra.Authorizer); ok {
		c.services.conversationAuthImpl = impl
	}

	c.services.liveOperatorSend = conversation_domain.NewLiveOperatorSend()
	c.services.personSend = conversation_usecase.NewPersonSend(conversationAuthorizer, c.services.liveOperatorSend)

	if c.services.crmTelemetryPublisher == nil && c.services.crmTelemetryPub != nil {
		drops := crm_telemetry_usecase.NewLogDropRecorder()
		c.services.crmTelemetryPublisher = crm_telemetry_usecase.NewPublisherWithDrops(c.services.crmTelemetryPub, drops)
	}
	if c.services.crmTelemetryEmitter == nil && c.services.crmTelemetryPublisher != nil {
		c.services.crmTelemetryEmitter = crm_telemetry_usecase.NewEmitter(c.services.crmTelemetryPublisher)
	}
	telemetryPub := c.services.crmTelemetryPublisher
	eventLoggerEarly := ce_usecase.NewLogger(telemetryPub)

	c.services.delegations = conversation_repository.NewDelegationRepository(c.db)
	c.services.automationGate = conversation_usecase.NewAutomationGate(conversation_usecase.AutomationGateDeps{
		Owners:      c.repositories.inboxAssignment,
		Delegations: c.services.delegations,
		Profiles:    conversation_repository.NewEntryAutomationReader(c.db),
	})
	workflowRuns, ok := c.repositories.workflowRun.(conversation_usecase.WorkflowRunLookup)
	if !ok {
		log.Fatalf("[conversation-history] the workflow run repository cannot find active runs by conversation")
	}
	workflows, ok := c.repositories.workflow.(conversation_usecase.WorkflowLookup)
	if !ok {
		log.Fatalf("[conversation-history] the workflow repository cannot read workflows and their versions by id")
	}
	historyProvider, err := conversation_usecase.NewHistoryProviderService(conversation_usecase.HistoryDeps{
		Messages:     c.repositories.conversation,
		WhatsApp:     c.repositories.wcEntry,
		Leads:        c.repositories.lead,
		Users:        c.repositories.user,
		Agents:       c.repositories.agent,
		Windows:      c.repositories.leadMessageWindow,
		Media:        c.repositories.conversationMedia,
		Assignments:  c.repositories.inboxAssignment,
		Delegations:  c.services.delegations,
		Adapters:     c.liveAdapterRegistry(),
		Workspaces:   workspaceResolver,
		WorkflowRuns: workflowRuns,
		Workflows:    workflows,
	})
	if err != nil {
		log.Fatalf("[conversation-history] %v", err)
	}
	c.services.conversationHistory = historyProvider
	c.services.conversationMessagesSince = historyProvider
	c.services.conversationHistoryReader = conversation_usecase.NewHistoryReader(conversationAuthorizer, historyProvider, c.adOriginReader())

	messageMarker := conversation_usecase.NewMessageMarkerService(
		c.repositories.conversation,
		c.services.whatsappClientFactory,
		c.repositories.wcEntry,
		c.liveAdapterRegistry(),
	)
	c.services.messageMarker = messageMarker

	StageProvider := stage_usecase.NewStageProviderService(c.repositories.stage)

	InitialStageAssigner := stage_usecase.NewInitialStageAssignerService(c.repositories.stage)

	labelProvider := label_usecase.NewLabelProviderService(c.repositories.label)
	templateSender := conversation_usecase.NewTemplateSenderService(eventLoggerEarly)

	hub, err := wsdelivery.NewConversationHub(wsdelivery.ConversationHubDeps{
		Authorizer:    c.services.conversationAuth,
		Users:         c.repositories.user,
		OperatorSend:  c.services.liveOperatorSend,
		SharedState:   c.redisProvider.SharedState(),
		Metrics:       c.services.metrics,
		ReplicaID:     c.replicaID,
		PublicAddress: c.cfg.PublicReplicaURL,
		History:       historyProvider,
		HistoryReader: c.services.conversationHistoryReader,
		Marker:        messageMarker,
		Stages:        StageProvider,
		Labels:        labelProvider,
		Templates:     templateSender,
		Workspaces:    workspaceResolver,
		Messages:      c.repositories.conversation,
		Campaigns:     c.repositories.wcCampaign,
		Departments:   c.repositories.workspaceDepartment,
		Presence:      crm_telemetry_usecase.NewPresenceAdapter(telemetryPub),
		MediaLimits: channelmedia.Limits{
			Adapters: c.liveAdapterRegistry(),
			Official: medialimits.Official(),
		},
	})
	if err != nil {
		log.Fatalf("[container] %v", err)
	}
	c.services.conversationHub = hub

	var analysisProvider conversation_domain.AnalysisProvider
	if c.repositories.conversationAnalyses != nil {
		analysisProvider = cauc.NewConversationAnalysisProvider(c.repositories.conversationAnalyses)
	}

	c.services.campaignWorkspaceResolver = workspaceResolver
	c.services.aiAttendanceService = aa_usecase.NewAsyncSessionService(telemetryPub)
	agentRuns, err := workflow_usecase.NewAgentRunFinishCanceller(workflow_repository.NewAgentCallRunsRepository(c.db), c.repositories.workflowRunLog)
	if err != nil {
		log.Fatalf("[conversation-status] %v", err)
	}
	conversationStatusUpdater, err := conversation_usecase.NewConversationStatusService(conversation_usecase.StatusDeps{
		WhatsApp:    c.repositories.wcEntry,
		Delegations: c.services.delegations,
		Events:      eventLoggerEarly,
		WorkspaceOf: func(entryID, entryType string) string {
			ws, _ := workspaceResolver.GetEntryWorkspaceID(entryID, entryType)
			return ws
		},
		AISessions:  c.services.aiAttendanceService,
		Outcomes:    outcomeCaptureReader{configs: c.repositories.workspaceConfig},
		Announcer:   c.services.conversationHub,
		Departments: c.entryDepartmentResolver(),
		AgentRuns:   agentRuns,
	})
	if err != nil {
		log.Fatalf("[conversation-status] %v", err)
	}
	c.services.conversationStatusService = conversationStatusUpdater
	c.services.personFinish = conversation_usecase.NewPersonFinish(conversationAuthorizer, conversationStatusUpdater)
	c.services.conversationStatusUpdater = conversationStatusUpdater
	followUpReplies, err := scheduled_message_usecase.NewReplyCancellation(c.repositories.scheduledMessage)
	if err != nil {
		log.Fatalf("[message-arrivals] %v", err)
	}
	c.services.messageArrivals, err = conversation_usecase.NewMessageArrivals(conversation_usecase.MessageArrivalsDeps{
		Broadcaster: hub,
		Workspaces:  workspaceResolver,
		Stages:      InitialStageAssigner,
		Status:      conversationStatusUpdater,
		Replies:     followUpReplies,
	})
	if err != nil {
		log.Fatalf("[message-arrivals] %v", err)
	}

	inboxSvc := conversation_usecase.NewInboxService(
		historyProvider,
		StageProvider,
		labelProvider,
		workspaceResolver,
		InitialStageAssigner,
		c.services.conversationAuth,
		analysisProvider,
		conversationStatusUpdater,
		conversation_usecase.InboxSources{
			Schedules: conversation_usecase.NewAnalysisScheduleReader(c.redisProvider.SharedState()),
			Templates: templateSender,
			LiveReads: livedecisions_usecase.InboxReads{Reads: livedecision_repository.NewReadStore(c.db)},
		},
	)
	c.services.inboxService = inboxSvc
	c.services.conversationEntryLookup = conversation_usecase.NewEntryLookup(c.services.conversationAuth, inboxSvc)

	assignmentRepo := ia_repo.New(c.db)
	c.services.conversationAutomation = conversation_usecase.NewConversationAutomationService(
		c.services.conversationHub,
	).WithDelegations(c.services.delegations)
	candidates, err := ia_usecase.NewCandidateResolver(ia_usecase.CandidateDeps{
		Online: c.services.conversationHub,
		Roster: ia_usecase.NewRosterService(
			c.repositories.workspace,
			c.repositories.workspaceDepartment,
			c.services.conversationAuth,
			c.redisProvider.SharedState(),
		),
		Presence: c.repositories.agentPresence,
		Workload: ia_repo.NewWorkloadRepository(c.db),
		Handouts: ah_repository.NewHandoutReader(c.db),
	})
	if err != nil {
		log.Fatalf("[inbox-assignment] %v", err)
	}
	entryInfo := conversation_repository.NewEntryAutomationReader(c.db)
	c.services.assignmentService, err = ia_usecase.NewAssignmentService(ia_usecase.AssignmentDeps{
		Assignments:       assignmentRepo,
		Candidates:        candidates,
		WorkspaceResolver: workspaceResolver,
		WorkspaceConfig:   c.repositories.workspaceConfig,
		Telemetry:         telemetryPub,
		Events:            eventLoggerEarly,
		Outreach:          ia_repo.NewOutreachRepository(c.db),
		Routes:            c.repositories.departmentRoutes,
		AccessRevoker:     c.services.conversationAuthImpl,
		Portfolio:         c.customerPortfolioEntries(),
		Automation:        entryInfo,
		Entries:           c.services.conversationHub,
		Alerts:            c.services.conversationHub,
		Pauser:            c.services.conversationAutomation,
		AISessions:        c.services.aiAttendanceService,
		Departments:       c.repositories.workspaceDepartment,
		Accounts:          entryInfo,
		Receivers:         c.services.conversationAuth,
	})
	if err != nil {
		log.Fatalf("[inbox-assignment] %v", err)
	}

	c.services.templateSender = templateSender
	c.services.personTemplateSend = conversation_usecase.NewPersonTemplateSend(c.services.conversationAuth, templateSender)

	operatorSendFinalizer, err := conversation_usecase.NewOperatorSendFinalizer(
		conversationStatusUpdater,
		workspaceResolver,
		eventLoggerEarly,
		c.services.aiAttendanceService,
		InitialStageAssigner,
	)
	if err != nil {
		log.Fatalf("[container] %v", err)
	}
	c.services.operatorSendFinalizer = operatorSendFinalizer
	{
		callRegistry := whatsapp_infra.NewInMemoryCallRegistry()
		signaling := whatsapp_infra.NewCallSignalingClient(c.repositories.businessPhone, nil, "")

		var publicMediaIP string
		stunCtx, stunCancel := context.WithTimeout(context.Background(), 10*time.Second)
		ip, err := natdiscovery.PublicIP(stunCtx, c.cfg.WhatsAppStunServers)
		stunCancel()
		if err == nil {
			publicMediaIP = ip
			log.Printf("[whatsapp-calls] discovered public media IP via STUN: %s", ip)
		} else {
			log.Printf("[whatsapp-calls] WARNING: could not resolve public media IP (%v); relying on STUN srflx candidates for the public mapping.", err)
		}

		muxPort := c.cfg.WhatsAppMediaUDPMuxPort
		if bound, err := media.EnableSharedUDPMux(muxPort); err != nil {
			log.Printf("[whatsapp-calls] WARNING: could not bind media UDP mux on :%d (%v), WhatsApp media falls back to per-call sockets, which does NOT scale. Fix WHATSAPP_MEDIA_UDP_MUX_PORT.", muxPort, err)
		} else {
			log.Printf("[whatsapp-calls] WebRTC media UDP mux on port %d, one shared socket for all calls ", bound)
		}

		c.services.whatsappCallSignaling = signaling
		c.services.whatsappCallRegistry = callRegistry
		c.services.whatsappPublicMediaIP = publicMediaIP
		c.services.campaignWorkspaceResolver = workspaceResolver

		whatsappCallSource := conversation_usecase.NewWhatsAppCallSource(
			signaling,
			callRegistry,
			c.repositories.callPermission,
			publicMediaIP,
			c.cfg.WhatsAppStunServers,
			log.Default(),
		)
		c.services.whatsappCallWebhook = conversation_usecase.NewWhatsAppCallWebhookConsumer(callRegistry, lateInboundCalls{handle: c.inboundCallsHandle()})
		c.services.whatsappCallPermissionWebhook = conversation_usecase.NewWhatsAppCallPermissionConsumer(
			c.repositories.businessPhone,
			c.repositories.callPermission,
			c.repositories.wcEntry,
			c.repositories.conversation,
			c.services.messageArrivals,
		)

		callSource := conversation_usecase.NewDispatchingCallSource(whatsappCallSource, c.sipTrunks.CallSource)
		c.services.crmCallSource = callSource
	}
}

func (c *Container) initConversationSenders(turnAssembler *agentturn.Assembler) {
	messageSender := conversation_usecase.NewMessageSenderService(
		c.repositories.conversation,
		c.repositories.lead,
		c.repositories.wcEntry,
		c.repositories.leadMessageWindow,
		c.repositories.conversationMedia,
		c.services.whatsappClientFactory,
		c.services.messageArrivals,
		c.repositories.wcCampaign,
		c.services.ai,
		c.services.toolRegistry,
		c.repositories.stage,
		c.services.analysisScheduler,
		c.liveAdapterRegistry(),
	)
	messageSender.SetCallPermissionRepo(c.repositories.callPermission)
	messageSender.SetMediaLibrary(c.repositories.media)
	messageSender.SetMediaDownloads(mediaDownloads())
	c.services.messageSender = messageSender

	operatorSend, err := conversation_usecase.NewOperatorSendUseCase(
		messageSender,
		c.repositories.user,
		c.services.operatorSendFinalizer,
		c.services.serviceMessageBilling,
		conversation_usecase.NewSendRequestLedger(c.repositories.conversation, c.redisProvider.SharedState()),
	)
	if err != nil {
		log.Fatalf("[container] %v", err)
	}
	c.services.operatorSend = operatorSend
	c.services.liveOperatorSend.Use(operatorSend)
	c.wireMessageActions(messageSender)

	c.services.channelAIReply = conversation_usecase.NewChannelAIReplyService(conversation_usecase.ChannelAIReplyDeps{
		Agents:        c.publishedAgents(),
		AI:            c.agentTurnAI(),
		Messages:      c.repositories.conversation,
		Sender:        messageSender,
		Adapters:      c.liveAdapterRegistry(),
		HandOff:       c.services.assignmentService,
		Events:        ce_usecase.NewLogger(c.services.crmTelemetryPublisher),
		Guard:         c.loopGuard(),
		Assembler:     turnAssembler,
		TurnLock:      c.agentTurnLock(),
		Attendance:    c.services.aiAttendanceService,
		Leads:         c.repositories.lead,
		WorkflowNames: c.actorNames(),
		Variables:     c.automationNeeds().runtime,
	})
}

func (c *Container) wireMessageActions(messageSender *conversation_usecase.MessageSenderService) {
	actions, err := messageaction.NewUseCases(messageaction.Deps{
		Access:   c.services.conversationAuth,
		Messages: c.repositories.conversation,
		Channels: messageaction.Channels{
			Adapters: c.liveAdapterRegistry(),
			Official: messageSender.OfficialReactions(),
		},
		Writer: messagestate.Writer{Messages: c.repositories.conversation, Announcer: c.services.conversationHub},
		Send:   c.services.liveOperatorSend,
	})
	if err != nil {
		log.Fatalf("[container] %v", err)
	}
	c.services.messageActions = actions
}

func (c *Container) mustChannelAIReply() *conversation_usecase.ChannelAIReplyService {
	if c.services.channelAIReply == nil {
		log.Fatal("[container] channel AI reply service read before initConversationSenders; " +
			"channels would silently never answer with an agent")
	}
	return c.services.channelAIReply
}

func (c *Container) startConversationHub() {
	if c.repositories.wcEntry != nil {
		wcEntries := c.repositories.wcEntry
		c.services.conversationAutomation.Register(
			shared.EntryTypeWhatsApp,
			func(_ context.Context, entryID string, enabled *bool) error {
				return wcEntries.UpdateAutomationEnabled(entryID, enabled)
			},
		)
	}

	if err := c.services.conversationHub.BindCommands(wsdelivery.HubCommands{
		Assigner:       c.services.assignmentService,
		Attention:      c.services.assignmentService,
		PersonAssign:   c.services.personAssign,
		Status:         c.services.conversationStatusUpdater,
		PersonFinish:   c.services.personFinish,
		MessageActions: c.services.messageActions,
		PersonalMarks:  c.services.personalInbox,
		TeamChat:       c.teamChat(),
		Inbox:          c.services.inboxService,
	}); err != nil {
		log.Fatalf("[conversation-hub] %v", err)
	}
	go c.services.conversationHub.Run()
}

func (c *Container) registerChannelAdapter(adapter conversation_domain.ChannelAdapter) {
	if adapter == nil {
		return
	}
	if err := channelAdapterPortsGap(adapter); err != nil {
		log.Fatalf("[container] %v", err)
	}
	c.services.channelAdapters = append(c.services.channelAdapters, adapter)
	c.liveAdapterRegistry().Replace(c.services.channelAdapters...)
}

func channelAdapterPortsGap(adapter conversation_domain.ChannelAdapter) error {
	if _, described := adapter.(conversation_domain.CapabilitiesAdapter); !described {
		return fmt.Errorf("the %s channel adapter does not describe its capabilities; agent replies could not respect its text rules", adapter.EntryType())
	}
	if _, batched := adapter.(conversation_domain.WindowStatesAdapter); !batched {
		return fmt.Errorf("the %s channel adapter cannot report window states for a page of entries; its inbox rows would all show the composer closed", adapter.EntryType())
	}
	return nil
}

func (c *Container) liveAdapterRegistry() *conversation_domain.LiveAdapterRegistry {
	if c.services.liveChannelAdapters == nil {
		c.services.liveChannelAdapters = conversation_domain.NewLiveAdapterRegistry()
	}
	return c.services.liveChannelAdapters
}

func logChannelCapabilities(channel string, capabilities map[string]bool) {
	names := make([]string, 0, len(capabilities))
	for name := range capabilities {
		names = append(names, name)
	}
	sort.Strings(names)

	active, missing := make([]string, 0, len(names)), make([]string, 0)
	for _, name := range names {
		if capabilities[name] {
			active = append(active, name)
			continue
		}
		missing = append(missing, name)
	}

	log.Printf("[%s] capabilities: %s", channel, strings.Join(active, ", "))
	if len(missing) > 0 {
		log.Printf("[%s] WARNING: inactive capabilities: %s (these fail silently at runtime)",
			channel, strings.Join(missing, ", "))
	}
}
