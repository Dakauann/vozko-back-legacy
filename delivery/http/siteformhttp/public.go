package siteformhttp

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"vozko/delivery/http/response"
	"vozko/domain/publicform"
	"vozko/domain/sitesubmission"
	"vozko/domain/strictjson"
	"vozko/infra/http/middleware"
	"vozko/usecases/siteforms"
)

const (
	PublicPrefix     = "/public/sites/forms/"
	challengePath    = PublicPrefix + "{formKey}/challenge"
	submissionsPath  = PublicPrefix + "{formKey}/submissions"
	slotsPath        = PublicPrefix + "{formKey}/slots"
	bookingsPath     = PublicPrefix + "{formKey}/bookings"
	maxChallengeBody = 1 << 10
	preflightMaxAge  = "600"
)

type PublicForms interface {
	AllowedOrigin(ctx context.Context, formKey, origin string) (bool, error)
	Challenge(ctx context.Context, formKey string, v siteforms.Visitor) (publicform.Challenge, error)
	Submit(ctx context.Context, formKey string, v siteforms.Visitor, body []byte) (siteforms.SubmitResult, error)
	Slots(ctx context.Context, formKey string, v siteforms.Visitor) (siteforms.BookingSlots, error)
	Book(ctx context.Context, formKey string, v siteforms.Visitor, body []byte) (siteforms.SubmitResult, error)
}

type PublicHandler struct {
	forms PublicForms
}

func NewPublicHandler(forms PublicForms) *PublicHandler {
	return &PublicHandler{forms: forms}
}

func RegisterPublicRoutes(r *mux.Router, h *PublicHandler) {
	if h == nil {
		return
	}
	r.HandleFunc(challengePath, h.Preflight).Methods(http.MethodOptions)
	r.HandleFunc(submissionsPath, h.Preflight).Methods(http.MethodOptions)
	r.HandleFunc(bookingsPath, h.Preflight).Methods(http.MethodOptions)
	r.HandleFunc(challengePath, h.Challenge).Methods(http.MethodPost)
	r.HandleFunc(submissionsPath, h.Submit).Methods(http.MethodPost)
	r.HandleFunc(slotsPath, h.Slots).Methods(http.MethodGet)
	r.HandleFunc(bookingsPath, h.Book).Methods(http.MethodPost)
}

type SubmitResponse struct {
	ID      string                       `json:"id" example:"7d1f0a52-1b0f-4f7e-9a34-0d5f8f1e2c3b"`
	Success sitesubmission.SuccessAction `json:"success"`
}

func visitorOf(r *http.Request) siteforms.Visitor {
	return siteforms.Visitor{Origin: strings.TrimRight(r.Header.Get("Origin"), "/"), IP: middleware.GetClientIP(r), UserAgent: r.UserAgent()}
}

func (h *PublicHandler) cors(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Add("Vary", "Origin")
	origin := strings.TrimRight(r.Header.Get("Origin"), "/")
	if origin == "" {
		return false
	}
	allowed, err := h.forms.AllowedOrigin(r.Context(), mux.Vars(r)["formKey"], origin)
	if err != nil || !allowed {
		return false
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	return true
}

// @Summary		Pré-verificação CORS de um formulário de página
// @Description	Responde 204 com Access-Control-Allow-Origin igual à origem quando ela é exatamente um endereço publicado da página deste formulário (https://<endereço ativo>); qualquer outra origem recebe 403 sem cabeçalhos CORS. Sem credenciais. Vale também para /challenge.
// @Tags			Páginas (público)
// @Param			formKey	path	string	true	"chave pública do formulário"
// @Success		204
// @Failure		403
// @Router			/public/sites/forms/{formKey}/submissions [options]
func (h *PublicHandler) Preflight(w http.ResponseWriter, r *http.Request) {
	if !h.cors(w, r) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Max-Age", preflightMaxAge)
	w.WriteHeader(http.StatusNoContent)
}

// @Summary		Desafio de um formulário de página
// @Description	Emite a prova de trabalho de uso único que o envio precisa (mesmo algoritmo do chat do site; o solucionador é pow.js). O corpo é {} e a origem precisa ser um endereço publicado da página. Códigos: 403 `submission_origin_refused`; 404 `form_unknown`; 429 `rate_limited` (por IP; a prévia tem uma cota própria por IP); 503 `form_unavailable`.
// @Tags			Páginas (público)
// @Accept			json
// @Produce		json
// @Param			formKey	path		string	true	"chave pública do formulário"
// @Success		200		{object}	publicform.Challenge
// @Failure		400		{object}	response.ErrorResponse
// @Failure		403		{object}	response.ErrorResponse
// @Failure		404		{object}	response.ErrorResponse
// @Failure		429		{object}	response.ErrorResponse
// @Failure		503		{object}	response.ErrorResponse
// @Router			/public/sites/forms/{formKey}/challenge [post]
func (h *PublicHandler) Challenge(w http.ResponseWriter, r *http.Request) {
	h.cors(w, r)
	w.Header().Set("Cache-Control", "no-store")
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxChallengeBody))
	if err != nil {
		writeError(w, sitesubmission.ErrBodyInvalid)
		return
	}
	var empty struct{}
	if strings.TrimSpace(string(raw)) != "" && strictjson.Decode(raw, &empty) != nil {
		writeError(w, sitesubmission.ErrBodyInvalid)
		return
	}
	challenge, err := h.forms.Challenge(r.Context(), mux.Vars(r)["formKey"], visitorOf(r))
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, challenge)
}

