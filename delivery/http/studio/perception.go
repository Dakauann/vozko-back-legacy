package studiohttp

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/studio"
	"vozko/domain/studio/perception"
)

const maxPerceptionBytes = studio.MaxDocumentBytes

type PerceptionAnalyzer interface {
	Shapes(ctx context.Context, request *perception.ShapesRequest) ([][]perception.Shape, error)
}

func (h *Handler) WithPerception(analyzer PerceptionAnalyzer) *Handler {
	h.perception = analyzer
	return h
}

type PerceptionLayers struct {
	Depth  bool `json:"depth" example:"true"`
	Layout bool `json:"layout" example:"true"`
}

type PerceptionRequest struct {
	Approximate bool                `json:"approximate"`
	Layers      PerceptionLayers    `json:"layers"`
	Surfaces    []PerceptionSurface `json:"surfaces,omitempty"`
	Video       *PerceptionVideo    `json:"video,omitempty"`
}

type PerceptionSurface struct {
	ID     string         `json:"id,omitempty"`
	Canvas map[string]any `json:"canvas" swaggertype:"object"`
	Layers []any          `json:"layers" swaggertype:"array,object"`
	Texts  map[string]any `json:"texts" swaggertype:"object"`
	QR     map[string]int `json:"qr,omitempty"`
}

type PerceptionVideo struct {
	Canvas     map[string]any `json:"canvas" swaggertype:"object"`
	Background string         `json:"background" example:"#0f172a"`
	Layers     []any          `json:"layers" swaggertype:"array,object"`
	Frames     []any          `json:"frames" swaggertype:"array,object"`
	Times      []float64      `json:"times"`
}

type PerceptionShape struct {
	Kind   string      `json:"kind" enums:"arrow,segment,guide,area,ring"`
	From   []float64   `json:"from,omitempty"`
	To     []float64   `json:"to,omitempty"`
	Points [][]float64 `json:"points,omitempty"`
	At     []float64   `json:"at,omitempty"`
	Tone   string      `json:"tone" enums:"ok,bad"`
}

type PerceptionResponse struct {
	Shapes [][]PerceptionShape `json:"shapes"`
}

func presentShapes(scenes [][]perception.Shape) PerceptionResponse {
	out := PerceptionResponse{Shapes: make([][]PerceptionShape, len(scenes))}
	for i, shapes := range scenes {
		out.Shapes[i] = make([]PerceptionShape, len(shapes))
		for k, s := range shapes {
			shape := PerceptionShape{Kind: s.Kind, Tone: string(s.Tone)}
			if s.From != nil {
				shape.From = s.From[:]
			}
			if s.To != nil {
				shape.To = s.To[:]
			}
			if s.At != nil {
				shape.At = s.At[:]
			}
			for _, p := range s.Points {
				shape.Points = append(shape.Points, []float64{p[0], p[1]})
			}
			out.Shapes[i][k] = shape
		}
	}
	return out
}

// @Summary		Medir os problemas de design de cenas do Estúdio
// @Description	Devolve as marcas que os painéis "Sombras e camadas cobertas" (layers.depth) e "Problemas de layout" (layers.layout) desenham sobre o editor: setas da direção de cada sombra, bordas que cortam sombras, áreas cobertas, guias de alinhamento, margens apertadas, quase encostados, sobreposições e faixas vazias, em coordenadas de 0 a 1. O editor envia o que só ele mede: em imagem, cada prancheta como está na tela (canvas, camadas e o layout medido de cada texto em texts); em vídeo, os quadros posados nos instantes de video.times (camadas numa tabela, elementos com a transformação de cada instante, um número repete o elemento daquela posição no quadro anterior). Uma resposta traz uma lista de marcas por prancheta ou por instante, na ordem do pedido. Use surfaces ou video, nunca os dois; até 100 cenas e 10 MB. Fatos que não podem ser lidos, um texto sem o layout medido ou um instante que falta respondem 422 invalid_facts; corpo grande demais responde 413 facts_too_large.
// @Tags			Estúdio
// @Accept			json
// @Produce		json
// @Param			body	body		PerceptionRequest	true	"cenas medidas pelo editor"
// @Success		200		{object}	PerceptionResponse
// @Failure		413		{object}	response.ErrorResponse
// @Failure		422		{object}	response.ErrorResponse
// @Failure		503		{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/studio/perception/analyze [post]
func (h *Handler) AnalyzePerception(w http.ResponseWriter, r *http.Request) {
	if _, ok := httpx.RequireWorkspace(w, r); !ok {
		return
	}
	if h.perception == nil {
		response.WriteErrorWithCode(w, http.StatusServiceUnavailable, "unavailable", "A medição de design não está disponível", nil)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxPerceptionBytes))
	if err != nil {
		writePerceptionError(w, err)
		return
	}
	request, err := perception.DecodeShapesRequest(raw)
	if err != nil {
		writePerceptionError(w, err)
		return
	}
	shapes, err := h.perception.Shapes(r.Context(), request)
	if err != nil {
		writePerceptionError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, presentShapes(shapes))
}

func writePerceptionError(w http.ResponseWriter, err error) {
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig):
		response.WriteErrorWithCode(w, http.StatusRequestEntityTooLarge, "facts_too_large", "As cenas enviadas passam do tamanho máximo", nil)
	case errors.Is(err, perception.ErrMalformed), errors.Is(err, perception.ErrIncomplete), errors.Is(err, perception.ErrUnmeasured), errors.Is(err, perception.ErrUnknownView):
		response.WriteErrorWithCode(w, http.StatusUnprocessableEntity, "invalid_facts", "As cenas enviadas não puderam ser medidas", nil)
	default:
		log.Printf("[studio] perception analysis failed: %v", err)
		response.WriteError(w, http.StatusInternalServerError, "Failed to measure the design", nil)
	}
}
