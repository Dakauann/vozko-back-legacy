package sitehttp

import (
	"bytes"
	"context"
	"io"
	"math"
	"net/http"

	"github.com/gorilla/mux"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/imageguard"
	workspace_domain "vozko/domain/workspace"
	site_usecase "vozko/usecases/site"
	"vozko/usecases/siteinspect"
	"vozko/usecases/sitethumbnail"
)

const (
	CodeThumbnailInvalid = "thumbnail_invalid"
	captureEnvelopeBytes = 64 << 10
	snapshotBodyBytes    = 2*siteinspect.MaxSnapshotBytes + captureEnvelopeBytes
	thumbnailBodyBytes   = 2*(1536<<10) + captureEnvelopeBytes
)

var captureRefusals = []response.ErrorRefusal{
	{Err: sitethumbnail.ErrImageInvalid, Refusal: response.NewRefusal(http.StatusUnprocessableEntity, CodeThumbnailInvalid, "A miniatura precisa ser uma imagem JPEG do site")},
}

type CaptureProjects interface {
	Get(ctx context.Context, workspaceID, id string) (*site_usecase.View, error)
}

type OutlineSnapshots interface {
	Submit(ctx context.Context, v *site_usecase.View, snapshot siteinspect.Snapshot) siteinspect.Report
}

type ThumbnailUploads interface {
	Upload(ctx context.Context, workspaceID, projectID string, image []byte) (sitethumbnail.Upload, error)
}

type CaptureHandler struct {
	projects   CaptureProjects
	outlines   OutlineSnapshots
	thumbnails ThumbnailUploads
}

func NewCaptureHandler(projects CaptureProjects, outlines OutlineSnapshots, thumbnails ThumbnailUploads) *CaptureHandler {
	return &CaptureHandler{projects: projects, outlines: outlines, thumbnails: thumbnails}
}

func RegisterCaptureRoutes(protected *mux.Router, h *CaptureHandler, ac AccessControl) {
	if h == nil {
		return
	}
	sites, update := workspace_domain.ResourceSites, workspace_domain.ActionUpdate
	protected.HandleFunc("/sites/projects/{id}/outline", ac(sites, update, h.SubmitOutline)).Methods(http.MethodPost)
	protected.HandleFunc("/sites/projects/{id}/thumbnail", ac(sites, update, h.UploadThumbnail)).Methods(http.MethodPost)
}

type OutlineSnapshotRequest struct {
	Documents map[string]string `json:"documents" example:"/:<!doctype html><html>...</html>"`
}

type ThumbnailRequest struct {
	Image string `json:"image" example:"data:image/jpeg;base64,/9j/4AAQSkZJRg..."`
}

type ThumbnailResponse struct {
	Accepted          bool `json:"accepted"`
	RetryAfterSeconds int  `json:"retryAfterSeconds" example:"600"`
}

func presentOutline(report siteinspect.Report) OutlineResponse {
	return OutlineResponse{
		Routes: nonNil(report.Routes), Forms: nonNil(report.Forms), OpenGaps: nonNil(report.Gaps), GapsChecked: report.Known, ReasonCode: report.ReasonCode, ReasonRoute: report.ReasonRoute,
		BuildErrors: buildDetails(&site_usecase.BuildFailedError{Errors: report.BuildIssues}).Errors, RuleIssues: report.RuleIssues, Checks: nonNil(report.Checks), PreviewURL: report.PreviewURL,
	}
}

func decodeCapture(w http.ResponseWriter, r *http.Request, limit int64, into any) bool {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		writeError(w, err, "Failed to read the body")
		return false
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	return httpx.DecodeStrictJSON(w, r, into)
}

