package sitetools

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"vozko/domain/copilot"
	"vozko/domain/imageguard"
	"vozko/domain/tools"
	"vozko/usecases/copilot/copilottools"
	"vozko/usecases/siterender"
)

const (
	referenceRefused = "esse endereço não pode ser aberto (precisa ser uma página pública https); nada foi capturado"
	referenceFailed  = "a captura falhou (%s); tente de novo"
	referenceBlind   = "o modelo desta conversa não enxerga imagens: vieram só as cores, fontes e espaçamentos medidos"
	referenceUse     = "use como referência de estilo; nunca copie textos, imagens nem marcas de outra empresa"
)

type siteCaptureReferenceArgs struct {
	URL string `json:"url" req:"true" desc:"endereço https de uma página que o usuário deu"`
}

type siteCaptureReferenceTool struct {
	siteScope
	deps Deps
}

func NewSiteCaptureReferenceTool(deps Deps) copilot.Tool {
	return &siteCaptureReferenceTool{deps: deps}
}

func (t *siteCaptureReferenceTool) Meta() copilot.Meta { return readMeta() }

func (t *siteCaptureReferenceTool) Definition() tools.Definition {
	return copilottools.Definition("site_capture_reference",
		"Abre num navegador isolado uma página pública que o usuário deu e devolve uma imagem dela (computador e celular) e os tokens medidos: cores de fundo e de texto com a participação de cada uma, fontes e o ritmo de espaçamento.",
		siteCaptureReferenceArgs{})
}

func (t *siteCaptureReferenceTool) Execute(ctx context.Context, cc copilot.Context, args map[string]interface{}) copilot.Result {
	var a siteCaptureReferenceArgs
	if err := copilottools.DecodeArgs(args, &a); err != nil {
		return failure(err.Error())
	}
	if _, err := t.deps.open(ctx, cc, forReading); err != nil {
		return failure(err.Error())
	}
	if t.deps.Renderer == nil {
		return failure(fmt.Sprintf(referenceFailed, siterender.CodeUnavailable))
	}
	captureCtx, cancel := bounded(ctx, lookLimit)
	defer cancel()
	ref, err := t.deps.Renderer.CaptureReference(captureCtx, strings.TrimSpace(a.URL))
	switch {
	case errors.Is(err, siterender.ErrReferenceRefused), errors.Is(err, siterender.ErrURLRefused):
		return failure(referenceRefused)
	case err != nil:
		return failure(fmt.Sprintf(referenceFailed, siterender.ErrorCode(err)))
	}
	body := asPageData(map[string]interface{}{"url": ref.URL, "title": clip(ref.Title, maxReferenceTitle), "tokens": ref.Tokens, "note": referenceUse})
	if !cc.SeesImages || len(ref.Image) == 0 {
		body["note"] = referenceBlind + "; " + referenceUse
		return copilot.Result{Status: copilot.StatusOK, Data: body}
	}
	return copilot.Result{Status: copilot.StatusOK, Data: body, Images: []string{imageguard.JPEGDataURLPrefix + base64.StdEncoding.EncodeToString(ref.Image)}}
}