// @Summary		Enviar um formulário de página
// @Description	Recebe o envio do runtime da página: {utm:{source,medium,campaign,term,content}, referrer, fbclid, gclid, fbp, fbc, variant, idempotencyKey, website, route, fields:{nome: texto}, challengeToken, nonce}. Campos desconhecidos são recusados; os campos são validados contra o formulário publicado. Repetir o mesmo idempotencyKey devolve o sucesso original sem criar nada. O sucesso traz a ação configurada: message, redirect (route) ou whatsapp (url). Códigos: 400 `submission_invalid`, `submission_refused`, `challenge_invalid`, `challenge_expired`, `challenge_unsolved`, `challenge_reused` (o runtime busca um novo desafio uma vez); 403 `submission_origin_refused`; 404 `form_unknown`; 422 `submission_fields_invalid` com `details` {campo: código}; 429 `rate_limited` (por IP antes da prova de trabalho; por formulário só depois de uma prova válida); 503 `form_unavailable` (a verificação do desafio está fora do ar; tente de novo). O lead é gravado no envio; responsável, negociação, conversa, automação, inscrição de email, aviso e evento de conversão rodam depois da resposta, com novas tentativas, e nunca atrasam nem bloqueiam o envio.
// @Tags			Páginas (público)
// @Accept			json
// @Produce		json
// @Param			formKey	path		string	true	"chave pública do formulário"
// @Success		200		{object}	SubmitResponse
// @Failure		400		{object}	response.ErrorResponse
// @Failure		403		{object}	response.ErrorResponse
// @Failure		404		{object}	response.ErrorResponse
// @Failure		422		{object}	response.ErrorResponse
// @Failure		429		{object}	response.ErrorResponse
// @Failure		503		{object}	response.ErrorResponse
// @Router			/public/sites/forms/{formKey}/submissions [post]
func (h *PublicHandler) Submit(w http.ResponseWriter, r *http.Request) {
	h.cors(w, r)
	w.Header().Set("Cache-Control", "no-store")
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, sitesubmission.MaxBodyBytes))
	if err != nil {
		writeError(w, sitesubmission.ErrBodyInvalid)
		return
	}
	result, err := h.forms.Submit(r.Context(), mux.Vars(r)["formKey"], visitorOf(r), raw)
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, SubmitResponse{ID: result.SubmissionID, Success: result.Success})
}

type SlotResponse struct {
	Start time.Time `json:"start" example:"2026-10-29T12:00:00Z"`
	End   time.Time `json:"end" example:"2026-10-29T12:30:00Z"`
}

type SlotsResponse struct {
	TimeZone        string         `json:"timeZone" example:"America/Sao_Paulo"`
	DurationMinutes int            `json:"durationMinutes" example:"30"`
	Fields          []string       `json:"fields" example:"email"`
	ConsentText     string         `json:"consentText" example:"Autorizo o contato pelo WhatsApp."`
	Preview         bool           `json:"preview"`
	Slots           []SlotResponse `json:"slots"`
}

type BookedResponse struct {
	Start       time.Time `json:"start" example:"2026-10-29T12:00:00Z"`
	End         time.Time `json:"end" example:"2026-10-29T12:30:00Z"`
	TimeZone    string    `json:"timeZone" example:"America/Sao_Paulo"`
	MeetingLink string    `json:"meetingLink,omitempty" example:"https://meet.google.com/abc-defg-hij"`
}

type BookingResponse struct {
	ID      string                       `json:"id" example:"7d1f0a52-1b0f-4f7e-9a34-0d5f8f1e2c3b"`
	Success sitesubmission.SuccessAction `json:"success"`
	Booking *BookedResponse              `json:"booking,omitempty"`
}

func presentSlots(in siteforms.BookingSlots) SlotsResponse {
	out := SlotsResponse{TimeZone: in.TimeZone, DurationMinutes: in.DurationMinutes, Fields: make([]string, 0, len(in.Fields)), ConsentText: in.ConsentText, Preview: in.Preview, Slots: make([]SlotResponse, 0, len(in.Slots))}
	for _, f := range in.Fields {
		out.Fields = append(out.Fields, string(f))
	}
	for _, slot := range in.Slots {
		out.Slots = append(out.Slots, SlotResponse{Start: slot.Start.UTC(), End: slot.End.UTC()})
	}
	return out
}

