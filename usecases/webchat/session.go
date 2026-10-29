package webchat

import (
	"context"
	"errors"
	"time"

	"vozko/domain/cache"
	"vozko/domain/conversation"
	"vozko/domain/sitepublish"
	wcdomain "vozko/domain/webchat"
	"vozko/usecases/publicform"
)

const challengeKeyPrefix = "webchat:challenge:"

type VisitorDeps struct {
	Widgets        wcdomain.WidgetRepository
	Visitors       wcdomain.VisitorRepository
	Conversations  wcdomain.ConversationRepository
	Keys           wcdomain.Keys
	OneShot        OneShot
	Limits         Limits
	Leads          LeadFinder
	Transcript     conversation.MessageHistoryManager
	Messages       MessageReader
	Presenter      MessagePresenter
	Media          conversation.MediaStore
	Assignments    Assignments
	Automation     Automation
	Events         wcdomain.EventPublisher
	Operators      OperatorNotifier
	SiteOrigins    SiteOrigins
	Previews       PreviewGrants
	PreviewEditors []string
	Now            func() time.Time
	Async          func(func())
}

type VisitorService struct {
	VisitorDeps
}

func NewVisitorService(deps VisitorDeps) *VisitorService {
	if deps.Now == nil {
		deps.Now = func() time.Time { return time.Now().UTC() }
	}
	if deps.Async == nil {
		deps.Async = func(fn func()) { go fn() }
	}
	return &VisitorService{VisitorDeps: deps}
}

type Session struct {
	Widget  *wcdomain.Widget
	Visitor *wcdomain.Visitor
	IPHash  string
	Demo    bool
}

type Client struct {
	ParentOrigin string
	IP           string
	UserAgent    string
	Locale       string
}

type StartSessionInput struct {
	PublicKey      string
	Token          string
	ChallengeToken string
	Nonce          string
	Identity       string
	Client         Client
}

type PublicIntake struct {
	Name             wcdomain.FieldRule
	Email            wcdomain.FieldRule
	Phone            wcdomain.FieldRule
	PrivacyPolicyURL string
}

type PublicWidget struct {
	Name              string
	AccentColor       string
	Position          wcdomain.Position
	LauncherLabel     string
	WelcomeTitle      string
	WelcomeMessage    string
	TeamName          string
	AssistantName     string
	Intake            PublicIntake
	AllowHumanRequest bool
	AllowAttachments  bool
	IdentityMode      wcdomain.IdentityMode
}

type VisitorState struct {
	IntakePending  bool
	IntakeRequired bool
	Verified       bool
	Name           string
	Email          string
	Phone          string
}

type SessionView struct {
	Token     string
	ExpiresAt time.Time
	Widget    PublicWidget
	Visitor   VisitorState
}

func publicWidget(w *wcdomain.Widget) PublicWidget {
	return PublicWidget{
		Name:           w.Name,
		AccentColor:    w.AccentColor,
		Position:       w.Position,
		LauncherLabel:  w.LauncherLabel,
		WelcomeTitle:   w.WelcomeTitle,
		WelcomeMessage: w.WelcomeMessage,
		TeamName:       w.TeamName,
		AssistantName:  w.AssistantName,
		Intake: PublicIntake{
			Name:             w.IntakeName,
			Email:            w.IntakeEmail,
			Phone:            w.IntakePhone,
			PrivacyPolicyURL: w.PrivacyPolicyURL,
		},
		AllowHumanRequest: w.AllowHumanRequest,
		AllowAttachments:  w.AllowAttachments,
		IdentityMode:      w.IdentityMode,
	}
}

func visitorState(w *wcdomain.Widget, v *wcdomain.Visitor) VisitorState {
	return VisitorState{
		IntakePending:  v.IntakePending(w),
		IntakeRequired: v.MustCompleteIntake(w),
		Verified:       v.IdentityVerified,
		Name:           v.Name,
		Email:          v.Email,
		Phone:          v.Phone,
	}
}

func (s *VisitorService) widgetByKey(ctx context.Context, publicKey string) (*wcdomain.Widget, error) {
	w, err := s.Widgets.FindByPublicKey(ctx, publicKey)
	if err != nil {
		return nil, err
	}
	if s.SiteOrigins == nil {
		w.AcceptSiteOrigins(nil)
		return w, nil
	}
	origins, err := s.SiteOrigins.WidgetOrigins(ctx, w.WorkspaceID, w.ID)
	if err != nil {
		return nil, err
	}
	w.AcceptSiteOrigins(origins)
	return w, nil
}

func (s *VisitorService) servingWidget(ctx context.Context, publicKey string) (*wcdomain.Widget, error) {
	w, err := s.widgetByKey(ctx, publicKey)
	if err != nil {
		return nil, err
	}
	if !w.Serves() {
		return nil, wcdomain.ErrWidgetPaused
	}
	return w, nil
}

