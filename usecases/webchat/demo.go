package webchat

import (
	"context"

	"github.com/google/uuid"

	"vozko/domain/sitepublish"
	wcdomain "vozko/domain/webchat"
)

func (s *VisitorService) startDemo(w *wcdomain.Widget, preview sitepublish.PreviewGrant, in StartSessionInput) (*SessionView, error) {
	now := s.Now()
	visitorID := ""
	if resumed, err := wcdomain.VerifyDemoToken(s.Keys, in.Token, now); err == nil && resumed.WidgetID == w.ID {
		visitorID = resumed.VisitorID
	}
	if visitorID == "" {
		if err := s.spendChallenge(w, in.ChallengeToken, in.Nonce, now); err != nil {
			return nil, err
		}
		visitorID = uuid.NewString()
	}
	token, err := wcdomain.NewDemoToken(s.Keys, wcdomain.VisitorGrant{WidgetID: w.ID, VisitorID: visitorID, ExpiresAt: preview.Expires}, now)
	if err != nil {
		return nil, err
	}
	visitor := demoVisitor(w, visitorID)
	return &SessionView{
		Token:     token,
		ExpiresAt: preview.Expires,
		Widget:    publicWidget(w),
		Visitor:   visitorState(w, visitor),
	}, nil
}

func demoVisitor(w *wcdomain.Widget, visitorID string) *wcdomain.Visitor {
	return &wcdomain.Visitor{ID: visitorID, WorkspaceID: w.WorkspaceID, WidgetID: w.ID}
}

func (s *VisitorService) demoSession(ctx context.Context, grant wcdomain.VisitorGrant, ip string) (*Session, error) {
	w, err := s.Widgets.FindByIDUnscoped(ctx, grant.WidgetID)
	if err != nil {
		return nil, wcdomain.ErrVisitorTokenInvalid
	}
	if !w.Serves() {
		return nil, wcdomain.ErrWidgetPaused
	}
	return &Session{Widget: w, Visitor: demoVisitor(w, grant.VisitorID), IPHash: wcdomain.HashIP(s.Keys, ip), Demo: true}, nil
}

func (s *VisitorService) demoIntake(session *Session, answers wcdomain.IntakeAnswers) (VisitorState, error) {
	intake, err := session.Widget.ResolveIntake(answers, wcdomain.Intake{}, s.Now())
	if err != nil {
		return VisitorState{}, err
	}
	return VisitorState{Name: intake.Name, Email: intake.Email, Phone: intake.Phone}, nil
}

func (s *VisitorService) demoSend(session *Session, in SendInput) (*wcdomain.VisitorMessage, error) {
	if err := s.allowMessage(session); err != nil {
		return nil, err
	}
	providerID, err := wcdomain.InboundProviderID(session.Visitor.ID, in.ClientMessageID)
	if err != nil {
		return nil, err
	}
	if in.SelectionID != "" {
		return nil, wcdomain.ErrSelectionNotOffered
	}
	text, err := wcdomain.CleanVisitorText(in.Text)
	if err != nil {
		return nil, err
	}
	return &wcdomain.VisitorMessage{ID: providerID, Author: wcdomain.AuthorVisitor, Text: text, CreatedAt: s.Now()}, nil
}

func (s *VisitorService) demoUpload(session *Session, in UploadInput) (*wcdomain.VisitorMessage, error) {
	attachment, err := s.admitAttachment(session, in)
	if err != nil {
		return nil, err
	}
	return &wcdomain.VisitorMessage{
		ID:        attachment.ProviderID,
		Author:    wcdomain.AuthorVisitor,
		Media:     &wcdomain.VisitorMedia{Kind: string(attachment.MediaType), MimeType: attachment.MimeType, Filename: attachment.Filename},
		CreatedAt: s.Now(),
	}, nil
}
