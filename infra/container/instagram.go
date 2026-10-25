package container

import (
	"context"
	"log"
	"vozko/domain/webhook"
	webhook_repository "vozko/infra/repositories/webhook"

	instagramhttp "vozko/delivery/http/instagram"
	conversation_domain "vozko/domain/conversation"
	igdomain "vozko/domain/instagram"
	"vozko/domain/shared"
	"vozko/domain/shared/deps"
	iginfra "vozko/infra/instagram"
	instagram_repository "vozko/infra/repositories/instagram"
	conversation_usecase "vozko/usecases/conversation"
	iguc "vozko/usecases/instagram"
)

type instagramBundle struct {
	Accounts        igdomain.AccountRepository
	Contacts        igdomain.ContactRepository
	Conversations   igdomain.ConversationRepository
	Media           igdomain.MediaRepository
	Comments        igdomain.CommentRepository
	ProcessedEvent  webhook.ProcessedEventRepository
	CommentRuleEval *iguc.CommentRules
	PrivateReplyUC  *iguc.SendPrivateReplyUseCase
	ModerateComment *iguc.ModerateCommentUseCase
	ReplyComment    *iguc.ReplyToCommentUseCase
	CommentAnalysis *deps.Handle[iguc.CommentForgetter]

	OAuth        igdomain.OAuthService
	Messaging    igdomain.MessagingService
	MediaService igdomain.MediaService
	CommentSvc   igdomain.CommentService
	Subscription igdomain.SubscriptionService

	Handler        *instagramhttp.Handler
	WebhookHandler *instagramhttp.WebhookHandler

	Consume       *iguc.ConsumeWebhookUseCase
	RefreshTokens *iguc.RefreshTokensUseCase

	WebhookSecrets []string
}

