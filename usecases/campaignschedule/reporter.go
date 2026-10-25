package campaignschedule

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"vozko/domain/campaign/scheduledstart"
	"vozko/domain/notification"
	"vozko/domain/workspace"
)

var ErrIncomplete = errors.New("campaign schedule reporter: a channel, a failure recorder, a publisher and a logger are required")

const actionLabel = "Abrir campanha"

var copyOf = map[scheduledstart.Failure]string{
	scheduledstart.FailureBlocked:       "No horário agendado, a verificação de disparo encontrou um impedimento e a campanha não começou. Abra a campanha para ver o que falta e inicie ou agende de novo.",
	scheduledstart.FailureUnconfirmed:   "Os riscos da campanha mudaram depois do agendamento e precisam de uma nova confirmação. Abra a campanha, revise os riscos e inicie ou agende de novo.",
	scheduledstart.FailureNeedsReview:   "Um envio preparado a partir de leads começa pela revisão na página de leads, que confere o saldo e quem já recebe outra campanha.",
	scheduledstart.FailureNothingToSend: "No horário agendado não havia contatos aguardando envio, então a campanha não começou.",
	scheduledstart.FailureStartFailed:   "O início falhou no horário agendado. Abra a campanha para iniciar ou agendar de novo.",
}

type Channel struct {
	Key      string
	Label    string
	Path     string
	Resource workspace.Resource
}

func (c Channel) complete() bool {
	return strings.TrimSpace(c.Key) != "" && strings.TrimSpace(c.Label) != "" && strings.HasPrefix(c.Path, "/") && c.Resource != ""
}

type Deps struct {
	Channel       Channel
	Failures      scheduledstart.Recorder
	Notifications notification.Publisher
	Logf          func(format string, args ...any)
}

type Reporter struct {
	d Deps
}

func New(d Deps) (*Reporter, error) {
	if !d.Channel.complete() || d.Failures == nil || d.Notifications == nil || d.Logf == nil {
		return nil, ErrIncomplete
	}
	return &Reporter{d: d}, nil
}

type Attempt struct {
	WorkspaceID  string
	CampaignID   string
	CampaignName string
	ScheduledBy  string
	ScheduledAt  time.Time
}

func (r *Reporter) Failed(ctx context.Context, a Attempt, cause error) error {
	failure := scheduledstart.FailureOf(cause)
	r.d.Logf("scheduled start of %s failed (%s): %v", a.CampaignID, failure, cause)
	recordErr := r.d.Failures.RecordScheduleFailure(ctx, a.CampaignID, failure)
	if _, err := r.d.Notifications.Publish(ctx, r.notice(a, failure)); err != nil {
		r.d.Logf("could not tell anyone that the scheduled start of %s failed: %v", a.CampaignID, err)
	}
	return recordErr
}

func (r *Reporter) Cleared(ctx context.Context, campaignID string, current scheduledstart.Failure) {
	if r == nil || current == "" {
		return
	}
	if err := r.d.Failures.RecordScheduleFailure(ctx, campaignID, ""); err != nil {
		r.d.Logf("started %s but could not clear its failed schedule: %v", campaignID, err)
	}
}

func (r *Reporter) notice(a Attempt, failure scheduledstart.Failure) notification.Draft {
	return notification.Draft{
		WorkspaceID: a.WorkspaceID,
		Category:    notification.CategoryCampaigns,
		Severity:    notification.SeverityCritical,
		Title:       notification.ClipTitle(r.d.Channel.Label + " agendada não começou: " + a.CampaignName),
		Body:        bodyOf(failure),
		ActionURL:   r.d.Channel.Path + a.CampaignID,
		ActionLabel: actionLabel,
		Audience:    r.audience(a.ScheduledBy),
		DedupKey:    "campaign_schedule_failed:" + r.d.Channel.Key + ":" + a.CampaignID + ":" + strconv.FormatInt(a.ScheduledAt.Unix(), 10),
		Source:      notification.SourceCampaignSchedule,
	}
}

func (r *Reporter) audience(scheduledBy string) notification.Audience {
	if by := strings.TrimSpace(scheduledBy); by != "" {
		return notification.Users(by)
	}
	return notification.Holding(r.d.Channel.Resource, workspace.ActionStart)
}

func bodyOf(failure scheduledstart.Failure) string {
	if body, ok := copyOf[failure]; ok {
		return body
	}
	return copyOf[scheduledstart.FailureStartFailed]
}