// @Summary		Horários livres de um agendamento de página
// @Description	Lista os horários que o visitante pode reservar no bloco de agendamento (vz-booking) desta chave, calculados no servidor a partir das regras do bloco (duração, intervalo entre reuniões, antecedência mínima, dias à frente e horário de atendimento por dia da semana) e do que está ocupado no Google Agenda do workspace. Nunca devolve títulos, participantes ou detalhes de eventos: só início e fim de cada horário livre, em UTC, mais o fuso do negócio, os campos extras pedidos e o texto do consentimento. A lista fica em cache por um minuto. A origem precisa ser um endereço publicado da página (CORS como nos envios); a chave de prévia vz-preview aceita só origens de prévia e devolve horários de exemplo sem ler agenda nenhuma. Códigos: 403 submission_origin_refused; 404 form_unknown (a chave não é de um agendamento publicado); 429 rate_limited (por IP); 503 booking_unavailable (o Google não respondeu ou a conta foi desconectada; nada é mostrado como livre).
// @Tags			Páginas (público)
// @Produce		json
// @Param			formKey	path		string	true	"chave pública do agendamento"
// @Success		200		{object}	SlotsResponse
// @Failure		403		{object}	response.ErrorResponse
// @Failure		404		{object}	response.ErrorResponse
// @Failure		429		{object}	response.ErrorResponse
// @Failure		503		{object}	response.ErrorResponse
// @Router			/public/sites/forms/{formKey}/slots [get]
func (h *PublicHandler) Slots(w http.ResponseWriter, r *http.Request) {
	h.cors(w, r)
	w.Header().Set("Cache-Control", "no-store")
	slots, err := h.forms.Slots(r.Context(), mux.Vars(r)["formKey"], visitorOf(r))
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, presentSlots(slots))
}

// @Summary		Reservar um horário de um agendamento de página
// @Description	Recebe a reserva do runtime da página com o mesmo corpo e a mesma prova de trabalho de um envio de formulário (pegue o desafio em POST /public/sites/forms/{formKey}/challenge), com fields lead.name, lead.phone, lead.email quando pedido, field.mensagem quando pedido, consent e field.horario (o início escolhido em RFC 3339). O servidor confere que o horário é um dos que o bloco oferece, trava a agenda do workspace, confere de novo no Google que o horário (com o intervalo entre reuniões) continua livre e só então cria o evento (com link do Meet quando configurado e convite ao email do visitante quando informado). Depois grava o lead como um formulário da página: funil, etapa, responsável, negociação, aviso, canal de acompanhamento, gatilho do fluxo do formulário e evento de conversão Lead com o id do envio (deduplicado com a API de Conversões). Os campos guardados incluem field.horario, field.horario_fim e field.link_reuniao, que os fluxos recebem. Repetir o mesmo idempotencyKey devolve a reserva original sem criar outro evento; se a gravação falhar, o evento criado é cancelado. Códigos: os mesmos do envio de formulário, mais 409 booking_slot_taken (o horário não é oferecido ou acabou de ser ocupado; o runtime recarrega os horários) e 503 booking_unavailable (o Google não respondeu, a conta foi desconectada ou outra reserva está em andamento; nada é reservado).
// @Tags			Páginas (público)
// @Accept			json
// @Produce		json
// @Param			formKey	path		string	true	"chave pública do agendamento"
// @Success		200		{object}	BookingResponse
// @Failure		400		{object}	response.ErrorResponse
// @Failure		403		{object}	response.ErrorResponse
// @Failure		404		{object}	response.ErrorResponse
// @Failure		409		{object}	response.ErrorResponse
// @Failure		422		{object}	response.ErrorResponse
// @Failure		429		{object}	response.ErrorResponse
// @Failure		503		{object}	response.ErrorResponse
// @Router			/public/sites/forms/{formKey}/bookings [post]
func (h *PublicHandler) Book(w http.ResponseWriter, r *http.Request) {
	h.cors(w, r)
	w.Header().Set("Cache-Control", "no-store")
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, sitesubmission.MaxBodyBytes))
	if err != nil {
		writeError(w, sitesubmission.ErrBodyInvalid)
		return
	}
	result, err := h.forms.Book(r.Context(), mux.Vars(r)["formKey"], visitorOf(r), raw)
	if err != nil {
		writeError(w, err)
		return
	}
	out := BookingResponse{ID: result.SubmissionID, Success: result.Success}
	if b := result.Booked; b != nil {
		out.Booking = &BookedResponse{Start: b.Start.UTC(), End: b.End.UTC(), TimeZone: b.TimeZone, MeetingLink: b.MeetingLink}
	}
	response.WriteSuccess(w, http.StatusOK, out)
}
