package sitetools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"vozko/domain/copilot"
	"vozko/domain/mediagen"
	"vozko/domain/site"
	"vozko/domain/tools"
	"vozko/domain/workspace"
	"vozko/usecases/copilot/copilottools"
	"vozko/usecases/copilot/copilottools/mediatools"
)

const (
	imageNeedsAlt   = "alt é obrigatório quando a imagem vai para um node"
	imageNeedsRev   = "rev é obrigatório com node: o rev do arquivo em que o node foi lido"
	imageNoPlace    = "a imagem não cabe nesse node, então nada foi gerado nem cobrado: %s"
	imagePlaceLater = " A imagem foi gerada e cobrada (media_id %s, media_url %s): coloque com site_edit usando replace_image e esse media_id; nunca gere de novo."
	imageUnplaced   = "imagem gerada e guardada na biblioteca; coloque com site_edit (replace_image com media_id, ou um img com o media_url literal)"
	probeMediaID    = "probe"
	fieldNode       = "node"
	placeLibrary    = "só a biblioteca"
)

type queuedGeneration interface {
	copilot.Tool
	copilot.ChoiceAsker
	copilot.Validator
	copilot.Describer
	GenerationRequest(ctx context.Context, cc copilot.Context, args map[string]interface{}) (mediagen.Request, error)
}

type siteImagePlacementArgs struct {
	Node string `json:"node,omitempty" desc:"opcional: um img do site (arquivo#offset); a imagem nova entra no lugar"`
	Rev  int    `json:"rev,omitempty" desc:"com node: rev do arquivo em que o node foi lido"`
	Alt  string `json:"alt,omitempty" desc:"com node: o que a imagem mostra, para quem não vê"`
}

type siteGenerateImageTool struct {
	siteScope
	deps  Deps
	inner queuedGeneration
}

func NewSiteGenerateImageTool(deps Deps) copilot.Tool {
	return &siteGenerateImageTool{deps: deps, inner: mediatools.NewGenerateImageTool(deps.Media).(queuedGeneration)}
}

func (t *siteGenerateImageTool) Meta() copilot.Meta { return t.inner.Meta() }

func (t *siteGenerateImageTool) AlsoRequires() []workspace.PermissionEntry {
	return []workspace.PermissionEntry{{Resource: workspace.ResourceSites, Action: workspace.ActionUpdate}}
}

func (t *siteGenerateImageTool) NeedsApproval(mode copilot.Mode) bool {
	return mode != copilot.ModeFull
}

func (t *siteGenerateImageTool) Definition() tools.Definition {
	inner := t.inner.Definition()
	def := copilottools.Definition("site_generate_image",
		"Gera uma imagem com IA pelo mesmo módulo de generate_image e, com node, já a coloca num img do site aberto pelo mesmo caminho de site_edit; sem node, só guarda na biblioteca. "+
			"É cobrada do saldo pelo custo do provedor; conforme o modo do usuário, roda direto ou pede aprovação (o usuário escolhe o modelo no cartão). Devolve media_id, media_url e, colocada, changed e open_gaps.",
		siteImagePlacementArgs{})
	for name, param := range inner.Parameters {
		def.Parameters[name] = param
	}
	def.Required = append(append([]string(nil), inner.Required...), def.Required...)
	return def
}

func (t *siteGenerateImageTool) Choices(ctx context.Context, cc copilot.Context, args map[string]interface{}) ([]copilot.ChoiceField, error) {
	choices, err := t.inner.Choices(ctx, cc, args)
	if err != nil {
		return nil, err
	}
	return mediatools.WithPreferredModels(ctx, t.deps.Media, mediagen.KindImage, choices), nil
}

func (t *siteGenerateImageTool) Preview(ctx context.Context, cc copilot.Context, args map[string]interface{}) *copilot.Preview {
	if previewer, ok := t.inner.(copilot.Previewer); ok {
		return previewer.Preview(ctx, cc, args)
	}
	return nil
}

func (t *siteGenerateImageTool) Describe(ctx context.Context, cc copilot.Context, args map[string]interface{}) []copilot.Field {
	fields := t.inner.Describe(ctx, cc, args)
	var p siteImagePlacementArgs
	copilottools.BindArgs(args, &p)
	place := placeLibrary
	if node := strings.TrimSpace(p.Node); node != "" {
		place = node
	}
	return append(fields, copilot.Field{Key: fieldNode, Value: place})
}

