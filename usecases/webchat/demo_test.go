package webchat

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"vozko/domain/publicform"
	"vozko/domain/sitepublish"
	wcdomain "vozko/domain/webchat"
)

const previewOrigin = "https://preview--abc.vozko.site"

type fakePreviews struct {
	grants map[string]sitepublish.PreviewGrant
	err    error
	asked  []string
}

func (f *fakePreviews) PreviewOrigin(origin string) bool {
	_, ok := sitepublish.Addresses{Domain: "vozko.site"}.PreviewOriginHost(origin)
	return ok
}

func (f *fakePreviews) PreviewGrant(_ context.Context, origin string) (sitepublish.PreviewGrant, bool, error) {
	f.asked = append(f.asked, origin)
	if f.err != nil {
		return sitepublish.PreviewGrant{}, false, f.err
	}
	g, ok := f.grants[origin]
	return g, ok, nil
}

func (h *harness) withPreview(workspaceID string) *fakePreviews {
	previews := &fakePreviews{grants: map[string]sitepublish.PreviewGrant{
		previewOrigin: {WorkspaceID: workspaceID, ProjectID: "p1", Origin: previewOrigin, Expires: h.now.Add(30 * time.Minute)},
	}}
	h.svc.Previews = previews
	h.svc.PreviewEditors = []string{"https://app.vozko.com"}
	return previews
}

func (h *harness) challengeFrom(t *testing.T, origin string) (string, string) {
	t.Helper()
	c, err := h.svc.Challenge(context.Background(), h.widget.PublicKey, origin)
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; ; n++ {
		if nonce := strconv.Itoa(n); publicform.SolvesChallenge(c.Token, nonce, c.Bits) {
			return c.Token, nonce
		}
	}
}

func (h *harness) demoSession(t *testing.T) (*SessionView, *Session) {
	t.Helper()
	token, nonce := h.challengeFrom(t, previewOrigin)
	view, err := h.svc.StartSession(context.Background(), StartSessionInput{
		PublicKey: h.widget.PublicKey, ChallengeToken: token, Nonce: nonce,
		Client: Client{ParentOrigin: previewOrigin, IP: "203.0.113.7"},
	})
	if err != nil {
		t.Fatal(err)
	}
	s, err := h.svc.Authenticate(context.Background(), view.Token, "203.0.113.7")
	if err != nil {
		t.Fatal(err)
	}
	return view, s
}

func (h *harness) assertNothingPersisted(t *testing.T) {
	t.Helper()
	switch {
	case len(h.visitors.byID) != 0:
		t.Fatalf("visitors = %d", len(h.visitors.byID))
	case len(h.conversations.byID) != 0:
		t.Fatalf("conversations = %d", len(h.conversations.byID))
	case len(h.transcript.records) != 0:
		t.Fatalf("messages = %d", len(h.transcript.records))
	case len(h.assignments.ensured) != 0 || len(h.assignments.handoffs) != 0:
		t.Fatalf("assignments = %v %v", h.assignments.ensured, h.assignments.handoffs)
	case len(h.automation.inputs) != 0:
		t.Fatalf("automation = %d", len(h.automation.inputs))
	case len(h.events.events) != 0:
		t.Fatalf("events = %d", len(h.events.events))
	case len(h.leads.calls) != 0:
		t.Fatalf("leads = %v", h.leads.calls)
	}
}

