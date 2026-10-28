package siteforms

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"vozko/domain/actor"
	ads "vozko/domain/advertising"
	"vozko/domain/conversation"
	"vozko/domain/emailaddress"
	"vozko/domain/emailconsent"
	"vozko/domain/lead"
	"vozko/domain/notification"
	"vozko/domain/opportunity"
	"vozko/domain/shared"
	"vozko/domain/site"
	"vozko/domain/sitesubmission"
	wo "vozko/domain/whatsapp_outreach"
	"vozko/domain/workspace"
	emailconsent_usecase "vozko/usecases/emailconsent"
	"vozko/usecases/emailinbox"
	"vozko/usecases/formlead"
	opportunity_usecase "vozko/usecases/opportunity"
	uwuc "vozko/usecases/unofficial_whatsapp"
)

const (
	dealSource         = "site_form"
	conversionsTimeout = 15 * time.Second
	effectsTimeout     = 2 * time.Minute
	effectsLease       = 3 * time.Minute
	effectsBatch       = 50
	dealsPageSize      = 100
	logPrefix          = "[site-forms]"
	submissionsPath    = "/dashboard/pages/"
)

var (
	errFollowUpUnknown = errors.New("siteforms: the follow up channel is not known")
	errNobodyToOwn     = errors.New("siteforms: the form names nobody to own the lead or the deal")
	errEffectDropped   = errors.New("siteforms: the effect cannot succeed and is dropped")
)

func dropped(err error) error {
	return fmt.Errorf("%w: %w", errEffectDropped, err)
}

func (s *Service) startEffects(ctx context.Context, submissionID string) {
	detached := context.WithoutCancel(ctx)
	s.d.Async(func() {
		if err := s.RunEffects(detached, submissionID); err != nil {
			log.Printf("%s effects of submission %s wait for the next sweep: %v", logPrefix, submissionID, err)
		}
	})
}

func (s *Service) RunPendingEffects(ctx context.Context) error {
	ids, err := s.d.Effects.Due(ctx, s.now(), effectsBatch)
	if err != nil {
		return err
	}
	var failures []error
	for _, id := range ids {
		if err := s.RunEffects(ctx, id); err != nil {
			failures = append(failures, fmt.Errorf("submission %s: %w", id, err))
		}
	}
	return errors.Join(failures...)
}

func (s *Service) RunEffects(ctx context.Context, submissionID string) error {
	ctx, cancel := context.WithTimeout(ctx, effectsTimeout)
	defer cancel()
	run, err := s.d.Effects.Claim(ctx, submissionID, s.now(), effectsLease)
	if err != nil || run == nil {
		return err
	}
	failure := s.apply(ctx, run)
	run.Settle(s.now(), failure)
	if run.Abandoned() {
		log.Printf("%s effects %v of submission %s given up after %d attempts: %s", logPrefix, run.Pending, run.SubmissionID, run.Attempts, run.LastError)
	}
	return s.d.Effects.Settle(context.WithoutCancel(ctx), *run)
}

func (s *Service) apply(ctx context.Context, run *sitesubmission.EffectRun) error {
	var failures []error
	for _, effect := range sitesubmission.EffectOrder() {
		if !run.Has(effect) || run.Blocked(effect) {
			continue
		}
		err := s.runEffect(ctx, run, effect)
		if errors.Is(err, errEffectDropped) {
			log.Printf("%s %s of submission %s dropped: %v", logPrefix, effect, run.SubmissionID, err)
			err = nil
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", effect, err))
			continue
		}
		run.Done(effect)
		if err := s.d.Effects.Progress(ctx, *run); err != nil {
			return errors.Join(append(failures, err)...)
		}
	}
	return errors.Join(failures...)
}

func (s *Service) runEffect(ctx context.Context, run *sitesubmission.EffectRun, effect sitesubmission.Effect) error {
	switch effect {
	case sitesubmission.EffectEmailOptIn:
		return s.subscribe(ctx, run)
	case sitesubmission.EffectOwner:
		return s.placeOwner(ctx, run)
	case sitesubmission.EffectDeal:
		return s.openDeal(ctx, run)
	case sitesubmission.EffectConversation:
		return s.openConversation(ctx, run)
	case sitesubmission.EffectTrigger:
		return s.fireTrigger(ctx, run)
	case sitesubmission.EffectNotice:
		return s.notify(ctx, run)
	case sitesubmission.EffectConversion:
		return s.signal(ctx, run)
	}
	return dropped(fmt.Errorf("unknown effect %q", effect))
}

