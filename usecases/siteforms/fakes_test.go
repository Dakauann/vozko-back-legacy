package siteforms

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strconv"
	"sync"
	"time"

	ads "vozko/domain/advertising"
	"vozko/domain/conversation"
	"vozko/domain/emailconsent"
	"vozko/domain/lead"
	"vozko/domain/notification"
	"vozko/domain/opportunity"
	"vozko/domain/shortlink"
	"vozko/domain/sitesubmission"
	wo "vozko/domain/whatsapp_outreach"
	emailconsent_usecase "vozko/usecases/emailconsent"
	"vozko/usecases/emailinbox"
	"vozko/usecases/formlead"
	lead_usecase "vozko/usecases/lead"
	opportunity_usecase "vozko/usecases/opportunity"
	uwuc "vozko/usecases/unofficial_whatsapp"
)

var testNow = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

type formShelf struct {
	keys        map[string]map[string]string
	active      map[string]ActiveForm
	activations []Activation
	deactivated []string
	embeds      map[string][]string
	failKeys    error
	failList    error
	owners      map[string]string
	names       map[string]string
}

func newFormShelf() *formShelf {
	return &formShelf{keys: map[string]map[string]string{}, active: map[string]ActiveForm{}, embeds: map[string][]string{}, owners: map[string]string{}}
}

func (f *formShelf) EnsureKeys(_ context.Context, workspaceID, projectID string, proposed map[string]string) (map[string]string, error) {
	if f.failKeys != nil {
		return nil, f.failKeys
	}
	if f.keys[projectID] == nil {
		f.keys[projectID] = map[string]string{}
	}
	f.owners[projectID] = workspaceID
	out := map[string]string{}
	for formID, key := range proposed {
		if existing, ok := f.keys[projectID][formID]; ok {
			out[formID] = existing
			continue
		}
		f.keys[projectID][formID] = key
		out[formID] = key
	}
	return out, nil
}

func (f *formShelf) Activate(_ context.Context, a Activation) error {
	for key, form := range f.active {
		if form.ProjectID == a.ProjectID {
			delete(f.active, key)
		}
	}
	for _, form := range a.Forms {
		f.active[form.FormKey] = form
	}
	f.activations = append(f.activations, a)
	f.embeds[a.ProjectID] = a.Widgets
	return nil
}

func (f *formShelf) Deactivate(_ context.Context, projectID string) error {
	for key, form := range f.active {
		if form.ProjectID == projectID {
			delete(f.active, key)
		}
	}
	delete(f.embeds, projectID)
	f.deactivated = append(f.deactivated, projectID)
	return nil
}

func (f *formShelf) ActiveForm(_ context.Context, formKey string) (*ActiveForm, error) {
	if formKey == "broken" {
		return nil, errors.New("db down")
	}
	form, ok := f.active[formKey]
	if !ok {
		return nil, nil
	}
	return &form, nil
}

