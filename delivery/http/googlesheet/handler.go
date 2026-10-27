package googlesheet

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/mux"

	"vozko/delivery/http/httpx"
	"vozko/delivery/http/response"
	"vozko/domain/googleaccount"
	"vozko/domain/googlesheet"
	"vozko/domain/shared"
	"vozko/infra/http/middleware"
	gsuc "vozko/usecases/googlesheet"
)

const maxRequestBody = 256 << 10

type Sheets interface {
	ListRows(ctx context.Context, workspaceID, id string, pagination shared.Pagination) (*shared.PaginatedResult[*googlesheet.Row], error)
	Create(ctx context.Context, in gsuc.CreateInput) (*googlesheet.Sheet, error)
	Get(ctx context.Context, workspaceID, id string) (*googlesheet.Sheet, error)
	List(ctx context.Context, in googlesheet.ListInput) (*shared.PaginatedResult[*googlesheet.Sheet], error)
	UpdateDetails(ctx context.Context, in gsuc.DetailsInput) (*googlesheet.Sheet, error)
	EditColumns(ctx context.Context, in gsuc.ColumnsInput) (*googlesheet.Sheet, error)
	Delete(ctx context.Context, workspaceID, id string) error
	Health(ctx context.Context, workspaceID, id string) (*gsuc.HealthView, error)
	RetryProblems(ctx context.Context, workspaceID, id string) (int, error)
	Reactivate(ctx context.Context, in gsuc.ReactivateInput) (*googlesheet.Sheet, error)
	CreateWorkbook(ctx context.Context, in gsuc.WorkbookInput) ([]*googlesheet.Sheet, error)
	UpdateLayout(ctx context.Context, in gsuc.LayoutInput) (*googlesheet.Sheet, error)
	Workbooks(ctx context.Context, workspaceID string) ([]gsuc.WorkbookView, error)
	PreviewTabName(pattern, timeZone string) (string, string, error)
}

func (h *Handler) ListRows(w http.ResponseWriter, r *http.Request) {
	page, err := h.sheets.ListRows(r.Context(), middleware.GetWorkspaceID(r), mux.Vars(r)["id"], httpx.ParsePagination(r.URL.Query()))
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]RowResponse, 0, len(page.Items))
	for _, row := range page.Items {
		items = append(items, RowResponse{ID: row.ID, Values: row.Cells, Status: string(row.Status), RowNumber: row.RowNumber, Origin: string(row.Origin.Kind), CreatedAt: row.CreatedAt, WrittenAt: row.WrittenAt, ErrorCode: row.ErrorCode})
	}
	response.WriteSuccess(w, http.StatusOK, shared.PaginatedResult[RowResponse]{Items: items, Page: page.Page, PageSize: page.PageSize, TotalItems: page.TotalItems, TotalPages: page.TotalPages})
}

type Rows interface {
	Enqueue(ctx context.Context, in gsuc.EnqueueInput) (*gsuc.EnqueueResult, error)
}

type Handler struct {
	sheets Sheets
	rows   Rows
}

func NewHandler(sheets Sheets, rows Rows) *Handler {
	if sheets == nil || rows == nil {
		panic("googlesheet http: the handler requires sheets and rows")
	}
	return &Handler{sheets: sheets, rows: rows}
}

// @Summary		Catálogo das colunas de planilha
// @Description	Tipos de coluna, valores preenchidos pelo sistema (com o tipo que cada um exige), limites, códigos de erro das linhas e motivos de atenção. A tela de colunas usa este catálogo em vez de repetir as regras.
// @Tags			Google Planilhas
// @Produce		json
// @Success		200	{object}	CatalogResponse
// @Security		BearerAuth
// @Router			/google-sheets/catalog [get]
func (h *Handler) Catalog(w http.ResponseWriter, _ *http.Request) {
	response.WriteSuccess(w, http.StatusOK, catalog())
}