// @Summary		Enviar o mapa renderizado da página
// @Description	O editor aberto por quem edita (sites:update) envia o DOM renderizado de cada rota da pré-visualização atual, lido no próprio navegador, e recebe o mesmo mapa de GET .../outline: árvore de elementos por rota, pendências e verificações, calculadas aqui com as regras do servidor. O envio precisa ter exatamente as rotas de site.json, cada documento até 1,5 MB e o conjunto até 6 MB; cada documento precisa trazer o carimbo vz-revs da versão de trabalho atual, senão o mapa volta com gapsChecked false e reasonCode snapshot_stale ou snapshot_invalid. É uma leitura informativa: a publicação nunca usa este mapa e roda a própria renderização e as próprias verificações. No máximo uma leitura por página a cada 5 s; dentro desse intervalo volta o mapa guardado desta versão ou reasonCode inspection_throttled.
// @Tags			Páginas
// @Accept			json
// @Produce		json
// @Param			id		path		string					true	"id da página"
// @Param			body	body		OutlineSnapshotRequest	true	"DOM renderizado de cada rota"
// @Success		200		{object}	OutlineResponse
// @Failure		400		{object}	response.ErrorResponse	"invalid_body"
// @Failure		404		{object}	response.ErrorResponse	"project_not_found"
// @Failure		413		{object}	response.ErrorResponse	"project_too_large"
// @Security		BearerAuth
// @Router			/sites/projects/{id}/outline [post]
func (h *CaptureHandler) SubmitOutline(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	var req OutlineSnapshotRequest
	if !decodeCapture(w, r, snapshotBodyBytes, &req) {
		return
	}
	if req.Documents == nil {
		response.WriteErrorWithCode(w, http.StatusBadRequest, httpx.CodeInvalidBody, "documents é obrigatório", nil)
		return
	}
	v, err := h.projects.Get(r.Context(), workspaceID, mux.Vars(r)["id"])
	if err != nil {
		writeError(w, err, "Failed to outline the page")
		return
	}
	response.WriteSuccess(w, http.StatusOK, presentOutline(h.outlines.Submit(r.Context(), v, siteinspect.Snapshot(req.Documents))))
}

// @Summary		Enviar a miniatura da página
// @Description	O editor aberto por quem edita (sites:update) envia uma imagem JPEG da pré-visualização (de 320x200 a 2560x2560 px, até 1,5 MB, em data URL). A imagem é decodificada e recodificada aqui antes de entrar na biblioteca de mídia como a miniatura da página, e a miniatura anterior sai da biblioteca. No máximo uma miniatura por página a cada 10 minutos: fora desse intervalo accepted é false e retryAfterSeconds diz quando enviar de novo. A miniatura da publicação continua sendo feita no servidor.
// @Tags			Páginas
// @Accept			json
// @Produce		json
// @Param			id		path		string				true	"id da página"
// @Param			body	body		ThumbnailRequest	true	"imagem JPEG em data URL"
// @Success		200		{object}	ThumbnailResponse
// @Failure		400		{object}	response.ErrorResponse	"invalid_body"
// @Failure		404		{object}	response.ErrorResponse	"project_not_found"
// @Failure		413		{object}	response.ErrorResponse	"project_too_large"
// @Failure		422		{object}	response.ErrorResponse	"thumbnail_invalid"
// @Security		BearerAuth
// @Router			/sites/projects/{id}/thumbnail [post]
func (h *CaptureHandler) UploadThumbnail(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := httpx.RequireWorkspace(w, r)
	if !ok {
		return
	}
	var req ThumbnailRequest
	if !decodeCapture(w, r, thumbnailBodyBytes, &req) {
		return
	}
	image, err := imageguard.DataURL(req.Image, sitethumbnail.ImageLimits.MaxBytes)
	if err != nil {
		writeCaptureError(w, sitethumbnail.ErrImageInvalid)
		return
	}
	v, err := h.projects.Get(r.Context(), workspaceID, mux.Vars(r)["id"])
	if err != nil {
		writeError(w, err, "Failed to store the thumbnail")
		return
	}
	res, err := h.thumbnails.Upload(r.Context(), workspaceID, v.Project.ID, image)
	if err != nil {
		writeCaptureError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, ThumbnailResponse{Accepted: res.Accepted, RetryAfterSeconds: int(math.Ceil(res.RetryAfter.Seconds()))})
}

func writeCaptureError(w http.ResponseWriter, err error) {
	if !response.WriteErrorRefusal(w, err, captureRefusals) {
		writeError(w, err, "Failed to store the thumbnail")
	}
}
