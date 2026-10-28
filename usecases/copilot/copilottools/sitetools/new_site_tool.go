package sitetools

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"

	"vozko/domain/copilot"
	"vozko/domain/site"
	"vozko/domain/tools"
	"vozko/domain/workspace"
	"vozko/usecases/copilot/copilottools"
	"vozko/usecases/copilot/copilottools/screencall"
	site_usecase "vozko/usecases/site"
)

const (
	defaultStack     = site.StackReact
	defaultStarter   = "blank"
	stateOpened      = "opened"
	stateWaiting     = "waiting"
	newSiteOpened    = "página criada e aberta: o editor de páginas já está na tela do usuário e as ferramentas site_ entram na sua próxima chamada. Continue o trabalho agora, começando por site_read; não peça que ele abra nem que confirme nada"
	newSiteWaiting   = "página criada, mas o editor continua fechado: ele abre quando o usuário tocar no cartão. Diga isso uma vez e não diga que abriu. Quando ele abrir, o app avisa você e as ferramentas site_ chegam; nunca peça que ele avise que abriu"
	newSiteNameBad   = "name é obrigatório e vai até 120 caracteres"
	newSiteStarter   = "starter %q não existe para %s; use um de: %s"
	newSiteFailed    = "a página não pôde ser criada agora; tente de novo"
	newSiteTaken     = "este pedido já criou uma página para outra pessoa; nada foi criado"
	newSiteNamespace = "6c1f4e2a-8b3d-4f5e-9a7b-2c4d6e8f0a1b"
)

var openingWords = screencall.Words{
	Silent:      "o app não confirmou a abertura a tempo",
	Expired:     "o app não respondeu a tempo",
	Unconfirmed: "o app começou a abrir o editor e não confirmou",
	Absent:      "nenhuma aba conseguiu abrir o editor",
	Refused:     "o app não abriu o editor",
}

type newSiteArgs struct {
	Name    string `json:"name" req:"true" desc:"nome da página, como aparece em Páginas"`
	Stack   string `json:"stack,omitempty" enum:"react,html" desc:"react (padrão) ou html"`
	Starter string `json:"starter,omitempty" enum:"blank,captura-de-leads" desc:"ponto de partida; blank (padrão) quando você vai escrever a página do zero"`
}

type siteOpening struct {
	ProjectID string `json:"projectId"`
	Name      string `json:"name"`
}

type newSiteTool struct{ deps Deps }

func NewNewSiteTool(deps Deps) copilot.Tool { return &newSiteTool{deps: deps} }

func (t *newSiteTool) OfferedOn(view copilot.View) bool { return !view.Focused() }

func (t *newSiteTool) Meta() copilot.Meta {
	return copilot.Meta{Resource: workspace.ResourceSites, Action: workspace.ActionCreate}
}

func (t *newSiteTool) Definition() tools.Definition {
	return copilottools.Definition("new_site",
		"Cria uma página (landing page) em Páginas a partir de um ponto de partida e mostra um cartão que leva o usuário ao editor dela. "+
			"O retorno diz o estado real: opened (o editor já abriu na tela dele e as ferramentas site_ entram na sua próxima chamada) ou waiting (criada, esperando ele abrir pelo cartão; o app avisa quando ele abrir). "+
			"No editor, você lê com site_read, edita com site_edit e vê com site_look, ao lado dele. Repetir o mesmo pedido nesta conversa devolve a mesma página.",
		newSiteArgs{})
}

func originOf(thread string, name string, stack site.Stack, starter string) site_usecase.Origin {
	command := uuid.NewSHA1(uuid.MustParse(newSiteNamespace), []byte(thread+"\n"+strings.TrimSpace(name)+"\n"+string(stack)+"\n"+starter))
	return site_usecase.Origin{ThreadID: thread, CommandID: command.String()}
}

func starterKeys(stack site.Stack) []string {
	all, err := site_usecase.Starters()
	if err != nil {
		return nil
	}
	var keys []string
	for _, s := range all {
		if s.Stack == stack {
			keys = append(keys, s.Key)
		}
	}
	return keys
}

func (t *newSiteTool) Execute(ctx context.Context, cc copilot.Context, args map[string]interface{}) copilot.Result {
	var a newSiteArgs
	if err := copilottools.DecodeArgs(args, &a); err != nil {
		return failure(err.Error())
	}
	stack := site.Stack(strings.TrimSpace(a.Stack))
	if stack == "" {
		stack = defaultStack
	}
	starter := strings.TrimSpace(a.Starter)
	if starter == "" {
		starter = defaultStarter
	}
	thread, err := threadOf(cc)
	if err != nil {
		return failure(err.Error())
	}
	if t.deps.Storage == nil {
		return failure(newSiteFailed)
	}
	view, err := t.deps.Storage.Create(ctx, site_usecase.CreateInput{
		WorkspaceID: cc.WorkspaceID, UserID: cc.UserID, Name: a.Name, Stack: stack, Starter: starter, Origin: originOf(thread, a.Name, stack, starter),
	})
	if err != nil {
		return failure(createFailure(err, stack, starter))
	}
	card, err := copilot.NewNavigationCard(workspace.ScreenSiteEditor, view.Project.ID)
	if err != nil {
		return failure(newSiteFailed)
	}
	opening := t.open(ctx, cc, view.Project)
	state, next := stateWaiting, newSiteWaiting
	if opening.Opened {
		state, next = stateOpened, newSiteOpened
	}
	return copilot.Result{Status: copilot.StatusOK, Card: card, Opening: &opening, Data: map[string]interface{}{"state": state, "project_id": view.Project.ID, "stack": view.Project.Stack, "next": next}}
}

func (t *newSiteTool) open(ctx context.Context, cc copilot.Context, project site_usecase.Project) copilot.ProjectOpening {
	waiting := copilot.WaitingSite(project.ID)
	if cc.Screen == nil {
		return waiting
	}
	reply, err := screencall.Run(ctx, cc, copilot.ScreenOpenSite, siteOpening{ProjectID: project.ID, Name: project.Name}, openingWords)
	if err != nil {
		log.Printf("[copilot] new_site %s stays closed: %v", project.ID, err)
		return waiting
	}
	opening, err := copilot.SiteOpeningOf(project.ID, reply.Data)
	if err != nil {
		return waiting
	}
	return opening
}

func createFailure(err error, stack site.Stack, starter string) string {
	switch {
	case errors.Is(err, site_usecase.ErrNameInvalid):
		return newSiteNameBad
	case errors.Is(err, site_usecase.ErrStarterUnknown):
		return fmt.Sprintf(newSiteStarter, starter, stack, strings.Join(starterKeys(stack), ", "))
	case errors.Is(err, site_usecase.ErrOriginTaken):
		return newSiteTaken
	}
	log.Printf("[copilot] new_site could not create the page: %v", err)
	return newSiteFailed
}