// @Summary		Listar planilhas
// @Tags			Google Planilhas
// @Produce		json
// @Param			search		query		string	false	"Busca pelo título"
// @Param			takesInput	query		bool	false	"Só planilhas com colunas preenchidas por agentes ou fluxos"
// @Param			page		query		int		false	"Página"
// @Param			pageSize	query		int		false	"Tamanho da página"
// @Success		200			{object}	SheetListResponse
// @Security		BearerAuth
// @Router			/google-sheets [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, err := h.sheets.List(r.Context(), googlesheet.ListInput{
		WorkspaceID: middleware.GetWorkspaceID(r),
		Search:      strings.TrimSpace(q.Get("search")),
		TakesInput:  q.Get("takesInput") == "true",
		Pagination:  httpx.ParsePagination(q),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, toListResponse(page))
}

// @Summary		Criar planilha
// @Description	Cria a planilha no Google Drive da conta conectada ao Google Planilhas, com o cabeçalho, os formatos, as validações, o estilo e as cores das opções, e a registra no workspace. Com spreadsheetId, cria uma aba nova num arquivo que já tem planilhas deste workspace. Com tabPeriod=month, cada mês ganha a sua aba, nomeada pelo tabPattern. Definições recusadas devolvem um código próprio com field, column, header, option e limit em details.
// @Tags			Google Planilhas
// @Accept			json
// @Produce		json
// @Param			request	body		CreateSheetRequest	true	"Planilha"
// @Success		201		{object}	SheetEnvelope
// @Failure		400		{object}	response.ErrorResponse
// @Failure		412		{object}	response.ErrorResponse
// @Failure		429		{object}	response.ErrorResponse
// @Failure		502		{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/google-sheets [post]
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateSheetRequest
	if !decode(w, r, &req) {
		return
	}
	userID := userOf(r)
	sheet, err := h.sheets.Create(r.Context(), req.input(middleware.GetWorkspaceID(r), userID, req.SpreadsheetID))
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusCreated, SheetEnvelope{Sheet: toSheetResponse(sheet)})
}

// @Summary		Consultar planilha
// @Tags			Google Planilhas
// @Produce		json
// @Param			id	path		string	true	"Planilha"
// @Success		200	{object}	SheetEnvelope
// @Failure		404	{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/google-sheets/{id} [get]
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	sheet, err := h.sheets.Get(r.Context(), middleware.GetWorkspaceID(r), mux.Vars(r)["id"])
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, SheetEnvelope{Sheet: toSheetResponse(sheet)})
}

// @Summary		Editar título e descrição
// @Description	Muda o nome e a descrição da planilha no painel. O arquivo no Google Drive mantém o nome que tem lá.
// @Tags			Google Planilhas
// @Accept			json
// @Produce		json
// @Param			id		path		string					true	"Planilha"
// @Param			request	body		UpdateDetailsRequest	true	"Alterações"
// @Success		200		{object}	SheetEnvelope
// @Failure		409		{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/google-sheets/{id} [patch]
func (h *Handler) UpdateDetails(w http.ResponseWriter, r *http.Request) {
	var req UpdateDetailsRequest
	if !decode(w, r, &req) {
		return
	}
	sheet, err := h.sheets.UpdateDetails(r.Context(), gsuc.DetailsInput{
		WorkspaceID: middleware.GetWorkspaceID(r), ID: mux.Vars(r)["id"], Version: req.Version, Title: req.Title, Description: req.Description,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, SheetEnvelope{Sheet: toSheetResponse(sheet)})
}

// @Summary		Editar colunas
// @Description	Adiciona colunas ao fim, renomeia cabeçalhos, muda instruções e obrigatoriedade, arquiva colunas e acrescenta opções. O cabeçalho no Google é atualizado na mesma operação. Tipo e chave de uma coluna não mudam.
// @Tags			Google Planilhas
// @Accept			json
// @Produce		json
// @Param			id		path		string				true	"Planilha"
// @Param			request	body		EditColumnsRequest	true	"Alterações"
// @Success		200		{object}	SheetEnvelope
// @Failure		400		{object}	response.ErrorResponse
// @Failure		409		{object}	response.ErrorResponse
// @Failure		502		{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/google-sheets/{id}/columns [patch]
func (h *Handler) EditColumns(w http.ResponseWriter, r *http.Request) {
	var req EditColumnsRequest
	if !decode(w, r, &req) {
		return
	}
	sheet, err := h.sheets.EditColumns(r.Context(), gsuc.ColumnsInput{
		WorkspaceID: middleware.GetWorkspaceID(r), ID: mux.Vars(r)["id"], Version: req.Version, Edit: req.edit(),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, SheetEnvelope{Sheet: toSheetResponse(sheet)})
}

// @Summary		Remover planilha
// @Description	Remove a planilha do workspace e descarta as linhas que ainda não foram gravadas. O arquivo continua no Google Drive.
// @Tags			Google Planilhas
// @Produce		json
// @Param			id	path		string	true	"Planilha"
// @Success		200	{object}	DeletedResponse
// @Failure		404	{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/google-sheets/{id} [delete]
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.sheets.Delete(r.Context(), middleware.GetWorkspaceID(r), mux.Vars(r)["id"]); err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, DeletedResponse{Deleted: true})
}