func (s *Service) subscribe(ctx context.Context, run *sitesubmission.EffectRun) error {
	sub, values := run.Submission, run.Plan.Values
	at := sub.CreatedAt
	_, err := s.d.EmailOptIns.Subscribe(ctx, run.WorkspaceID, emailconsent_usecase.SubscribeInput{
		Address: values.Email, LeadID: sub.LeadID, Source: emailconsent.SourceSiteForm, IP: sub.IPHash, ConsentedAt: &at,
	})
	return err
}

func (s *Service) pickOwner(workspaceID string, owner *site.Owner) (string, error) {
	if owner == nil {
		return "", dropped(errNobodyToOwn)
	}
	switch owner.Kind {
	case site.OwnerUser:
		if owner.ID != "" {
			return owner.ID, nil
		}
	case site.OwnerRoulette:
		picked, err := s.d.Roulette.PickForLead(workspaceID, owner.ID)
		if err != nil {
			return "", err
		}
		if picked != "" {
			return picked, nil
		}
		return "", errNobodyToOwn
	}
	return "", dropped(errNobodyToOwn)
}

func (s *Service) placeOwner(ctx context.Context, run *sitesubmission.EffectRun) error {
	if run.OwnerID == "" {
		picked, err := s.pickOwner(run.WorkspaceID, run.Plan.Settings.Owner)
		if err != nil {
			return err
		}
		run.OwnerID = picked
		if err := s.d.Effects.Progress(ctx, *run); err != nil {
			return err
		}
	}
	system := conversation.Viewer{UserID: actor.SystemID, WorkspaceID: run.WorkspaceID, IsAdmin: true}
	_, err := s.d.LeadOwners.SetOwner(ctx, system, run.Submission.LeadID, run.OwnerID)
	if lead.IsInputRefusal(err) {
		return dropped(err)
	}
	return err
}

func (s *Service) dealOwner(run *sitesubmission.EffectRun) string {
	switch {
	case run.OwnerID != "":
		return run.OwnerID
	case run.Plan.LeadOwner != "":
		return run.Plan.LeadOwner
	case run.Plan.Settings.WorkflowID != "":
		return actor.WorkflowPrefix + run.Plan.Settings.WorkflowID
	}
	return ""
}

func (s *Service) hasOpenDeal(ctx context.Context, workspaceID, leadID, funnelID string) (bool, error) {
	q := opportunity.LeadDealsQuery{WorkspaceID: workspaceID, LeadID: leadID, Limit: dealsPageSize}
	for {
		deals, err := s.d.LeadDeals.DealsOfLead(ctx, q)
		if err != nil {
			return false, err
		}
		for _, deal := range opportunity.EntryDeals(deals).Open() {
			if deal.PipelineID == funnelID {
				return true, nil
			}
		}
		if len(deals) < dealsPageSize {
			return false, nil
		}
		last := deals[len(deals)-1]
		q.Before = &shared.Keyset{At: last.CreatedAt, ID: last.ID}
	}
}

func (s *Service) openDeal(ctx context.Context, run *sitesubmission.EffectRun) error {
	settings, sub := run.Plan.Settings, run.Submission
	owner := s.dealOwner(run)
	if owner == "" {
		return dropped(errNobodyToOwn)
	}
	open, err := s.hasOpenDeal(ctx, run.WorkspaceID, sub.LeadID, settings.FunnelID)
	if err != nil {
		return err
	}
	if open {
		return nil
	}
	stageID := settings.StageID
	if stageID == "" {
		if stageID, err = s.d.Deals.InitialStage(run.WorkspaceID, settings.FunnelID); err != nil {
			return err
		}
	}
	title := strings.TrimSpace(run.Plan.Values.Name)
	if title == "" {
		title = "Formulário " + sub.FormID
	}
	_, err = s.d.Deals.Create(run.WorkspaceID, opportunity_usecase.CreateInput{
		LeadID: sub.LeadID, PipelineID: settings.FunnelID, StageID: stageID, OwnerID: owner, Title: title, Source: dealSource, Actor: actor.SystemID,
	})
	if _, refused := opportunity_usecase.RefusalOf(err); refused {
		return dropped(err)
	}
	return err
}

func (s *Service) openConversation(ctx context.Context, run *sitesubmission.EffectRun) error {
	if run.EntryID == "" {
		channel := run.Plan.Settings.FollowUp
		if channel == nil {
			return dropped(errFollowUpUnknown)
		}
		entryID, entryType, err := s.openEntry(ctx, run.WorkspaceID, run.Submission.LeadID, *channel, run.Plan.Values)
		if err != nil {
			return err
		}
		run.EntryID, run.EntryType = entryID, entryType
		if err := s.d.Effects.Progress(ctx, *run); err != nil {
			return err
		}
	}
	return s.d.Submissions.AttachConversation(ctx, run.SubmissionID, run.EntryID, run.EntryType)
}