func (f *formShelf) ProjectsEmbedding(_ context.Context, _ string, widgetID string) ([]string, error) {
	var out []string
	for project, widgets := range f.embeds {
		for _, w := range widgets {
			if w == widgetID {
				out = append(out, project)
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

func (f *formShelf) WorkspaceForms(_ context.Context, workspaceID string) ([]PageForm, error) {
	if f.failList != nil {
		return nil, f.failList
	}
	var out []PageForm
	for projectID, keys := range f.keys {
		if f.owners[projectID] != workspaceID {
			continue
		}
		for formID, key := range keys {
			_, live := f.active[key]
			out = append(out, PageForm{ProjectID: projectID, ProjectName: f.names[projectID], FormID: formID, FormKey: key, Live: live})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ProjectName+out[i].FormID < out[j].ProjectName+out[j].FormID })
	return out, nil
}

type hostnames map[string][]string

func (h hostnames) ActiveHostnames(_ context.Context, projectID string) ([]string, error) {
	if projectID == "broken" {
		return nil, errors.New("db down")
	}
	return h[projectID], nil
}

type submissionShelf struct {
	rows     []sitesubmission.Submission
	attached map[string]string
	failSave error
	effects  *effectShelf
}

func (s *submissionShelf) ByIdempotency(_ context.Context, formKey, key string) (*sitesubmission.Submission, error) {
	for _, r := range s.rows {
		if r.FormKey == formKey && r.IdempotencyKey == key {
			return &r, nil
		}
	}
	return nil, nil
}

func (s *submissionShelf) Create(ctx context.Context, sub sitesubmission.Submission, run sitesubmission.EffectRun) (sitesubmission.Submission, bool, error) {
	if s.failSave != nil {
		return sitesubmission.Submission{}, false, s.failSave
	}
	if existing, _ := s.ByIdempotency(ctx, sub.FormKey, sub.IdempotencyKey); existing != nil {
		return *existing, false, nil
	}
	s.rows = append(s.rows, sub)
	if len(run.Pending) > 0 {
		s.effects.runs[run.SubmissionID] = &run
	}
	return sub, true, nil
}

type effectShelf struct {
	mu           sync.Mutex
	runs         map[string]*sitesubmission.EffectRun
	locked       map[string]time.Time
	progressed   int
	failProgress error
	failClaim    error
	failSpend    error
}

func newEffectShelf() *effectShelf {
	return &effectShelf{runs: map[string]*sitesubmission.EffectRun{}, locked: map[string]time.Time{}}
}

func (e *effectShelf) Claim(_ context.Context, id string, now time.Time, lease time.Duration) (*sitesubmission.EffectRun, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.failClaim != nil {
		return nil, e.failClaim
	}
	run, ok := e.runs[id]
	if !ok || run.FinishedAt != nil || run.NextAttemptAt.After(now) || e.locked[id].After(now) {
		return nil, nil
	}
	e.locked[id] = now.Add(lease)
	claimed := *run
	claimed.Pending = slices.Clone(run.Pending)
	return &claimed, nil
}

func (e *effectShelf) Due(_ context.Context, now time.Time, limit int) ([]string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []string
	for id, run := range e.runs {
		if run.FinishedAt == nil && !run.NextAttemptAt.After(now) && !e.locked[id].After(now) && len(out) < limit {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (e *effectShelf) Progress(_ context.Context, run sitesubmission.EffectRun) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.failProgress != nil {
		return e.failProgress
	}
	stored := e.runs[run.SubmissionID]
	if e.failSpend != nil && stored.Has(sitesubmission.EffectTrigger) && !run.Has(sitesubmission.EffectTrigger) {
		return e.failSpend
	}
	e.progressed++
	stored.Pending, stored.OwnerID, stored.EntryID, stored.EntryType = slices.Clone(run.Pending), run.OwnerID, run.EntryID, run.EntryType
	return nil
}

func (e *effectShelf) Settle(_ context.Context, run sitesubmission.EffectRun) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	stored := run
	stored.Pending = slices.Clone(run.Pending)
	e.runs[run.SubmissionID] = &stored
	delete(e.locked, run.SubmissionID)
	return nil
}

func (e *effectShelf) run(id string) sitesubmission.EffectRun {
	e.mu.Lock()
	defer e.mu.Unlock()
	return *e.runs[id]
}

type leadDeals struct {
	deals []*opportunity.Opportunity
	err   error
	asked []opportunity.LeadDealsQuery
}

func (l *leadDeals) DealsOfLead(_ context.Context, q opportunity.LeadDealsQuery) ([]*opportunity.Opportunity, error) {
	l.asked = append(l.asked, q)
	if l.err != nil {
		return nil, l.err
	}
	start := 0
	if q.Before != nil {
		for i, d := range l.deals {
			if d.ID == q.Before.ID {
				start = i + 1
			}
		}
	}
	end := min(start+q.Limit, len(l.deals))
	return l.deals[start:end], nil
}

func (s *submissionShelf) AttachConversation(_ context.Context, id, conversationID, conversationType string) error {
	if s.attached == nil {
		s.attached = map[string]string{}
	}
	s.attached[id] = conversationType + ":" + conversationID
	return nil
}

func (s *submissionShelf) List(_ context.Context, q SubmissionQuery) ([]sitesubmission.Submission, int64, error) {
	var out []sitesubmission.Submission
	for _, r := range s.rows {
		if r.WorkspaceID == q.WorkspaceID && r.ProjectID == q.ProjectID && (q.FormID == "" || r.FormID == q.FormID) {
			out = append(out, r)
		}
	}
	return out, int64(len(out)), nil
}

type spent struct {
	mu   sync.Mutex
	seen map[string]bool
}

func (s *spent) SetNX(key, _ string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen == nil {
		s.seen = map[string]bool{}
	}
	if s.seen[key] {
		return false, nil
	}
	s.seen[key] = true
	return true, nil
}

type limiter struct {
	deny map[string]bool
	err  error
	keys []string
}

func (l *limiter) Allow(key string) (bool, time.Duration, error) {
	l.keys = append(l.keys, key)
	if l.err != nil {
		return false, 0, l.err
	}
	return !l.deny[key], 0, nil
}

type leadDesk struct {
	writes     []formlead.Submission
	triggers   []formlead.Trigger
	writeErr   error
	triggerErr error
	created    bool
	owner      string
}

func (l *leadDesk) Write(_ context.Context, sub formlead.Submission) (formlead.Outcome, error) {
	if l.writeErr != nil {
		return formlead.Outcome{}, l.writeErr
	}
	l.writes = append(l.writes, sub)
	if sub.Identity.Phone == "" && sub.Identity.Email == "" {
		return formlead.Outcome{}, lead.ErrLeadIdentityRequired
	}
	return formlead.Outcome{LeadID: "lead-1", Created: l.created, Owner: l.owner}, nil
}

func (l *leadDesk) Trigger(_ context.Context, in formlead.Trigger) error {
	if l.triggerErr != nil {
		return l.triggerErr
	}
	l.triggers = append(l.triggers, in)
	return nil
}

type optIns struct {
	got []emailconsent_usecase.SubscribeInput
}

func (o *optIns) Subscribe(_ context.Context, _ string, in emailconsent_usecase.SubscribeInput) (emailconsent.Subscription, error) {
	o.got = append(o.got, in)
	return emailconsent.Subscription{}, nil
}

type roulette struct {
	pick string
	err  error
	asks []string
}

func (r *roulette) PickForLead(_ string, departmentID string) (string, error) {
	r.asks = append(r.asks, departmentID)
	return r.pick, r.err
}

type owners struct{ set []string }

func (o *owners) SetOwner(ctx context.Context, a lead_usecase.Actor, id, owner string) (*lead.Lead, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	o.set = append(o.set, a.UserID+"|"+id+"|"+owner)
	return &lead.Lead{ID: id}, nil
}

type deals struct {
	created []opportunity_usecase.CreateInput
}

func (d *deals) InitialStage(_, pipelineID string) (string, error) {
	return pipelineID + "-first", nil
}

func (d *deals) Create(_ string, in opportunity_usecase.CreateInput) (*opportunity.Opportunity, error) {
	d.created = append(d.created, in)
	return &opportunity.Opportunity{ID: "opp-1"}, nil
}

type officialEntries struct {
	got  []wo.OpenEntryInput
	fail int
}

func (o *officialEntries) Open(_ context.Context, in wo.OpenEntryInput) (*wo.OpenedEntry, error) {
	o.got = append(o.got, in)
	if o.fail > 0 {
		o.fail--
		return nil, errors.New("meta down")
	}
	return &wo.OpenedEntry{EntryID: "entry-official", EntryType: "whatsapp", LeadID: "lead-1"}, nil
}

type unofficialStarts struct{ got []uwuc.StartConversationInput }

func (u *unofficialStarts) Execute(_ context.Context, in uwuc.StartConversationInput) (*uwuc.StartedConversation, error) {
	u.got = append(u.got, in)
	return &uwuc.StartedConversation{ConversationID: "conv-unofficial"}, nil
}

type emailOpens struct {
	got []emailinbox.OpenConversationInput
}

func (e *emailOpens) OpenConversation(_ context.Context, in emailinbox.OpenConversationInput) (string, error) {
	e.got = append(e.got, in)
	return "conv-email", nil
}

type notices struct {
	drafts []notification.Draft
	fail   int
}

func (n *notices) Publish(_ context.Context, d notification.Draft) (notification.Record, error) {
	if n.fail > 0 {
		n.fail--
		return notification.Record{}, errors.New("notices down")
	}
	n.drafts = append(n.drafts, d)
	return notification.Record{}, nil
}

type conversions struct {
	mu   sync.Mutex
	sent []ads.WebsiteLead
}

func (c *conversions) SendWebsiteLead(_ context.Context, _ string, lead ads.WebsiteLead) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, lead)
	return nil
}

type links struct {
	created []shortlink.CreateShortLinkInput
}

func (l *links) Execute(_ context.Context, in shortlink.CreateShortLinkInput) (*shortlink.ShortLink, error) {
	l.created = append(l.created, in)
	n := strconv.Itoa(len(l.created))
	return &shortlink.ShortLink{ID: "sl-" + n, Code: "c" + n, TargetURL: in.TargetURL}, nil
}

type numbers map[string]string

func (n numbers) NumberOf(_ context.Context, _ string, ref string) (string, error) {
	if ref == "broken" {
		return "", errStoreDown
	}
	if number, ok := n[ref]; ok {
		return number, nil
	}
	return "", ErrWhatsAppNumber
}

type references struct {
	rows []PageReference
}

func (r *references) Find(_ context.Context, key PageReferenceKey) (*PageReference, error) {
	for _, row := range r.rows {
		if row.Key == key {
			return &row, nil
		}
	}
	return nil, nil
}

func (r *references) Create(_ context.Context, ref PageReference) (PageReference, bool, error) {
	for _, row := range r.rows {
		if row.Key == ref.Key {
			return row, false, nil
		}
	}
	r.rows = append(r.rows, ref)
	return ref, true, nil
}

func (r *references) Target(_ context.Context, code string) (*conversation.PageReferenceTarget, error) {
	for _, row := range r.rows {
		if row.Code == code {
			return &conversation.PageReferenceTarget{WorkspaceID: row.WorkspaceID, ProjectID: row.Key.ProjectID, Route: row.Key.Route, NumberRef: row.Key.NumberRef}, nil
		}
	}
	return nil, nil
}

type projects map[string]string

func (p projects) WorkspaceOf(_ context.Context, projectID string) (string, error) {
	if projectID == "broken" {
		return "", errStoreDown
	}
	if ws, ok := p[projectID]; ok {
		return ws, nil
	}
	return "", ErrProjectNotFound
}

var errStoreDown = errors.New("db down")

func (f *formShelf) WorkspaceForm(ctx context.Context, workspaceID, formKey string) (*PageForm, error) {
	forms, err := f.WorkspaceForms(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for _, form := range forms {
		if form.FormKey == formKey {
			return &form, nil
		}
	}
	return nil, nil
}