// @Summary		Saúde da planilha
// @Description	Linhas aguardando, em envio, retidas, recusadas e gravadas nos últimos dias, com as linhas com problema mais recentes.
// @Tags			Google Planilhas
// @Produce		json
// @Param			id	path		string	true	"Planilha"
// @Success		200	{object}	HealthResponse
// @Failure		404	{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/google-sheets/{id}/health [get]
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	view, err := h.sheets.Health(r.Context(), middleware.GetWorkspaceID(r), mux.Vars(r)["id"])
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, toHealthResponse(view))
}

// @Summary		Reenviar linhas com problema
// @Description	Devolve à fila as linhas retidas e recusadas da planilha. A planilha precisa estar ativa.
// @Tags			Google Planilhas
// @Produce		json
// @Param			id	path		string	true	"Planilha"
// @Success		200	{object}	RetryResponse
// @Failure		409	{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/google-sheets/{id}/retry [post]
func (h *Handler) Retry(w http.ResponseWriter, r *http.Request) {
	requeued, err := h.sheets.RetryProblems(r.Context(), middleware.GetWorkspaceID(r), mux.Vars(r)["id"])
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, RetryResponse{Requeued: requeued})
}

// @Summary		Reativar planilha
// @Description	Confere o acesso, a aba e o cabeçalho no Google e volta a gravar. Com repairHeader=true reescreve o cabeçalho a partir das colunas do painel.
// @Tags			Google Planilhas
// @Accept			json
// @Produce		json
// @Param			id		path		string				true	"Planilha"
// @Param			request	body		ReactivateRequest	true	"Opções"
// @Success		200		{object}	SheetEnvelope
// @Failure		409		{object}	response.ErrorResponse
// @Failure		502		{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/google-sheets/{id}/reactivate [post]
func (h *Handler) Reactivate(w http.ResponseWriter, r *http.Request) {
	var req ReactivateRequest
	if !decode(w, r, &req) {
		return
	}
	sheet, err := h.sheets.Reactivate(r.Context(), gsuc.ReactivateInput{
		WorkspaceID: middleware.GetWorkspaceID(r), ID: mux.Vars(r)["id"], RepairHeader: req.RepairHeader,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, SheetEnvelope{Sheet: toSheetResponse(sheet)})
}

// @Summary		Criar planilhas num mesmo arquivo
// @Description	Cria uma ou mais planilhas como abas de um só arquivo do Google, na ordem pedida. Sem spreadsheetId, a primeira cria o arquivo e as outras viram abas dele. Uma coluna de link pode apontar para uma planilha anterior do pedido por linkSheetRef. Se uma falhar, as que já foram criadas são desfeitas; details.sheet diz qual falhou.
// @Tags			Google Planilhas
// @Accept			json
// @Produce		json
// @Param			request	body		WorkbookRequest	true	"Planilhas"
// @Success		201		{object}	WorkbookResponse
// @Failure		400		{object}	response.ErrorResponse
// @Failure		409		{object}	response.ErrorResponse
// @Failure		412		{object}	response.ErrorResponse
// @Failure		502		{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/google-sheets/workbook [post]
func (h *Handler) CreateWorkbook(w http.ResponseWriter, r *http.Request) {
	var req WorkbookRequest
	if !decode(w, r, &req) {
		return
	}
	sheets, err := h.sheets.CreateWorkbook(r.Context(), req.input(middleware.GetWorkspaceID(r), userOf(r)))
	if err != nil {
		writeError(w, err)
		return
	}
	out := WorkbookResponse{Sheets: make([]SheetResponse, len(sheets))}
	for i, sheet := range sheets {
		out.Sheets[i] = toSheetResponse(sheet)
	}
	response.WriteSuccess(w, http.StatusCreated, out)
}

// @Summary		Mudar estilo e nome das abas por período
// @Description	Troca o estilo da tabela e o padrão do nome das abas por período. O estilo é aplicado na aba atual na mesma operação; abas de meses anteriores ficam como estão.
// @Tags			Google Planilhas
// @Accept			json
// @Produce		json
// @Param			id		path		string			true	"Planilha"
// @Param			request	body		LayoutRequest	true	"Alterações"
// @Success		200		{object}	SheetEnvelope
// @Failure		400		{object}	response.ErrorResponse
// @Failure		409		{object}	response.ErrorResponse
// @Failure		502		{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/google-sheets/{id}/layout [patch]
func (h *Handler) UpdateLayout(w http.ResponseWriter, r *http.Request) {
	var req LayoutRequest
	if !decode(w, r, &req) {
		return
	}
	sheet, err := h.sheets.UpdateLayout(r.Context(), gsuc.LayoutInput{
		WorkspaceID: middleware.GetWorkspaceID(r), ID: mux.Vars(r)["id"], Version: req.Version, Style: req.style(), TabPattern: req.TabPattern,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, SheetEnvelope{Sheet: toSheetResponse(sheet)})
}

// @Summary		Arquivos de planilhas do workspace
// @Description	Os arquivos do Google em que o workspace tem planilhas, com as planilhas de cada um e as abas por período já criadas. usable diz se a conta conectada pode criar abas novas no arquivo.
// @Tags			Google Planilhas
// @Produce		json
// @Success		200	{object}	SpreadsheetListResponse
// @Security		BearerAuth
// @Router			/google-sheets/spreadsheets [get]
func (h *Handler) Spreadsheets(w http.ResponseWriter, r *http.Request) {
	views, err := h.sheets.Workbooks(r.Context(), middleware.GetWorkspaceID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, toSpreadsheets(views))
}

// @Summary		Modelos de planilha
// @Description	Modelos prontos com colunas, fontes, tipos, opções, cores, estilo e abas por período. Os textos (nome, cabeçalhos, opções) vêm dos catálogos do painel pela chave de cada item.
// @Tags			Google Planilhas
// @Produce		json
// @Success		200	{object}	PresetListResponse
// @Security		BearerAuth
// @Router			/google-sheets/presets [get]
func (h *Handler) Presets(w http.ResponseWriter, _ *http.Request) {
	response.WriteSuccess(w, http.StatusOK, presets())
}

// @Summary		Prévia do nome da aba por período
// @Description	Mostra o nome que a aba do período atual terá com o padrão informado, ou recusa o padrão com o código do problema.
// @Tags			Google Planilhas
// @Accept			json
// @Produce		json
// @Param			request	body		TabNamePreviewRequest	true	"Padrão"
// @Success		200		{object}	TabNamePreviewResponse
// @Failure		400		{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/google-sheets/tab-name-preview [post]
func (h *Handler) PreviewTabName(w http.ResponseWriter, r *http.Request) {
	var req TabNamePreviewRequest
	if !decode(w, r, &req) {
		return
	}
	title, period, err := h.sheets.PreviewTabName(req.Pattern, req.TimeZone)
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, TabNamePreviewResponse{Title: title, Period: period})
}

func userOf(r *http.Request) string {
	if claims := middleware.GetClaims(r); claims != nil {
		return claims.UserID
	}
	return ""
}

// @Summary		Adicionar linha
// @Description	Valida os valores com as mesmas regras de agentes e fluxos e põe a linha na fila de gravação. O cabeçalho Idempotency-Key é obrigatório; repetir a chave devolve a mesma linha.
// @Tags			Google Planilhas
// @Accept			json
// @Produce		json
// @Param			id				path		string			true	"Planilha"
// @Param			Idempotency-Key	header		string			true	"Chave única da operação"
// @Param			request			body		AddRowRequest	true	"Valores por chave de coluna"
// @Success		202				{object}	AddRowResponse
// @Failure		400				{object}	response.ErrorResponse
// @Failure		422				{object}	response.ErrorResponse
// @Security		BearerAuth
// @Router			/google-sheets/{id}/rows [post]
func (h *Handler) AddRow(w http.ResponseWriter, r *http.Request) {
	var req AddRowRequest
	if !decode(w, r, &req) {
		return
	}
	userID := userOf(r)
	clientKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if clientKey == "" {
		writeError(w, googlesheet.ErrIdempotencyKeyRequired)
		return
	}
	result, err := h.rows.Enqueue(r.Context(), gsuc.EnqueueInput{
		WorkspaceID:    middleware.GetWorkspaceID(r),
		SheetID:        mux.Vars(r)["id"],
		Values:         req.Values,
		IdempotencyKey: googlesheet.KeyForManual(userID, clientKey),
		Origin:         googlesheet.Origin{Kind: googlesheet.OriginManual, ActorID: userID},
	})
	if err != nil {
		writeError(w, err)
		return
	}
	response.WriteSuccess(w, http.StatusAccepted, AddRowResponse{RowID: result.RowID, Duplicate: result.Duplicate})
}

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxRequestBody))
	decoder.UseNumber()
	if err := decoder.Decode(into); err != nil {
		response.WriteErrorWithCode(w, http.StatusBadRequest, "invalid_body", "Corpo da requisição inválido", nil)
		return false
	}
	return true
}

