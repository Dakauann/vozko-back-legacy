package sitetools

import (
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"vozko/domain/copilot"
	"vozko/domain/site"
	"vozko/domain/sitepublish"
	"vozko/domain/tools"
	"vozko/domain/workspace"
	"vozko/usecases/copilot/copilottools"
	site_usecase "vozko/usecases/site"
	sitepublish_usecase "vozko/usecases/sitepublish"
)

const (
	publishLimit       = 4 * time.Minute
	publishFirst       = "primeira publicação"
	publishNoChanges   = "nada mudou desde a última publicação"
	publishUnknown     = "não foi possível comparar com a última publicação"
	publishPlatform    = "o subdomínio do site na plataforma"
	publishNoGaps      = "nenhuma"
	publishGapsUnread  = "não lidas: "
	publishHeld        = "o site foi construído, mas ficou retido para revisão antes de entrar no ar; avise o usuário que a equipe revisa e que ele recebe um aviso"
	publishLive        = "o site está no ar"
	publishGapsOpen    = "nada foi publicado: o site tem pendências que impedem publicar: %s. Corrija o que depende de você e peça ao usuário o que só ele tem"
	publishBlocked     = "nada foi publicado: o site quebra uma regra de publicação: %s"
	publishBuildFailed = "nada foi publicado: o site não compila: %s"
	publishFailed      = "nada foi publicado (%s); tente de novo em instantes"
	publishStale       = "nada foi publicado: o site mudou depois que o cartão de aprovação foi mostrado; leia o site de novo e proponha publicar outra vez"
	publishPinTTL      = 24 * time.Hour
	publishPinPrefix   = "site:publish:pin:"
	fieldAddresses     = "addresses"
	fieldChanges       = "changes"
	fieldGaps          = "open_gaps"
	maxListedChanges   = 6
)

var publishWords = []struct {
	err   error
	words string
}{
	{sitepublish_usecase.ErrAccessDenied, "o usuário não tem permissão para publicar sites"},
	{sitepublish_usecase.ErrSubscriptionInactive, "a assinatura do workspace não está ativa"},
	{sitepublish_usecase.ErrUnderReview, "o site está em revisão"},
	{sitepublish_usecase.ErrSiteSuspended, "o site está suspenso"},
	{sitepublish_usecase.ErrPrerenderFailed, "o site não pôde ser renderizado"},
	{sitepublish_usecase.ErrFormKeyMissing, "um formulário ficou sem chave pública"},
	{sitepublish_usecase.ErrFormsUnavailable, "os formulários não puderam ser ativados"},
	{sitepublish_usecase.ErrMediaUnavailable, "uma imagem da biblioteca não pôde ser copiada"},
	{sitepublish_usecase.ErrHostingUnavailable, "a hospedagem está indisponível"},
	{sitepublish_usecase.ErrPublishInProgress, "o site já está sendo publicado"},
}

type sitePublishArgs struct {
	Summary string `json:"summary,omitempty" desc:"uma frase com o que muda para o visitante, mostrada no cartão de aprovação"`
}

type sitePublishTool struct {
	siteScope
	deps Deps
}

func NewSitePublishTool(deps Deps) copilot.Tool {
	return &sitePublishTool{deps: deps}
}

func (t *sitePublishTool) Meta() copilot.Meta {
	return copilot.Meta{Mutating: true, Resource: workspace.ResourceSites, Action: workspace.ActionPublish}
}

func (t *sitePublishTool) NeedsApproval(copilot.Mode) bool { return true }

func (t *sitePublishTool) Definition() tools.Definition {
	return copilottools.Definition("site_publish",
		"Propõe publicar o site aberto: o usuário vê um cartão com os endereços, o que mudou desde a última publicação (arquivos e configurações) e as pendências, e aprova. Sempre pede aprovação, em qualquer modo. "+
			"Publique só com open_gaps vazio. O retorno diz se o site entrou no ar, os endereços e o id da publicação.",
		sitePublishArgs{})
}

func actorOf(cc copilot.Context) sitepublish_usecase.Actor {
	return sitepublish_usecase.Actor{UserID: cc.UserID, WorkspaceID: cc.WorkspaceID}
}

func (t *sitePublishTool) Validate(ctx context.Context, cc copilot.Context, _ map[string]interface{}) error {
	if t.deps.Publisher == nil {
		return fmt.Errorf("%w: %s", copilottools.ErrInvalidArgs, siteUnavailable)
	}
	if _, err := t.deps.open(ctx, cc, forEditing); err != nil {
		return fmt.Errorf("%w: %v", copilottools.ErrInvalidArgs, err)
	}
	return nil
}

func (t *sitePublishTool) addresses(ctx context.Context, cc copilot.Context, projectID string) string {
	records, err := t.deps.Publisher.ListHostnames(ctx, actorOf(cc), projectID)
	if err != nil {
		return publishPlatform
	}
	var hosts []string
	for _, r := range records {
		if r.Status == sitepublish.HostnameActive {
			hosts = append(hosts, r.Hostname)
		}
	}
	if len(hosts) == 0 {
		return publishPlatform
	}
	return strings.Join(hosts, ", ")
}

func changedSince(previous site.Project, current map[string]site.File) []string {
	var changed []string
	for _, path := range slices.Sorted(maps.Keys(current)) {
		if old, ok := previous.Files[path]; !ok || old.Content != current[path].Content {
			changed = append(changed, path)
		}
	}
	for _, path := range slices.Sorted(maps.Keys(previous.Files)) {
		if _, ok := current[path]; !ok {
			changed = append(changed, path+" apagado")
		}
	}
	return changed
}

