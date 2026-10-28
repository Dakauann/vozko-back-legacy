package googlesheet

import (
	"context"
	"errors"
	"log"
	"time"

	"vozko/domain/cache"
	"vozko/domain/googleaccount"
	"vozko/domain/googlesheet"
	"vozko/domain/shared"
)

const (
	defaultSettleWait = 15 * time.Second
	finishBudget      = 90 * time.Second
	expiryBatch       = 50
)

type EditView struct {
	Session   *googlesheet.EditSession
	StartedBy *NamedRef
	EndedBy   *NamedRef
}

type EditorView struct {
	Editing      *EditView
	AccountEmail string
}

type EditSessionsDeps struct {
	Manager  *Manager
	Sessions googlesheet.EditSessionRepository
	Leases   cache.Locker
}

type EditSessions struct {
	m          *Manager
	sessions   googlesheet.EditSessionRepository
	leases     cache.Locker
	settleWait time.Duration
}

func NewEditSessions(d EditSessionsDeps) *EditSessions {
	if d.Manager == nil || d.Sessions == nil || d.Leases == nil {
		panic("googlesheet: edit sessions require the manager, the session records and sheet leases")
	}
	return &EditSessions{m: d.Manager, sessions: d.Sessions, leases: d.Leases, settleWait: defaultSettleWait}
}

type StartEditInput struct {
	WorkspaceID   string
	SpreadsheetID string
	UserID        string
}

type EditInput struct {
	WorkspaceID string
	SessionID   string
	UserID      string
}

type EndEditInput struct {
	WorkspaceID string
	SessionID   string
	UserID      string
	Reason      googlesheet.EditEndReason
}

func (e *EditSessions) fileSheets(ctx context.Context, workspaceID, spreadsheetID string) ([]*googlesheet.Sheet, error) {
	sheets, err := e.m.sheets.InSpreadsheet(ctx, workspaceID, spreadsheetID)
	if err != nil {
		return nil, err
	}
	if len(sheets) == 0 {
		return nil, googlesheet.ErrSpreadsheetNotFound
	}
	return sheets, nil
}

func (e *EditSessions) Editor(ctx context.Context, workspaceID, spreadsheetID string) (*EditorView, error) {
	if _, err := e.fileSheets(ctx, workspaceID, spreadsheetID); err != nil {
		return nil, err
	}
	editing, err := e.m.editing(ctx, workspaceID, spreadsheetID)
	if err != nil {
		return nil, err
	}
	view := &EditorView{Editing: editing}
	access, err := e.m.access.Access(ctx, workspaceID, googleaccount.ProductSheets)
	switch {
	case errors.Is(err, googleaccount.ErrNotConnected):
	case err != nil:
		return nil, err
	default:
		view.AccountEmail = access.Email
	}
	return view, nil
}

func (e *EditSessions) Start(ctx context.Context, in StartEditInput) (*EditView, error) {
	sheets, err := e.fileSheets(ctx, in.WorkspaceID, in.SpreadsheetID)
	if err != nil {
		return nil, err
	}
	now := e.m.now()
	current, found, err := e.sessions.Current(ctx, in.WorkspaceID, in.SpreadsheetID)
	if err != nil {
		return nil, err
	}
	if found && !current.HoldsAt(now) {
		if err := e.expire(ctx, current, now); err != nil {
			return nil, err
		}
	}
	session, err := googlesheet.NewEditSession(in.WorkspaceID, in.SpreadsheetID, in.UserID, now)
	if err != nil {
		return nil, err
	}
	if err := e.sessions.Start(ctx, session); err != nil {
		return nil, e.m.named(ctx, err)
	}
	if err := e.settle(ctx, session, sheets, e.settleWait); err != nil {
		log.Printf("[googlesheet] edit session %s could not wait for the writes in flight: %v", session.ID, err)
	}
	return e.m.editView(ctx, session)
}

func (e *EditSessions) settle(ctx context.Context, session *googlesheet.EditSession, sheets []*googlesheet.Sheet, wait time.Duration) error {
	deadline := time.Now().Add(wait)
	for _, sheet := range sheets {
		release, busy, err := acquireSheetWithin(ctx, e.leases, sheet.ID, max(time.Until(deadline), leaseProbe))
		if err != nil || busy {
			return err
		}
		release()
	}
	session.Settle(e.m.now())
	return e.sessions.Settle(ctx, session)
}

func (e *EditSessions) Heartbeat(ctx context.Context, in EditInput) (*EditView, error) {
	session, err := e.sessions.Find(ctx, in.WorkspaceID, in.SessionID)
	if err != nil {
		return nil, err
	}
	if session.StartedBy != in.UserID {
		return nil, googlesheet.ErrEditSessionNotYours
	}
	if session.Ended() {
		return e.m.editView(ctx, session)
	}
	now := e.m.now()
	if err := session.Extend(now); err != nil {
		return e.ended(ctx, session, e.expire(ctx, session, now))
	}
	if err := e.sessions.Extend(ctx, session, now); errors.Is(err, googlesheet.ErrEditSessionEnded) {
		return e.reload(ctx, session)
	} else if err != nil {
		return nil, err
	}
	if !session.Settled() {
		sheets, err := e.fileSheets(ctx, session.WorkspaceID, session.SpreadsheetID)
		if err != nil {
			return nil, err
		}
		if err := e.settle(ctx, session, sheets, leaseProbe); err != nil {
			return nil, err
		}
	}
	return e.m.editView(ctx, session)
}