func (t *siteGenerateImageTool) placement(args map[string]interface{}) (siteImagePlacementArgs, error) {
	var p siteImagePlacementArgs
	if err := copilottools.DecodeArgs(args, &p); err != nil {
		return p, fmt.Errorf("%w: %v", copilottools.ErrInvalidArgs, err)
	}
	p.Node, p.Alt = strings.TrimSpace(p.Node), strings.TrimSpace(p.Alt)
	if p.Node == "" {
		return p, nil
	}
	if p.Alt == "" {
		return p, fmt.Errorf("%w: %s", copilottools.ErrInvalidArgs, imageNeedsAlt)
	}
	if p.Rev <= 0 {
		return p, fmt.Errorf("%w: %s", copilottools.ErrInvalidArgs, imageNeedsRev)
	}
	return p, nil
}

func (p siteImagePlacementArgs) batch(mediaID string) (json.RawMessage, error) {
	return json.Marshal([]map[string]interface{}{{"op": string(site.OpReplaceImage), "node": p.Node, "rev": p.Rev, "media_id": mediaID, "alt": p.Alt}})
}

func (p siteImagePlacementArgs) fits(project site.Project) error {
	raw, err := p.batch(probeMediaID)
	if err != nil {
		return err
	}
	_, err = site.PrepareBatch(project, raw)
	var refused *site.BatchError
	if errors.As(err, &refused) && len(refused.Failures) > 0 {
		f := refused.Failures[0]
		words := codeWords[f.Code]
		if words == "" {
			words = opDefault
		}
		return fmt.Errorf(imageNoPlace, f.Code+": "+words)
	}
	return err
}

func (t *siteGenerateImageTool) Validate(ctx context.Context, cc copilot.Context, args map[string]interface{}) error {
	p, err := t.placement(args)
	if err != nil {
		return err
	}
	view, err := t.deps.open(ctx, cc, forEditing)
	if err != nil {
		return fmt.Errorf("%w: %v", copilottools.ErrInvalidArgs, err)
	}
	if p.Node != "" {
		if err := p.fits(view.Source()); err != nil {
			return fmt.Errorf("%w: %v", copilottools.ErrInvalidArgs, err)
		}
	}
	return t.inner.Validate(ctx, cc, args)
}

func (t *siteGenerateImageTool) Execute(ctx context.Context, cc copilot.Context, args map[string]interface{}) copilot.Result {
	p, err := t.placement(args)
	if err != nil {
		return failure(err.Error())
	}
	view, err := t.deps.open(ctx, cc, forEditing)
	if err != nil {
		return failure(err.Error())
	}
	if p.Node != "" {
		if err := p.fits(view.Source()); err != nil {
			return failure(err.Error())
		}
	}
	req, err := t.inner.GenerationRequest(ctx, cc, args)
	if err != nil {
		return failure(err.Error())
	}
	job, result, message := mediatools.AwaitGeneration(ctx, t.deps.Media, cc, "site_generate_image", req)
	if message != "" {
		return failure(message)
	}
	media := &copilot.Media{Kind: copilot.MediaImage, URL: result.MediaURL, MediaID: result.MediaID, Alt: job.Prompt}
	body := map[string]interface{}{"media_id": result.MediaID, "media_url": result.MediaURL, "model": result.Model}
	if p.Node == "" {
		body["next"] = imageUnplaced
		return copilot.Result{Status: copilot.StatusOK, Data: body, Media: media}
	}
	raw, err := p.batch(result.MediaID)
	if err != nil {
		return failure(err.Error() + fmt.Sprintf(imagePlaceLater, result.MediaID, result.MediaURL))
	}
	fresh, err := t.deps.open(ctx, cc, forEditing)
	if err != nil {
		return failure(err.Error() + fmt.Sprintf(imagePlaceLater, result.MediaID, result.MediaURL))
	}
	outcome, err := t.deps.applyEdit(ctx, cc, fresh, "Imagem gerada", raw)
	if err != nil {
		return failure(err.Error() + fmt.Sprintf(imagePlaceLater, result.MediaID, result.MediaURL))
	}
	inspectCtx, cancel := bounded(ctx, inspectLimit)
	defer cancel()
	in := t.deps.inspect(inspectCtx, cc, outcome.view)
	data := outcome.data(in)
	for key, value := range body {
		data[key] = value
	}
	return copilot.Result{Status: copilot.StatusOK, Data: withOpenGaps(data, in), Media: media}
}