func (s *VisitorService) previewOf(ctx context.Context, w *wcdomain.Widget, origin string) (*sitepublish.PreviewGrant, error) {
	if s.Previews == nil || !s.Previews.PreviewOrigin(origin) {
		return nil, nil
	}
	grant, found, err := s.Previews.PreviewGrant(ctx, origin)
	if err != nil {
		return nil, err
	}
	if !found || grant.WorkspaceID != w.WorkspaceID {
		return nil, wcdomain.ErrOriginNotAllowed
	}
	return &grant, nil
}

func (s *VisitorService) servedWidget(ctx context.Context, publicKey, parentOrigin string) (*wcdomain.Widget, *sitepublish.PreviewGrant, error) {
	w, err := s.servingWidget(ctx, publicKey)
	if err != nil {
		return nil, nil, err
	}
	preview, err := s.previewOf(ctx, w, parentOrigin)
	if err != nil {
		return nil, nil, err
	}
	if preview == nil && !w.AllowsOrigin(parentOrigin) {
		return nil, nil, wcdomain.ErrOriginNotAllowed
	}
	return w, preview, nil
}

type Frame struct {
	Widget    *wcdomain.Widget
	Demo      bool
	Ancestors string
}

func (s *VisitorService) WidgetForFrame(ctx context.Context, publicKey, origin string) (*Frame, error) {
	w, err := s.servingWidget(ctx, publicKey)
	if err != nil {
		return nil, err
	}
	preview, err := s.previewOf(ctx, w, origin)
	if err != nil {
		return nil, err
	}
	if preview != nil {
		return &Frame{Widget: w, Demo: true, Ancestors: wcdomain.PreviewFrameAncestors(preview.Origin, s.PreviewEditors)}, nil
	}
	return &Frame{Widget: w, Ancestors: w.FrameAncestors()}, nil
}

func (s *VisitorService) Challenge(ctx context.Context, publicKey, parentOrigin string) (wcdomain.Challenge, error) {
	w, _, err := s.servedWidget(ctx, publicKey, parentOrigin)
	if err != nil {
		return wcdomain.Challenge{}, err
	}
	challenge, err := s.challenges().Issue(w.ID, s.Now())
	return challenge, wcdomain.PublicFormRefusal(err)
}

func (s *VisitorService) challenges() publicform.ChallengeGate {
	return publicform.ChallengeGate{Key: s.Keys.Challenge, Spent: s.OneShot, KeyPrefix: challengeKeyPrefix, Bits: wcdomain.ChallengeBits}
}

func (s *VisitorService) StartSession(ctx context.Context, in StartSessionInput) (*SessionView, error) {
	w, preview, err := s.servedWidget(ctx, in.PublicKey, in.Client.ParentOrigin)
	if err != nil {
		return nil, err
	}
	if preview != nil {
		return s.startDemo(w, *preview, in)
	}
	now := s.Now()
	ipHash := wcdomain.HashIP(s.Keys, in.Client.IP)

	visitor, err := s.resolveVisitor(ctx, w, in, ipHash, now)
	if err != nil {
		return nil, err
	}
	if visitor.Blocked {
		return nil, wcdomain.ErrVisitorBlocked
	}

	sighting := wcdomain.VisitorSighting{
		At:         now,
		IPHash:     ipHash,
		UserAgent:  wcdomain.TruncateUserAgent(in.Client.UserAgent),
		Locale:     in.Client.Locale,
		PageOrigin: in.Client.ParentOrigin,
	}
	if err := s.Visitors.Touch(ctx, visitor.ID, sighting); err != nil {
		return nil, err
	}

	token, err := wcdomain.NewVisitorToken(s.Keys, wcdomain.VisitorGrant{WidgetID: w.ID, VisitorID: visitor.ID}, now)
	if err != nil {
		return nil, err
	}
	return &SessionView{
		Token:     token,
		ExpiresAt: now.Add(wcdomain.VisitorTokenTTL),
		Widget:    publicWidget(w),
		Visitor:   visitorState(w, visitor),
	}, nil
}

func (s *VisitorService) resolveVisitor(ctx context.Context, w *wcdomain.Widget, in StartSessionInput, ipHash string, now time.Time) (*wcdomain.Visitor, error) {
	claims, verified, err := s.identity(w, in.Identity, now)
	if err != nil {
		return nil, err
	}
	if verified {
		return s.identifiedVisitor(ctx, w, claims, ipHash)
	}

	if in.Token != "" {
		if v, err := s.visitorFromToken(ctx, w, in.Token, now); err == nil && v.ExternalID == nil {
			return v, nil
		}
	}

	if err := s.spendChallenge(w, in.ChallengeToken, in.Nonce, now); err != nil {
		return nil, err
	}
	if err := s.allowNewVisitor(w, ipHash); err != nil {
		return nil, err
	}
	v := &wcdomain.Visitor{WorkspaceID: w.WorkspaceID, WidgetID: w.ID}
	if err := s.Visitors.Create(ctx, v); err != nil {
		return nil, err
	}
	return v, nil
}

