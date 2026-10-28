package sitetools

import (
	"context"
	"errors"
	"strings"
	"time"

	"vozko/domain/aichat"
	"vozko/domain/copilot"
	"vozko/domain/site"
	"vozko/domain/sitepublish"
	"vozko/domain/workspace"
	"vozko/usecases/copilot/copilottools/mediatools"
	"vozko/usecases/copilot/copilottools/screencall"
	site_usecase "vozko/usecases/site"
	"vozko/usecases/siteinspect"
	sitepublish_usecase "vozko/usecases/sitepublish"
	"vozko/usecases/siterender"
)

type Storage interface {
	Get(ctx context.Context, workspaceID, id string) (*site_usecase.View, error)
	Create(ctx context.Context, in site_usecase.CreateInput) (*site_usecase.View, error)
	Edit(ctx context.Context, in site_usecase.EditInput) (*site_usecase.EditResult, error)
	Preview(ctx context.Context, workspaceID, projectID string) (site_usecase.PreviewResult, error)
	GetSettings(ctx context.Context, workspaceID, projectID string) (*site_usecase.StoredSettings, error)
	PutSettings(ctx context.Context, workspaceID, projectID string, expected int64, raw []byte) (*site_usecase.StoredSettings, error)
	RevisionSource(ctx context.Context, workspaceID, projectID, revisionID string) (site.Project, error)
}

type Renderer interface {
	CaptureReference(ctx context.Context, url string) (siterender.Reference, error)
}

type Publisher interface {
	PublishPinned(ctx context.Context, a sitepublish_usecase.Actor, projectID string, version int64) (sitepublish_usecase.PublishResult, error)
	ListHostnames(ctx context.Context, a sitepublish_usecase.Actor, projectID string) ([]sitepublish.HostnameRecord, error)
	ListDeployments(ctx context.Context, a sitepublish_usecase.Actor, projectID string, limit int) ([]sitepublish.DeploymentRecord, error)
	SettingsChanges(ctx context.Context, a sitepublish_usecase.Actor, projectID string) (sitepublish.SettingsChanges, error)
}

type PublishPins interface {
	SetNX(key string, value string, ttl time.Duration) (bool, error)
	GetString(key string) (string, error)
}

type References interface {
	Catalog(ctx context.Context, workspaceID string) (site_usecase.ReferenceCatalog, error)
}

type Deps struct {
	Storage     Storage
	Renderer    Renderer
	Publisher   Publisher
	Media       mediatools.MediaGeneration
	References  References
	Pins        PublishPins
	Inspections siteinspect.InspectionCache
}

const (
	siteNotHere       = "esta ferramenta só funciona com uma página aberta no editor de páginas"
	siteNoEditor      = "o editor de páginas não está ligado a esta conversa; peça ao usuário para abrir a Elo de dentro do editor da página"
	siteGone          = "a página aberta não foi encontrada neste workspace; peça ao usuário para reabrir a página"
	siteArchived      = "a página está arquivada e não muda; o usuário precisa restaurá-la em Páginas"
	siteSuspended     = "a página está suspensa e não muda por aqui"
	siteNoThread      = "esta conversa não foi identificada, então nada foi feito; tente de novo"
	siteUnavailable   = "o serviço de páginas não está disponível agora; tente de novo em instantes"
	siteEditorAwaited = "A aba da página deve estar em segundo plano ou adormecida. Não peça ao usuário para avisar nem para deixar a aba aberta: quando o editor voltar a responder, o app avisa você nesta resposta ou retoma a conversa sozinho. Não repita o pedido agora; siga com o que não depende do editor ou encerre dizendo o que está pronto e o que falta."
	siteUnreadable    = "o editor devolveu uma resposta que não pôde ser lida"
)

var editorWords = screencall.Words{
	Silent:      "o editor não respondeu a tempo. " + siteEditorAwaited,
	Expired:     "o editor não respondeu a tempo e não mostrou este lote; ele já está guardado na página e aparece quando a aba recarregar. " + siteEditorAwaited,
	Unconfirmed: "o editor começou a mostrar este lote e não confirmou; o lote já está guardado na página: confira com site_read antes de repetir qualquer coisa. " + siteEditorAwaited,
	Absent:      "nenhuma aba com esta página aberta respondeu; o que foi guardado na página aparece quando a aba recarregar",
	Refused:     "o editor recusou o pedido",
}

type siteScope struct{}

func (siteScope) OfferedOn(view copilot.View) bool { return view.OnSite() }

func readMeta() copilot.Meta {
	return copilot.Meta{Resource: workspace.ResourceSites, Action: workspace.ActionRead}
}

func editMeta() copilot.Meta {
	return copilot.Meta{Resource: workspace.ResourceSites, Action: workspace.ActionUpdate}
}

func failure(message string) copilot.Result {
	return copilot.Result{Status: copilot.StatusError, Message: message}
}

func threadOf(cc copilot.Context) (string, error) {
	thread, ok := strings.CutPrefix(cc.ChargeReference, aichat.ChargeReference(""))
	if !ok || strings.TrimSpace(thread) == "" {
		return "", errors.New(siteNoThread)
	}
	return thread, nil
}

type purpose int

const (
	forReading purpose = iota
	forEditing
)

func (d Deps) open(ctx context.Context, cc copilot.Context, use purpose) (*site_usecase.View, error) {
	if !cc.View.OnSite() {
		return nil, errors.New(siteNotHere)
	}
	if cc.Screen == nil {
		return nil, errors.New(siteNoEditor)
	}
	if d.Storage == nil {
		return nil, errors.New(siteUnavailable)
	}
	view, err := d.Storage.Get(ctx, cc.WorkspaceID, cc.View.ProjectID)
	if err != nil || view == nil || view.Project.WorkspaceID != cc.WorkspaceID {
		return nil, errors.New(siteGone)
	}
	if use == forEditing {
		switch view.Project.Status {
		case site_usecase.StatusArchived:
			return nil, errors.New(siteArchived)
		case site_usecase.StatusSuspended:
			return nil, errors.New(siteSuspended)
		}
	}
	return view, nil
}

func bounded(ctx context.Context, limit time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, limit)
}

func SiteTools(deps Deps) []copilot.Tool {
	return []copilot.Tool{
		NewSiteReadTool(deps),
		NewSiteGrepTool(deps),
		NewSiteEditTool(deps),
		NewSiteLookTool(deps),
		NewSiteGenerateImageTool(deps),
		NewSiteCaptureReferenceTool(deps),
		NewSiteSettingsTool(deps),
		NewSitePublishTool(deps),
		NewNewSiteTool(deps),
	}
}