func refusal(status int, code, message string) response.Refusal {
	return response.Refusal{Status: status, Code: code, Message: message}
}

var refusals = []response.ErrorRefusal{
	{Err: googleaccount.ErrNotConnected, Refusal: refusal(http.StatusPreconditionFailed, googleaccount.CodeNotConnected, "Conecte o Google Planilhas antes")},
	{Err: googlesheet.ErrSheetNotFound, Refusal: refusal(http.StatusNotFound, googlesheet.CodeSheetNotFound, "Planilha não encontrada")},
	{Err: googlesheet.ErrStaleSheet, Refusal: refusal(http.StatusConflict, googlesheet.CodeStaleSheet, "A planilha mudou; recarregue e tente de novo")},
	{Err: googlesheet.ErrSheetNotActive, Refusal: refusal(http.StatusConflict, googlesheet.CodeSheetNotActive, "A planilha precisa de atenção antes de receber linhas")},
	{Err: googlesheet.ErrAccountChanged, Refusal: refusal(http.StatusConflict, googlesheet.CodeAccountChanged, "A conta Google conectada não é a que criou a planilha")},
	{Err: googlesheet.ErrGoogleBusy, Refusal: refusal(http.StatusTooManyRequests, googlesheet.CodeGoogleBusy, "O limite do Google para este minuto foi atingido; tente de novo em um minuto")},
	{Err: googlesheet.ErrInvalidSheet, Refusal: refusal(http.StatusBadRequest, googlesheet.CodeInvalidSheet, "Definição de planilha inválida")},
	{Err: googlesheet.ErrIdempotencyKeyRequired, Refusal: refusal(http.StatusBadRequest, googlesheet.CodeIdempotencyKeyRequired, "Informe o cabeçalho Idempotency-Key")},
	{Err: googlesheet.ErrRowTooLarge, Refusal: refusal(http.StatusRequestEntityTooLarge, googlesheet.CodeRowTooLarge, "A linha é grande demais para uma gravação")},
	{Err: googlesheet.ErrNoMessageColumns, Refusal: refusal(http.StatusUnprocessableEntity, googlesheet.CodeNoMessageColumns, "A planilha não tem colunas preenchidas por mensagem")},
	{Err: googlesheet.ErrNeedsConversation, Refusal: refusal(http.StatusUnprocessableEntity, googlesheet.CodeNeedsConversation, "Exportar mensagens precisa de uma conversa")},
	{Err: googlesheet.ErrNoInputColumns, Refusal: refusal(http.StatusUnprocessableEntity, googlesheet.CodeNoInputColumns, "A planilha não tem colunas preenchidas por agentes ou fluxos")},
	{Err: googlesheet.ErrSpreadsheetNotFound, Refusal: refusal(http.StatusNotFound, googlesheet.CodeSpreadsheetNotFound, "Nenhuma planilha deste workspace está nesse arquivo")},
	{Err: googlesheet.ErrTabNameTaken, Refusal: refusal(http.StatusConflict, googlesheet.CodeTabNameTaken, "O arquivo já tem uma aba com esse nome")},
	{Err: googlesheet.ErrLinkTargetInvalid, Refusal: refusal(http.StatusUnprocessableEntity, googlesheet.CodeLinkTargetInvalid, "O link precisa apontar para outra planilha de mensagens do mesmo arquivo")},
	{Err: googlesheet.ErrPresetNotFound, Refusal: refusal(http.StatusNotFound, googlesheet.CodePresetNotFound, "Modelo não encontrado")},
}