func (s *Service) openEntry(ctx context.Context, workspaceID, leadID string, channel site.FollowUp, values sitesubmission.EffectValues) (string, string, error) {
	switch channel.Kind {
	case site.FollowUpWhatsApp:
		opened, err := s.d.Official.Open(ctx, wo.OpenEntryInput{WorkspaceID: workspaceID, BusinessPhoneID: channel.ID, PhoneNumber: values.Phone, Name: values.Name})
		if err != nil {
			return "", "", err
		}
		return opened.EntryID, opened.EntryType, nil
	case site.FollowUpUnofficialWhatsApp:
		started, err := s.d.Unofficial.Execute(ctx, uwuc.StartConversationInput{WorkspaceID: workspaceID, UserID: actor.SystemID, InstanceID: channel.ID, PhoneNumber: values.Phone, Name: values.Name})
		if err != nil {
			return "", "", err
		}
		return started.ConversationID, string(shared.EntryTypeUnofficialWhatsApp), nil
	case site.FollowUpEmail:
		id, err := s.d.Email.OpenConversation(ctx, emailinbox.OpenConversationInput{WorkspaceID: workspaceID, SenderID: channel.ID, Address: emailaddress.Address(values.Email), LeadID: leadID, LeadName: values.Name})
		if err != nil {
			return "", "", err
		}
		return id, string(shared.EntryTypeEmail), nil
	}
	return "", "", dropped(errFollowUpUnknown)
}

func (s *Service) fireTrigger(ctx context.Context, run *sitesubmission.EffectRun) error {
	run.Done(sitesubmission.EffectTrigger)
	if err := s.d.Effects.Progress(ctx, *run); err != nil {
		run.Reopen(sitesubmission.EffectTrigger)
		return err
	}
	sub := run.Submission
	err := s.d.Leads.Trigger(ctx, formlead.Trigger{
		WorkspaceID: run.WorkspaceID, EntryID: run.EntryID, EntryType: run.EntryType,
		Form: formlead.Form{Kind: formlead.KindSiteForm, ID: sub.FormKey}, SubmissionID: sub.ID, Data: triggerData(sub),
	})
	if err == nil {
		return nil
	}
	run.Reopen(sitesubmission.EffectTrigger)
	if progress := s.d.Effects.Progress(ctx, *run); progress != nil {
		return errors.Join(err, progress)
	}
	return err
}

func triggerData(sub sitesubmission.Submission) map[string]any {
	fields := make(map[string]any, len(sub.Fields))
	for k, v := range sub.Fields {
		fields[k] = v
	}
	return map[string]any{
		"source": string(formlead.KindSiteForm), "project_id": sub.ProjectID, "form_id": sub.FormID, "form_key": sub.FormKey,
		"route": sub.Route, "variant": sub.Variant, "fields": fields,
		"utm": map[string]string{"source": sub.UTM.Source, "medium": sub.UTM.Medium, "campaign": sub.UTM.Campaign, "term": sub.UTM.Term, "content": sub.UTM.Content},
	}
}

func (s *Service) notify(ctx context.Context, run *sitesubmission.EffectRun) error {
	sub := run.Submission
	who := strings.TrimSpace(run.Plan.Values.Name)
	if who == "" {
		who = "Um visitante"
	}
	_, err := s.d.Notices.Publish(ctx, notification.Draft{
		WorkspaceID: run.WorkspaceID, Category: notification.CategoryLeads, Severity: notification.SeverityInfo,
		Title: notification.ClipTitle("Novo envio no formulário " + sub.FormID), Body: who + " enviou o formulário " + sub.FormID + " da página.",
		ActionURL: submissionsPath + sub.ProjectID, ActionLabel: "Ver envios",
		Audience: notification.Holding(workspace.ResourceSiteSubmissions, workspace.ActionRead), DedupKey: "site_submission:" + sub.ID, Source: notification.SourceSiteForm,
	})
	return err
}

func (s *Service) signal(ctx context.Context, run *sitesubmission.EffectRun) error {
	if !run.Plan.Values.Consent {
		return dropped(errors.New("the visitor did not consent"))
	}
	sub, values := run.Submission, run.Plan.Values
	ctx, cancel := context.WithTimeout(ctx, conversionsTimeout)
	defer cancel()
	return s.d.Conversions.SendWebsiteLead(ctx, run.WorkspaceID, ads.WebsiteLead{
		SubmissionID: sub.ID, At: sub.CreatedAt, Phone: values.Phone, Email: values.Email, Fbp: sub.Fbp, Fbc: sub.Fbc,
		UserAgent: sub.UserAgent, SourceURL: run.Plan.SourceURL,
	})
}