func (s *VisitorService) identity(w *wcdomain.Widget, token string, now time.Time) (wcdomain.IdentityClaims, bool, error) {
	if !w.IdentityMode.Verifies() {
		return wcdomain.IdentityClaims{}, false, nil
	}
	if token == "" {
		if w.IdentityMode == wcdomain.IdentityRequired {
			return wcdomain.IdentityClaims{}, false, wcdomain.ErrIdentityRequired
		}
		return wcdomain.IdentityClaims{}, false, nil
	}
	claims, err := wcdomain.VerifyIdentityToken(w.IdentitySecret, token, now)
	if err != nil {
		return wcdomain.IdentityClaims{}, false, err
	}
	claims.Phone = wcdomain.NormalizePhone(claims.Phone, w.DefaultCountryCode)
	return claims, true, nil
}

func (s *VisitorService) identifiedVisitor(ctx context.Context, w *wcdomain.Widget, claims wcdomain.IdentityClaims, ipHash string) (*wcdomain.Visitor, error) {
	v, err := s.Visitors.FindByExternalID(ctx, w.ID, claims.ExternalID)
	if errors.Is(err, wcdomain.ErrVisitorNotFound) {
		v, err = s.createIdentifiedVisitor(ctx, w, claims.ExternalID, ipHash)
	}
	if err != nil {
		return nil, err
	}
	if err := s.Visitors.ApplyIdentity(ctx, v.ID, claims); err != nil {
		return nil, err
	}
	return s.Visitors.FindByID(ctx, v.ID)
}

func (s *VisitorService) createIdentifiedVisitor(ctx context.Context, w *wcdomain.Widget, externalID, ipHash string) (*wcdomain.Visitor, error) {
	if err := s.allowNewVisitor(w, ipHash); err != nil {
		return nil, err
	}
	v := &wcdomain.Visitor{WorkspaceID: w.WorkspaceID, WidgetID: w.ID, ExternalID: &externalID}
	if err := s.Visitors.Create(ctx, v); err != nil {
		if existing, findErr := s.Visitors.FindByExternalID(ctx, w.ID, externalID); findErr == nil {
			return existing, nil
		}
		return nil, err
	}
	return v, nil
}

func (s *VisitorService) visitorFromToken(ctx context.Context, w *wcdomain.Widget, token string, now time.Time) (*wcdomain.Visitor, error) {
	grant, err := wcdomain.VerifyVisitorToken(s.Keys, token, now)
	if err != nil {
		return nil, err
	}
	if grant.WidgetID != w.ID {
		return nil, wcdomain.ErrVisitorTokenInvalid
	}
	v, err := s.Visitors.FindByID(ctx, grant.VisitorID)
	if err != nil {
		return nil, err
	}
	if v.WidgetID != w.ID {
		return nil, wcdomain.ErrVisitorTokenInvalid
	}
	return v, nil
}

func (s *VisitorService) spendChallenge(w *wcdomain.Widget, token, nonce string, now time.Time) error {
	_, err := s.challenges().Admit(token, nonce, w.ID, now)
	return wcdomain.PublicFormRefusal(err)
}

func (s *VisitorService) allowNewVisitor(w *wcdomain.Widget, ipHash string) error {
	if err := allow(s.Limits.SessionsPerIP, w.ID+":"+ipHash); err != nil {
		return err
	}
	return allow(s.Limits.SessionsPerWidget, w.ID)
}

func (s *VisitorService) Authenticate(ctx context.Context, token, ip string) (*Session, error) {
	grant, err := wcdomain.VerifyVisitorToken(s.Keys, token, s.Now())
	if errors.Is(err, wcdomain.ErrVisitorTokenInvalid) {
		if demo, demoErr := wcdomain.VerifyDemoToken(s.Keys, token, s.Now()); !errors.Is(demoErr, wcdomain.ErrVisitorTokenInvalid) {
			if demoErr != nil {
				return nil, demoErr
			}
			return s.demoSession(ctx, demo, ip)
		}
	}
	if err != nil {
		return nil, err
	}
	w, err := s.Widgets.FindByIDUnscoped(ctx, grant.WidgetID)
	if err != nil {
		return nil, wcdomain.ErrVisitorTokenInvalid
	}
	if !w.Serves() {
		return nil, wcdomain.ErrWidgetPaused
	}
	v, err := s.Visitors.FindByID(ctx, grant.VisitorID)
	if err != nil {
		return nil, wcdomain.ErrVisitorTokenInvalid
	}
	if v.WidgetID != w.ID {
		return nil, wcdomain.ErrVisitorTokenInvalid
	}
	if v.Blocked {
		return nil, wcdomain.ErrVisitorBlocked
	}
	return &Session{Widget: w, Visitor: v, IPHash: wcdomain.HashIP(s.Keys, ip)}, nil
}

func allow(limiter cache.RateLimiter, key string) error {
	return wcdomain.PublicFormRefusal(publicform.Allow(limiter, key))
}