type valueRefusal struct {
	Sheet  *int   `json:"sheet,omitempty"`
	Column string `json:"column"`
	Header string `json:"header"`
	Detail string `json:"detail,omitempty"`
}

type definitionRefusal struct {
	Sheet  *int   `json:"sheet,omitempty"`
	Field  string `json:"field"`
	Column *int   `json:"column,omitempty"`
	Key    string `json:"key,omitempty"`
	Header string `json:"header,omitempty"`
	Option string `json:"option,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type sheetRefusal struct {
	Sheet  *int   `json:"sheet,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type googleRefusal struct {
	Sheet   *int   `json:"sheet,omitempty"`
	Failure string `json:"failure"`
	Detail  string `json:"detail,omitempty"`
}

func writeError(w http.ResponseWriter, err error) {
	var sheet *int
	var workbookErr *gsuc.WorkbookError
	if errors.As(err, &workbookErr) {
		index := workbookErr.Sheet
		sheet = &index
	}
	var definitionErr *googlesheet.DefinitionError
	if errors.As(err, &definitionErr) {
		details := definitionRefusal{
			Sheet: sheet, Field: definitionErr.Field, Key: definitionErr.Key, Header: definitionErr.Header,
			Option: definitionErr.Option, Limit: definitionErr.Limit, Detail: definitionErr.Detail,
		}
		if definitionErr.Column >= 0 {
			column := definitionErr.Column
			details.Column = &column
		}
		response.WriteErrorWithDetails(w, http.StatusBadRequest, definitionErr.Code, definitionErr.Error(), details)
		return
	}
	var valueErr *googlesheet.ValueError
	if errors.As(err, &valueErr) {
		response.WriteErrorWithDetails(w, http.StatusUnprocessableEntity, googlesheet.ErrorCode(err), valueErr.Error(),
			valueRefusal{Sheet: sheet, Column: valueErr.Column, Header: valueErr.Header, Detail: valueErr.Detail})
		return
	}
	var googleErr *gsuc.GoogleRefusal
	if errors.As(err, &googleErr) {
		response.WriteErrorWithDetails(w, http.StatusBadGateway, googleErr.Failure.Code(), "O Google recusou a operação",
			googleRefusal{Sheet: sheet, Failure: string(googleErr.Failure.Kind), Detail: googleErr.Failure.Detail})
		return
	}
	if matched, ok := response.MatchRefusal(err, refusals); ok {
		response.WriteErrorWithDetails(w, matched.Status, matched.Code, matched.Message, sheetRefusal{Sheet: sheet, Detail: err.Error()})
		return
	}
	log.Printf("[googlesheet] request failed: %v", err)
	response.WriteErrorWithCode(w, http.StatusInternalServerError, googlesheet.CodeInternal, "Não foi possível concluir a operação com a planilha", nil)
}

func RefusalCodes() []string {
	return append(response.RefusalCodes(response.ErrorRefusalsOf(refusals)), googlesheet.CodeInternal, "invalid_body")
}