func (c *Container) initInstagram() {
	bundle := &instagramBundle{}
	c.instagram = bundle

	if err := igdomain.ValidateRedirectURI(c.cfg.InstagramRedirectURI); err != nil {
		log.Fatalf("[instagram] %v", err)
	}

	graphCfg := iginfra.GraphConfig{
		GraphVersion: c.cfg.InstagramGraphVersion,
		AppSecret:    c.cfg.InstagramAppSecret,
	}

	mediaSvc, err := iginfra.NewMediaService(graphCfg)
	if err != nil {
		log.Fatalf("[instagram] media client: %v", err)
	}
	commentSvc, err := iginfra.NewCommentService(graphCfg)
	if err != nil {
		log.Fatalf("[instagram] comment client: %v", err)
	}
	subscriptionSvc, err := iginfra.NewSubscriptionService(graphCfg)
	if err != nil {
		log.Fatalf("[instagram] subscription client: %v", err)
	}
	messagingSvc, err := iginfra.NewMessagingService(iginfra.MessagingConfig{
		GraphVersion:       c.cfg.InstagramGraphVersion,
		AppSecret:          c.cfg.InstagramAppSecret,
		RateLimiterFactory: c.redisProvider.RateLimiterFactory(),
	})
	if err != nil {
		log.Fatalf("[instagram] messaging client: %v", err)
	}

	bundle.OAuth = iginfra.NewOAuthService(iginfra.OAuthConfig{
		AppID:        c.cfg.InstagramAppID,
		AppSecret:    c.cfg.InstagramAppSecret,
		RedirectURI:  c.cfg.InstagramRedirectURI,
		GraphVersion: c.cfg.InstagramGraphVersion,
	})
	bundle.Messaging = messagingSvc
	bundle.MediaService = mediaSvc
	bundle.CommentSvc = commentSvc
	bundle.Subscription = subscriptionSvc

	bundle.Accounts = instagram_repository.NewAccountRepository(c.db)
	bundle.Contacts = instagram_repository.NewContactRepository(c.db)
	bundle.Conversations = instagram_repository.NewConversationRepository(c.db)
	bundle.Media = instagram_repository.NewMediaRepository(c.db)
	bundle.Comments = instagram_repository.NewCommentRepository(c.db)
	bundle.ProcessedEvent = webhook_repository.NewProcessedEventRepository(c.db)

	bundle.WebhookSecrets = append([]string{c.cfg.InstagramAppSecret}, c.cfg.MetaAppSecret)
	bundle.WebhookSecrets = append(bundle.WebhookSecrets, c.cfg.MetaAppSecretsExtra...)

	connect := iguc.NewConnectAccountUseCase(
		bundle.OAuth,
		bundle.Subscription,
		bundle.Messaging,
		bundle.Accounts,
		c.mustOAuthStateIssuer("ig:oauth", c.cfg.InstagramAppSecret, "/dashboard/instagram-accounts"),
	)

	replyComment := iguc.NewReplyToCommentUseCase(bundle.Accounts, commentSvc, bundle.Comments)
	bundle.CommentAnalysis = deps.Tracked[iguc.CommentForgetter](&c.handles, "instagram comment analysis")
	moderateComment, err := iguc.NewModerateCommentUseCase(bundle.Accounts, commentSvc, bundle.Comments, lateCommentForgetter{analysis: bundle.CommentAnalysis})
	if err != nil {
		log.Fatalf("[instagram] %v", err)
	}
	automation := c.commentAutomation()
	automation.Manager.Register(shared.EntryTypeInstagram, iguc.NewAccountOwnership(bundle.Accounts))
	privateReply, err := iguc.NewSendPrivateReplyUseCase(
		bundle.Accounts, bundle.Messaging, commentSvc, bundle.Comments,
		automation.Sender, bundle.Contacts, bundle.Conversations, c.messageHistoryManager(),
	)
	if err != nil {
		log.Fatalf("[instagram] %v", err)
	}
	bundle.PrivateReplyUC = privateReply
	bundle.ModerateComment = moderateComment
	bundle.ReplyComment = replyComment
	bundle.CommentRuleEval = iguc.NewCommentRules(
		automation.Evaluator,
		iguc.NewCommentActionRunner(replyComment, privateReply, moderateComment),
	)

	bundle.Handler = instagramhttp.NewHandler(instagramhttp.HandlerDeps{
		Connect:           connect,
		List:              iguc.NewListAccountsUseCase(bundle.Accounts),
		Get:               iguc.NewGetAccountUseCase(bundle.Accounts),
		UpdateCfg:         iguc.NewUpdateAccountConfigUseCase(bundle.Accounts),
		Disconnect:        iguc.NewDisconnectAccountUseCase(bundle.Accounts, bundle.Subscription),
		ListMedia:         iguc.NewListMediaUseCase(bundle.Accounts, mediaSvc, bundle.Media),
		GetMedia:          iguc.NewGetMediaUseCase(bundle.Accounts, mediaSvc, bundle.Media),
		ProxyMedia:        iguc.NewProxyMediaUseCase(bundle.Accounts, mediaSvc),
		ProxyAvatar:       iguc.NewProxyAvatarUseCase(bundle.Accounts, bundle.OAuth, mediaSvc),
		CreateMedia:       iguc.NewCreateMediaUseCase(bundle.Accounts, mediaSvc, bundle.Media),
		SetCommentEnabled: iguc.NewSetCommentEnabledUseCase(bundle.Accounts, mediaSvc, bundle.Media),
		ListComments:      iguc.NewListCommentsUseCase(bundle.Accounts, commentSvc, bundle.Comments),
		ReplyComment:      replyComment,
		Moderate:          moderateComment,
		PrivateReply:      privateReply,
		ManageRules:       automation.Manager,
		FrontendBaseURL:   c.cfg.FrontendBaseURL,
	})

	bundle.RefreshTokens = iguc.NewRefreshTokensUseCase(bundle.Accounts, bundle.OAuth)

	log.Printf("[instagram] channel enabled (graph=%s/%s)", iginfra.GraphHost, iginfra.DefaultGraphVersion)

	log.Printf("[instagram] redirect_uri sent to Instagram: %q (must match the dashboard byte-for-byte, including any trailing slash)",
		c.cfg.InstagramRedirectURI)
}