func TestAValidPreviewOfTheWidgetWorkspaceOpensADemoSessionThatPersistsNothing(t *testing.T) {
	h := newHarness(t, func(w *wcdomain.Widget) {
		w.IntakeName = wcdomain.FieldRequired
		w.AllowHumanRequest = true
		w.AllowAttachments = true
		w.WelcomeMessage = "Oi! Como podemos ajudar?"
	})
	h.withPreview("ws-1")
	view, s := h.demoSession(t)
	if !s.Demo || view.Widget.WelcomeMessage != "Oi! Como podemos ajudar?" || !view.Visitor.IntakePending {
		t.Fatalf("view = %+v session demo = %v", view, s.Demo)
	}
	if !view.ExpiresAt.Equal(h.now.Add(30 * time.Minute)) {
		t.Fatalf("a demo session outlives its preview: %v", view.ExpiresAt)
	}

	state, err := h.svc.SubmitIntake(context.Background(), s, wcdomain.IntakeAnswers{Name: "Ana"})
	if err != nil || state.IntakePending || state.Name != "Ana" {
		t.Fatalf("intake = %+v %v", state, err)
	}
	if _, err := h.svc.SubmitIntake(context.Background(), s, wcdomain.IntakeAnswers{}); err == nil {
		t.Fatal("a demo intake still validates the answers")
	}
	message, err := h.svc.Send(context.Background(), s, SendInput{ClientMessageID: "client-0001", Text: "  Quero um orçamento  "})
	if err != nil || message.Text != "Quero um orçamento" || message.Author != wcdomain.AuthorVisitor {
		t.Fatalf("send = %+v %v", message, err)
	}
	if _, err := h.svc.Send(context.Background(), s, SendInput{ClientMessageID: "client-0002", SelectionID: "opt-1"}); !errors.Is(err, wcdomain.ErrSelectionNotOffered) {
		t.Fatalf("a demo offers no options: %v", err)
	}
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
	file, err := h.svc.Upload(context.Background(), s, UploadInput{ClientMessageID: "client-0003", FileName: "foto.png", Data: png})
	if err != nil || file.Media == nil || file.Media.URL != "" || file.Media.Filename != "foto.png" {
		t.Fatalf("upload = %+v %v", file, err)
	}
	if _, err := h.svc.Upload(context.Background(), s, UploadInput{ClientMessageID: "client-0004", FileName: "x.txt", Data: []byte("plain text")}); !errors.Is(err, wcdomain.ErrAttachmentTypeRefused) {
		t.Fatalf("a demo upload still checks the type: %v", err)
	}
	history, err := h.svc.History(context.Background(), s, HistoryInput{})
	if err != nil || len(history.Messages) != 0 {
		t.Fatalf("history = %+v %v", history, err)
	}
	if err := h.svc.Typing(context.Background(), s, true); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.RequestHuman(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if len(h.svc.Media.(*fakeMedia).stored) != 0 {
		t.Fatal("a demo upload was stored")
	}
	h.assertNothingPersisted(t)
}

func TestADemoSessionResumesOnlyWithADemoTokenOfTheSameWidget(t *testing.T) {
	h := newHarness(t, nil)
	h.withPreview("ws-1")
	view, _ := h.demoSession(t)
	resumed, err := h.svc.StartSession(context.Background(), StartSessionInput{
		PublicKey: h.widget.PublicKey, Token: view.Token, Client: Client{ParentOrigin: previewOrigin},
	})
	if err != nil || resumed.Token == "" {
		t.Fatalf("resume = %v", err)
	}
	real := h.session(t)
	realToken, _ := wcdomain.NewVisitorToken(h.keys, wcdomain.VisitorGrant{WidgetID: h.widget.ID, VisitorID: real.Visitor.ID}, h.now)
	if _, err := h.svc.StartSession(context.Background(), StartSessionInput{
		PublicKey: h.widget.PublicKey, Token: realToken, Client: Client{ParentOrigin: previewOrigin},
	}); err == nil {
		t.Fatal("a preview resumed a real visitor")
	}
	if _, err := h.svc.StartSession(context.Background(), StartSessionInput{
		PublicKey: h.widget.PublicKey, Token: view.Token, Client: Client{ParentOrigin: siteOrigin},
	}); err == nil {
		t.Fatal("a published page resumed a demo visitor")
	}
}

func TestPreviewOriginsWithoutAValidGrantOfTheWorkspaceAreRefused(t *testing.T) {
	cases := map[string]func(*harness){
		"expired or forged grant": func(h *harness) { h.withPreview("ws-1").grants = nil },
		"other workspace":         func(h *harness) { h.withPreview("ws-2") },
		"grant lookup failing":    func(h *harness) { h.withPreview("ws-1").err = errors.New("redis down") },
		"allow listed by a wildcard": func(h *harness) {
			h.widget.AllowedOrigins = append(h.widget.AllowedOrigins, "https://*.vozko.site")
			h.withPreview("ws-1").grants = nil
		},
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, nil)
			arrange(h)
			if _, err := h.svc.Challenge(context.Background(), h.widget.PublicKey, previewOrigin); err == nil {
				t.Fatal("challenge accepted")
			}
			if _, err := h.svc.StartSession(context.Background(), StartSessionInput{
				PublicKey: h.widget.PublicKey, Client: Client{ParentOrigin: previewOrigin},
			}); err == nil {
				t.Fatal("session accepted")
			}
			if _, err := h.svc.WidgetForFrame(context.Background(), h.widget.PublicKey, previewOrigin); err == nil {
				t.Fatal("frame accepted")
			}
			h.assertNothingPersisted(t)
		})
	}
}

func TestPublishedOriginsKeepTheRealSessionWithoutAskingForAGrant(t *testing.T) {
	h := newHarness(t, nil)
	previews := h.withPreview("ws-1")
	s := h.session(t)
	if s.Demo {
		t.Fatal("a published page got a demo session")
	}
	if _, err := h.svc.Send(context.Background(), s, SendInput{ClientMessageID: "client-0001", Text: "Oi"}); err != nil {
		t.Fatal(err)
	}
	if len(h.transcript.records) != 1 || len(h.visitors.byID) != 1 {
		t.Fatalf("records = %d visitors = %d", len(h.transcript.records), len(h.visitors.byID))
	}
	if len(previews.asked) != 0 {
		t.Fatalf("a published origin asked for a grant: %v", previews.asked)
	}
	frame, err := h.svc.WidgetForFrame(context.Background(), h.widget.PublicKey, siteOrigin)
	if err != nil || frame.Demo || frame.Ancestors != h.widget.FrameAncestors() {
		t.Fatalf("frame = %+v %v", frame, err)
	}
}

func TestADemoFrameIsEmbeddableOnlyByItsPreviewAndTheEditor(t *testing.T) {
	h := newHarness(t, nil)
	h.withPreview("ws-1")
	frame, err := h.svc.WidgetForFrame(context.Background(), h.widget.PublicKey, previewOrigin)
	if err != nil || !frame.Demo || frame.Ancestors != previewOrigin+" https://app.vozko.com" {
		t.Fatalf("frame = %+v %v", frame, err)
	}
	h.widget.Status = wcdomain.StatusPaused
	if _, err := h.svc.WidgetForFrame(context.Background(), h.widget.PublicKey, previewOrigin); !errors.Is(err, wcdomain.ErrWidgetPaused) {
		t.Fatalf("a paused widget previews = %v", err)
	}
}

func TestAnExpiredDemoTokenIsRefused(t *testing.T) {
	h := newHarness(t, nil)
	h.withPreview("ws-1")
	view, _ := h.demoSession(t)
	h.now = h.now.Add(31 * time.Minute)
	if _, err := h.svc.Authenticate(context.Background(), view.Token, "203.0.113.7"); !errors.Is(err, wcdomain.ErrVisitorTokenExpired) {
		t.Fatalf("expired demo = %v", err)
	}
}