func (e *EditSessions) End(ctx context.Context, in EndEditInput) (*EditView, error) {
	session, err := e.sessions.Find(ctx, in.WorkspaceID, in.SessionID)
	if err != nil {
		return nil, err
	}
	if session.Ended() {
		return e.m.editView(ctx, session)
	}
	if err := session.End(googlesheet.EditEnd{Reason: in.Reason, By: in.UserID, At: e.m.now()}); err != nil {
		return nil, err
	}
	return e.ended(ctx, session, e.finish(ctx, session))
}

func (e *EditSessions) Execute(ctx context.Context) error {
	now := e.m.now()
	stale, err := e.sessions.Expired(ctx, now, expiryBatch)
	if err != nil {
		return err
	}
	var errs []error
	for _, session := range stale {
		errs = append(errs, e.expire(ctx, session, now))
	}
	return errors.Join(errs...)
}

func (e *EditSessions) History(ctx context.Context, workspaceID, spreadsheetID string, pagination shared.Pagination) (*shared.PaginatedResult[EditView], error) {
	if _, err := e.fileSheets(ctx, workspaceID, spreadsheetID); err != nil {
		return nil, err
	}
	pagination = shared.NormalizePagination(pagination)
	page, err := e.sessions.History(ctx, workspaceID, spreadsheetID, pagination)
	if err != nil {
		return nil, err
	}
	people := newNamedLookup("person", e.m.names.People)
	views := make([]EditView, len(page.Items))
	for i, session := range page.Items {
		view, err := e.m.editViewWith(ctx, people, session)
		if err != nil {
			return nil, err
		}
		views[i] = *view
	}
	return &shared.PaginatedResult[EditView]{Items: views, Page: page.Page, PageSize: page.PageSize, TotalItems: page.TotalItems, TotalPages: page.TotalPages}, nil
}

func (e *EditSessions) expire(ctx context.Context, session *googlesheet.EditSession, now time.Time) error {
	if err := session.End(googlesheet.EditEnd{Reason: googlesheet.EditExpired, At: now}); err != nil {
		return err
	}
	return e.finish(ctx, session)
}

func (e *EditSessions) finish(ctx context.Context, session *googlesheet.EditSession) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finishBudget)
	defer cancel()
	ended, err := e.sessions.End(ctx, session)
	if err != nil {
		return err
	}
	if !ended {
		stored, err := e.sessions.Find(ctx, session.WorkspaceID, session.ID)
		if err != nil {
			return err
		}
		*session = *stored
		return nil
	}
	check, err := e.m.inspectFile(ctx, session.WorkspaceID, session.SpreadsheetID)
	if err != nil {
		log.Printf("[googlesheet] the check after edit session %s did not finish: %v", session.ID, err)
		check = googlesheet.UncheckedEdit("a verificação não terminou; as gravações seguem conferindo cada aba antes de gravar")
	}
	session.Check = check
	if err := e.sessions.SaveCheck(ctx, session); err != nil {
		return err
	}
	e.m.resumeFineTabs(check)
	return nil
}

func (e *EditSessions) ended(ctx context.Context, session *googlesheet.EditSession, err error) (*EditView, error) {
	if err != nil {
		return nil, err
	}
	return e.m.editView(ctx, session)
}

func (e *EditSessions) reload(ctx context.Context, session *googlesheet.EditSession) (*EditView, error) {
	stored, err := e.sessions.Find(ctx, session.WorkspaceID, session.ID)
	if err != nil {
		return nil, err
	}
	return e.m.editView(ctx, stored)
}

func (m *Manager) editFree(ctx context.Context, workspaceID, spreadsheetID string) error {
	return m.named(ctx, m.edits.check(ctx, workspaceID, spreadsheetID, m.now()))
}

func (m *Manager) named(ctx context.Context, err error) error {
	var taken *googlesheet.EditTakenError
	if !errors.As(err, &taken) {
		return err
	}
	holder, lookupErr := newNamedLookup("person", m.names.People).ref(ctx, taken.Session.WorkspaceID, taken.Session.StartedBy)
	if lookupErr != nil {
		log.Printf("[googlesheet] name of the person editing %s not found: %v", taken.Session.SpreadsheetID, lookupErr)
	}
	if holder != nil {
		taken.HolderName = holder.Name
	}
	return err
}

func (m *Manager) editing(ctx context.Context, workspaceID, spreadsheetID string) (*EditView, error) {
	session, err := m.edits.holding(ctx, workspaceID, spreadsheetID, m.now())
	if err != nil || session == nil {
		return nil, err
	}
	return m.editView(ctx, session)
}

func (m *Manager) editView(ctx context.Context, session *googlesheet.EditSession) (*EditView, error) {
	return m.editViewWith(ctx, newNamedLookup("person", m.names.People), session)
}

func (m *Manager) editViewWith(ctx context.Context, people *namedLookup, session *googlesheet.EditSession) (*EditView, error) {
	startedBy, err := people.ref(ctx, session.WorkspaceID, session.StartedBy)
	if err != nil {
		return nil, err
	}
	endedBy, err := people.ref(ctx, session.WorkspaceID, session.EndedBy)
	if err != nil {
		return nil, err
	}
	return &EditView{Session: session, StartedBy: startedBy, EndedBy: endedBy}, nil
}