func (c *Container) initInstagramRuntime(history conversation_domain.MessageHistoryManager) {
	bundle := c.instagram
	if bundle == nil {
		return
	}

	if c.useCases == nil {
		log.Fatalf("[instagram] runtime wiring ran before useCases were built")
	}

	bundle.WebhookHandler = instagramhttp.NewWebhookHandler(
		c.useCases.publishWebhook,
		bundle.WebhookSecrets,
		c.cfg.InstagramWebhookVerifyToken,
	)

	handler := iguc.NewHandleWebhookUseCase(iguc.HandleWebhookDeps{
		Accounts:      bundle.Accounts,
		Contacts:      bundle.Contacts,
		Conversations: bundle.Conversations,
		Comments:      bundle.Comments,
		Media:         bundle.Media,
		Messaging:     bundle.Messaging,
		MediaFetcher:  bundle.MediaService,
		History:       history,
		Messages:      c.repositories.conversation,
		Attachments:   c.mediaStore(),
		AdOrigins:     c.adOriginRecorder(),
		Broadcaster:   c.services.conversationHub,
		Assignments:   c.services.assignmentService,
		AIReply:       c.mustChannelAIReply(),
		Workflows:     c.useCases.triggerEvaluator,
		CommentRules:  bundle.CommentRuleEval,
		Analysis:      c.services.analysisScheduler,
		Delegations:   c.services.delegations,
		Audience:      audienceEnqueuer(c),
	})

	bundle.Consume = iguc.NewConsumeWebhookUseCase(
		c.services.webhookQueueSub,
		c.services.webhookQueuePub,
		c.redisProvider.SharedState(),
		bundle.ProcessedEvent,
		handler,
	)
}

func (c *Container) wireInstagramConversationStack() {
	bundle := c.instagram
	if bundle == nil {
		return
	}

	adapter := iguc.NewChannelAdapter(
		bundle.Accounts,
		bundle.Contacts,
		bundle.Conversations,
		bundle.Messaging,
	)

	c.registerConversationChannel(conversationChannel{
		EntryType:     shared.EntryTypeInstagram,
		Adapter:       adapter,
		Conversations: bundle.Conversations,
		Automation: func(ctx context.Context, entryID string) (*bool, error) {
			conv, err := bundle.Conversations.FindByID(ctx, entryID)
			if err != nil {
				return nil, err
			}
			return conv.AutomationEnabled, nil
		},
		Contacts: instagramContactIdentity(bundle),
	})
}

func instagramContactIdentity(bundle *instagramBundle) conversation_usecase.ContactIdentityLookup {
	contacts, conversations := bundle.Contacts, bundle.Conversations

	display := func(c *igdomain.Contact) conversation_usecase.ContactDisplay {
		return conversation_usecase.ContactDisplay{
			ContactID:  c.ID,
			Ref:        c.IGSID,
			Handle:     c.Username,
			Name:       c.Name,
			PictureURL: c.ProfilePictureURL,
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

func instagramHandler(c *Container) *instagramhttp.Handler {
	if c.instagram == nil {
		return nil
	}
	return c.instagram.Handler
}

func instagramWebhookHandler(c *Container) *instagramhttp.WebhookHandler {
	if c.instagram == nil {
		return nil
	}
	return c.instagram.WebhookHandler
}

type lateCommentForgetter struct {
	analysis *deps.Handle[iguc.CommentForgetter]
}

func (l lateCommentForgetter) Forget(ctx context.Context, igCommentID string) {
	analysis, err := l.analysis.Get()
	if err != nil {
		log.Printf("[instagram] CRITICAL: comment %s stays in the audience analysis: %v", igCommentID, err)
		return
	}
	analysis.Forget(ctx, igCommentID)
}