func (t *sitePublishTool) changes(ctx context.Context, cc copilot.Context, view *site_usecase.View) string {
	deployments, err := t.deps.Publisher.ListDeployments(ctx, actorOf(cc), view.Project.ID, 10)
	if err != nil {
		return publishUnknown
	}
	for _, d := range deployments {
		if d.Status != sitepublish.DeploymentActive {
			continue
		}
		previous, err := t.deps.Storage.RevisionSource(ctx, cc.WorkspaceID, view.Project.ID, d.RevisionID)
		if err != nil {
			return publishUnknown
		}
		changed := changedSince(previous, view.Files)
		if len(changed) == 0 {
			return publishNoChanges
		}
		listed := changed[:min(len(changed), maxListedChanges)]
		line := strconv.Itoa(len(changed)) + " arquivos: " + strings.Join(listed, ", ")
		if len(changed) > len(listed) {
			line += fmt.Sprintf(" "+moreLinesTemplate, len(changed)-len(listed))
		}
		return line
	}
	return publishFirst
}

func (t *sitePublishTool) Describe(ctx context.Context, cc copilot.Context, args map[string]interface{}) []copilot.Field {
	var a sitePublishArgs
	copilottools.BindArgs(args, &a)
	view, err := t.deps.open(ctx, cc, forReading)
	if err != nil || t.deps.Publisher == nil {
		return []copilot.Field{{Key: fieldGaps, Value: siteUnavailable}}
	}
	inspectCtx, cancel := bounded(ctx, inspectLimit)
	defer cancel()
	in := t.deps.inspect(inspectCtx, cc, view)
	gaps := publishNoGaps
	switch {
	case !in.known:
		gaps = publishGapsUnread + in.reason
	case len(in.gaps) > 0:
		gaps = strings.Join(in.gapLines(), "; ")
	}
	fields := []copilot.Field{
		{Key: fieldAddresses, Value: t.addresses(ctx, cc, view.Project.ID)},
		{Key: fieldChanges, Value: t.changes(ctx, cc, view)},
		{Key: fieldSettings, Value: t.settings(ctx, cc, view.Project.ID)},
		{Key: fieldGaps, Value: gaps},
	}
	if summary := strings.TrimSpace(a.Summary); summary != "" {
		fields = append([]copilot.Field{{Key: "summary", Value: summary}}, fields...)
	}
	t.pin(cc, view.Project.Version)
	return fields
}

func (t *sitePublishTool) pin(cc copilot.Context, version int64) {
	if t.deps.Pins == nil || cc.ProposalID == "" {
		return
	}
	if _, err := t.deps.Pins.SetNX(publishPinPrefix+cc.ProposalID, strconv.FormatInt(version, 10), publishPinTTL); err != nil {
		log.Printf("[copilot] site_publish could not pin the version of proposal %s: %v", cc.ProposalID, err)
	}
}

func (t *sitePublishTool) pinned(cc copilot.Context) (int64, bool) {
	if t.deps.Pins == nil || cc.ProposalID == "" {
		return 0, false
	}
	raw, err := t.deps.Pins.GetString(publishPinPrefix + cc.ProposalID)
	if err != nil {
		return 0, false
	}
	version, err := strconv.ParseInt(raw, 10, 64)
	return version, err == nil && version > 0
}

func publishFailure(err error) string {
	var issues *sitepublish_usecase.IssuesFailure
	if errors.As(err, &issues) {
		lines := make([]string, 0, len(issues.Issues))
		for _, issue := range issues.Issues {
			lines = append(lines, issueLine(issue))
		}
		joined := strings.Join(capLines(lines), "; ")
		if errors.Is(issues.Kind, sitepublish_usecase.ErrGapsOpen) {
			return fmt.Sprintf(publishGapsOpen, joined)
		}
		return fmt.Sprintf(publishBlocked, joined)
	}
	if errors.Is(err, sitepublish_usecase.ErrPublishStale) {
		return publishStale
	}
	var build *sitepublish_usecase.BuildFailure
	if errors.As(err, &build) {
		lines := make([]string, 0, len(build.Errors))
		for _, b := range build.Errors {
			lines = append(lines, strings.TrimSpace(b.Path+" "+clip(b.Message, 160)))
		}
		return fmt.Sprintf(publishBuildFailed, strings.Join(capLines(lines), "; "))
	}
	for _, known := range publishWords {
		if errors.Is(err, known.err) {
			return fmt.Sprintf(publishFailed, known.words)
		}
	}
	log.Printf("[copilot] site_publish failed: %v", err)
	return fmt.Sprintf(publishFailed, "erro inesperado")
}

func (t *sitePublishTool) Execute(ctx context.Context, cc copilot.Context, _ map[string]interface{}) copilot.Result {
	if t.deps.Publisher == nil {
		return failure(siteUnavailable)
	}
	view, err := t.deps.open(ctx, cc, forEditing)
	if err != nil {
		return failure(err.Error())
	}
	version, ok := t.pinned(cc)
	if !ok {
		return failure(publishStale)
	}
	publishCtx, cancel := bounded(ctx, publishLimit)
	defer cancel()
	res, err := t.deps.Publisher.PublishPinned(publishCtx, actorOf(cc), view.Project.ID, version)
	if err != nil {
		return failure(publishFailure(err))
	}
	body := map[string]interface{}{"deployment_id": res.Deployment.ID}
	if res.Held {
		body["state"], body["next"] = "held", publishHeld
		return copilot.Result{Status: copilot.StatusOK, Data: body}
	}
	body["state"], body["next"], body["addresses"] = "live", publishLive, res.Hostnames
	return copilot.Result{Status: copilot.StatusOK, Data: body}
}
