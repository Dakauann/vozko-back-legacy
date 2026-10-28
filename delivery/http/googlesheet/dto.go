package googlesheet

import (
	"time"

	"vozko/domain/googlesheet"
	"vozko/domain/shared"
	gsuc "vozko/usecases/googlesheet"
)

type ColumnRequest struct {
	Header       string            `json:"header" example:"Nome do cliente"`
	Type         string            `json:"type" example:"text" enums:"text,long_text,number,money,date,datetime,boolean,choice,email,phone,url,link"`
	Options      []string          `json:"options,omitempty"`
	Colors       map[string]string `json:"colors,omitempty" swaggertype:"object,string"`
	Required     bool              `json:"required"`
	Instruction  string            `json:"instruction,omitempty" example:"Nome completo como o cliente informou"`
	Source       string            `json:"source,omitempty" example:"input" enums:"input,written_at,lead_name,lead_phone,lead_email,conversation_link,origin,message_sent_at,message_sender,message_direction,message_text,message_media_url,export_link"`
	LinkSheetID  string            `json:"linkSheetId,omitempty" example:"8f8e5f7a-3c1d-4a77-9a55-0d6f0f3b2a11"`
	LinkSheetRef string            `json:"linkSheetRef,omitempty" example:"negociacoes"`
	LinkLabel    string            `json:"linkLabel,omitempty" example:"Ver negociação"`
}

func colorsOf(colors map[string]string) map[string]googlesheet.OptionColor {
	if colors == nil {
		return nil
	}
	out := make(map[string]googlesheet.OptionColor, len(colors))
	for option, color := range colors {
		out[option] = googlesheet.OptionColor(color)
	}
	return out
}

func (c ColumnRequest) input() googlesheet.ColumnInput {
	return googlesheet.ColumnInput{
		Header:      c.Header,
		Type:        googlesheet.ColumnType(c.Type),
		Options:     c.Options,
		Colors:      colorsOf(c.Colors),
		Required:    c.Required,
		Instruction: c.Instruction,
		Source:      googlesheet.Source(c.Source),
		LinkSheetID: c.LinkSheetID,
		LinkLabel:   c.LinkLabel,
	}
}

func inputs(columns []ColumnRequest) []googlesheet.ColumnInput {
	out := make([]googlesheet.ColumnInput, len(columns))
	for i, c := range columns {
		out[i] = c.input()
	}
	return out
}

type CreateSheetRequest struct {
	SpreadsheetID string               `json:"spreadsheetId,omitempty" example:"1AbC"`
	FileTitle     string               `json:"fileTitle,omitempty" example:"CDT Jabaquara"`
	Title         string               `json:"title" example:"Pedidos do site"`
	TabName       string               `json:"tabName,omitempty" example:"Pedidos"`
	TabPeriod     string               `json:"tabPeriod,omitempty" example:"none" enums:"none,month"`
	TabPattern    string               `json:"tabPattern,omitempty" example:"Pagamentos · {mes} {ano}"`
	Style         string               `json:"style,omitempty" example:"graphite" enums:"plain,graphite,blue,green,purple"`
	Description   string               `json:"description,omitempty" example:"Pedidos registrados pelo agente de vendas"`
	TimeZone      string               `json:"timeZone,omitempty" example:"America/Sao_Paulo"`
	Columns       []ColumnRequest      `json:"columns"`
	Unique        *UniqueCreateRequest `json:"unique,omitempty"`
}

func (r CreateSheetRequest) input(workspaceID, userID, spreadsheetID string) gsuc.CreateInput {
	return gsuc.CreateInput{
		WorkspaceID: workspaceID, UserID: userID, SpreadsheetID: spreadsheetID, FileTitle: r.FileTitle, Title: r.Title, TabName: r.TabName,
		TabPeriod: googlesheet.TabPeriod(r.TabPeriod), TabPattern: r.TabPattern, Style: googlesheet.StylePreset(r.Style),
		Description: r.Description, TimeZone: r.TimeZone, Columns: inputs(r.Columns),
		Unique: r.Unique.input(),
	}
}

type UpdateDetailsRequest struct {
	Version     int                `json:"version" example:"3"`
	Title       *string            `json:"title,omitempty"`
	Description *string            `json:"description,omitempty"`
	Unique      *UniqueRuleRequest `json:"unique,omitempty"`
}

type ColumnUpdateRequest struct {
	Key         string            `json:"key" example:"valor"`
	Header      *string           `json:"header,omitempty"`
	Instruction *string           `json:"instruction,omitempty"`
	Required    *bool             `json:"required,omitempty"`
	Archived    *bool             `json:"archived,omitempty"`
	Options     []string          `json:"options,omitempty"`
	Colors      map[string]string `json:"colors,omitempty" swaggertype:"object,string"`
	LinkLabel   *string           `json:"linkLabel,omitempty"`
}

type EditColumnsRequest struct {
	Version int                   `json:"version" example:"3"`
	Add     []ColumnRequest       `json:"add,omitempty"`
	Update  []ColumnUpdateRequest `json:"update,omitempty"`
}

func (r EditColumnsRequest) edit() googlesheet.ColumnsEdit {
	edit := googlesheet.ColumnsEdit{Add: inputs(r.Add)}
	for _, u := range r.Update {
		edit.Update = append(edit.Update, googlesheet.ColumnUpdate{
			Key: u.Key, Header: u.Header, Instruction: u.Instruction, Required: u.Required, Archived: u.Archived, Options: u.Options,
			Colors: colorsOf(u.Colors), LinkLabel: u.LinkLabel,
		})
	}
	return edit
}

type ReactivateRequest struct {
	RepairHeader bool `json:"repairHeader" example:"true"`
}

type AddRowRequest struct {
	Values map[string]any `json:"values" swaggertype:"object"`
}

type RowResponse struct {
	ID           string            `json:"id"`
	Values       googlesheet.Cells `json:"values" swaggertype:"object"`
	Status       string            `json:"status" enums:"pending,in_flight,held,failed,written"`
	RowNumber    int               `json:"rowNumber,omitempty"`
	Origin       string            `json:"origin" enums:"agent,workflow,copilot,manual"`
	OriginDetail RowOriginResponse `json:"originDetail"`
	CreatedAt    time.Time         `json:"createdAt"`
	WrittenAt    *time.Time        `json:"writtenAt,omitempty"`
	ErrorCode    string            `json:"errorCode,omitempty"`
	Removal      string            `json:"removal,omitempty" enums:"cancel,remove"`
}

type OriginConversationResponse struct {
	EntryID   string `json:"entryId" example:"c7f1e2a0-9b3d-4a1e-8f2c-1d2e3f4a5b6c"`
	EntryType string `json:"entryType" example:"whatsapp"`
}

type OriginNameResponse struct {
	ID   string `json:"id" example:"8f8e5f7a-3c1d-4a77-9a55-0d6f0f3b2a11"`
	Name string `json:"name" example:"Cobrança"`
}

type RowOriginResponse struct {
	Kind         string                      `json:"kind" enums:"agent,workflow,copilot,manual"`
	Conversation *OriginConversationResponse `json:"conversation,omitempty"`
	Agent        *OriginNameResponse         `json:"agent,omitempty"`
	Workflow     *OriginNameResponse         `json:"workflow,omitempty"`
	RunID        string                      `json:"runId,omitempty" example:"5b1d7c9e-2f4a-4e8b-9c3d-7a6e5f4d3c2b"`
	Person       *OriginNameResponse         `json:"person,omitempty"`
}

type RowListResponse struct {
	Items      []RowResponse `json:"items"`
	Page       int           `json:"page"`
	PageSize   int           `json:"page_size"`
	TotalItems int64         `json:"total_items"`
	TotalPages int           `json:"total_pages"`
}

func nameOf(ref *gsuc.NamedRef) *OriginNameResponse {
	if ref == nil {
		return nil
	}
	return &OriginNameResponse{ID: ref.ID, Name: ref.Name}
}

func toRowOriginResponse(view gsuc.OriginView) RowOriginResponse {
	out := RowOriginResponse{
		Kind: string(view.Kind), Agent: nameOf(view.Agent), Workflow: nameOf(view.Workflow), RunID: view.RunID, Person: nameOf(view.Person),
	}
	if view.Conversation != nil {
		out.Conversation = &OriginConversationResponse{EntryID: view.Conversation.EntryID, EntryType: string(view.Conversation.EntryType)}
	}
	return out
}

func toRowListResponse(page *shared.PaginatedResult[gsuc.RowView]) RowListResponse {
	items := make([]RowResponse, len(page.Items))
	for i, item := range page.Items {
		row := item.Row
		items[i] = RowResponse{
			ID: row.ID, Values: row.Cells, Status: string(row.Status), RowNumber: row.RowNumber, Origin: string(row.Origin.Kind),
			OriginDetail: toRowOriginResponse(item.Origin), CreatedAt: row.CreatedAt, WrittenAt: row.WrittenAt, ErrorCode: row.ErrorCode,
			Removal: removalOf(row),
		}
	}
	return RowListResponse{Items: items, Page: page.Page, PageSize: page.PageSize, TotalItems: page.TotalItems, TotalPages: page.TotalPages}
}

type ColumnResponse struct {
	Key         string            `json:"key" example:"nome_do_cliente"`
	Header      string            `json:"header" example:"Nome do cliente"`
	Type        string            `json:"type" example:"text"`
	Options     []string          `json:"options,omitempty"`
	Colors      map[string]string `json:"colors,omitempty" swaggertype:"object,string"`
	Required    bool              `json:"required"`
	Instruction string            `json:"instruction,omitempty"`
	Source      string            `json:"source" example:"input"`
	LinkSheetID string            `json:"linkSheetId,omitempty"`
	LinkLabel   string            `json:"linkLabel,omitempty"`
	Archived    bool              `json:"archived"`
}

type SheetResponse struct {
	ID            string             `json:"id"`
	SpreadsheetID string             `json:"spreadsheetId"`
	Title         string             `json:"title"`
	TabName       string             `json:"tabName"`
	TabPeriod     string             `json:"tabPeriod" enums:"none,month"`
	TabPattern    string             `json:"tabPattern,omitempty"`
	TabPeriodKey  string             `json:"tabPeriodKey,omitempty" example:"2026-11"`
	TabBaseName   string             `json:"tabBaseName" example:"Pagamentos"`
	Style         string             `json:"style" enums:"plain,graphite,blue,green,purple"`
	TakesInput    bool               `json:"takesInput"`
	Description   string             `json:"description,omitempty"`
	TimeZone      string             `json:"timeZone"`
	URL           string             `json:"url" example:"https://docs.google.com/spreadsheets/d/abc/edit#gid=0"`
	Status        string             `json:"status" example:"active" enums:"active,needs_attention"`
	Reason        string             `json:"reason,omitempty" example:"header_mismatch"`
	ReasonDetail  string             `json:"reasonDetail,omitempty"`
	Repairable    bool               `json:"repairable"`
	Version       int                `json:"version"`
	Columns       []ColumnResponse   `json:"columns"`
	Unique        UniqueRuleResponse `json:"unique"`
	TakesMessages bool               `json:"takesMessages"`
	CreatedAt     time.Time          `json:"createdAt"`
	UpdatedAt     time.Time          `json:"updatedAt"`
	LastWrittenAt *time.Time         `json:"lastWrittenAt,omitempty"`
}

func toSheetResponse(s *googlesheet.Sheet) SheetResponse {
	columns := make([]ColumnResponse, len(s.Columns))
	for i, c := range s.Columns {
		var colors map[string]string
		if len(c.Colors) > 0 {
			colors = make(map[string]string, len(c.Colors))
			for option, color := range c.Colors {
				colors[option] = string(color)
			}
		}
		columns[i] = ColumnResponse{
			Key: c.Key, Header: c.Header, Type: string(c.Type), Options: c.Options, Colors: colors, Required: c.Required,
			Instruction: c.Instruction, Source: string(c.Source), LinkSheetID: c.LinkSheetID, LinkLabel: c.LinkLabel, Archived: c.Archived,
		}
	}
	return SheetResponse{
		ID: s.ID, SpreadsheetID: s.SpreadsheetID, Title: s.Title, TabName: s.TabName, TabPeriod: string(s.TabPeriod), TabPattern: s.TabPattern,
		TabPeriodKey: s.TabPeriodKey, TabBaseName: s.TabBaseName(), Style: string(s.Style), TakesInput: s.TakesInput(),
		Description: s.Description, TimeZone: s.TimeZone, URL: s.URL(),
		Status: string(s.Status), Reason: string(s.Reason), ReasonDetail: s.ReasonDetail, Repairable: s.Repairable(), Version: s.Version,
		Columns: columns, Unique: toUniqueResponse(s.Unique), TakesMessages: s.HasMessageColumns(), CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt, LastWrittenAt: s.LastWrittenAt,
	}
}

type SheetEnvelope struct {
	Sheet SheetResponse `json:"sheet"`
}

type SheetListResponse struct {
	Items      []SheetResponse `json:"items"`
	Page       int             `json:"page"`
	PageSize   int             `json:"pageSize"`
	TotalItems int64           `json:"totalItems"`
	TotalPages int             `json:"totalPages"`
}

func toListResponse(page *shared.PaginatedResult[*googlesheet.Sheet]) SheetListResponse {
	items := make([]SheetResponse, len(page.Items))
	for i, s := range page.Items {
		items[i] = toSheetResponse(s)
	}
	return SheetListResponse{Items: items, Page: page.Page, PageSize: page.PageSize, TotalItems: page.TotalItems, TotalPages: page.TotalPages}
}

type SheetFileResponse struct {
	SpreadsheetID string          `json:"spreadsheetId" example:"1AbC"`
	Title         string          `json:"title" example:"Altiva: negociações"`
	URL           string          `json:"url" example:"https://docs.google.com/spreadsheets/d/1AbC/edit"`
	Status        string          `json:"status" example:"active" enums:"active,needs_attention"`
	LastWrittenAt *time.Time      `json:"lastWrittenAt,omitempty"`
	Sheets        []SheetResponse `json:"sheets"`
}

type SheetFileListResponse struct {
	Items      []SheetFileResponse `json:"items"`
	Page       int                 `json:"page"`
	PageSize   int                 `json:"pageSize"`
	TotalItems int64               `json:"totalItems"`
	TotalPages int                 `json:"totalPages"`
}

type SheetFileEnvelope struct {
	File SheetFileResponse `json:"file"`
}

type RenameFileRequest struct {
	Title string `json:"title" example:"CDT Jabaquara"`
}

func toFileResponse(file googlesheet.Spreadsheet) SheetFileResponse {
	sheets := make([]SheetResponse, len(file.Sheets))
	for i, s := range file.Sheets {
		sheets[i] = toSheetResponse(s)
	}
	return SheetFileResponse{
		SpreadsheetID: file.ID, Title: file.Title(), URL: file.URL(), Status: string(file.Status()),
		LastWrittenAt: file.LastWrittenAt(), Sheets: sheets,
	}
}

func toFileListResponse(page *shared.PaginatedResult[googlesheet.Spreadsheet]) SheetFileListResponse {
	items := make([]SheetFileResponse, len(page.Items))
	for i, file := range page.Items {
		items[i] = toFileResponse(file)
	}
	return SheetFileListResponse{Items: items, Page: page.Page, PageSize: page.PageSize, TotalItems: page.TotalItems, TotalPages: page.TotalPages}
}

type ProblemRowResponse struct {
	ID          string    `json:"id"`
	Status      string    `json:"status" enums:"held,failed"`
	ErrorCode   string    `json:"errorCode" example:"google_sheet_write_access_lost"`
	ErrorDetail string    `json:"errorDetail,omitempty"`
	Attempts    int       `json:"attempts"`
	Origin      string    `json:"origin" enums:"agent,workflow,copilot,manual"`
	CreatedAt   time.Time `json:"createdAt"`
}

type HealthResponse struct {
	Sheet           SheetResponse        `json:"sheet"`
	Pending         int                  `json:"pending"`
	InFlight        int                  `json:"inFlight"`
	Held            int                  `json:"held"`
	Failed          int                  `json:"failed"`
	Written         int                  `json:"written"`
	WrittenCapped   bool                 `json:"writtenCapped"`
	OldestPendingAt *time.Time           `json:"oldestPendingAt,omitempty"`
	Problems        []ProblemRowResponse `json:"problems"`
}

func toHealthResponse(view *gsuc.HealthView) HealthResponse {
	h := view.Health
	problems := make([]ProblemRowResponse, len(h.Problems))
	for i, row := range h.Problems {
		problems[i] = ProblemRowResponse{
			ID: row.ID, Status: string(row.Status), ErrorCode: row.ErrorCode, ErrorDetail: row.ErrorDetail,
			Attempts: row.Attempts, Origin: string(row.Origin.Kind), CreatedAt: row.CreatedAt,
		}
	}
	return HealthResponse{
		Sheet: toSheetResponse(view.Sheet), Pending: h.Pending, InFlight: h.InFlight, Held: h.Held, Failed: h.Failed,
		Written: h.Written, WrittenCapped: h.Written >= googlesheet.HealthCountCap, OldestPendingAt: h.OldestPendingAt, Problems: problems,
	}
}

type RetryResponse struct {
	Requeued int `json:"requeued"`
}

type AddRowResponse struct {
	RowID     string `json:"rowId"`
	Duplicate bool   `json:"duplicate"`
}

type DeletedResponse struct {
	Deleted bool `json:"deleted" example:"true"`
}

type SourceResponse struct {
	Source string `json:"source" example:"lead_phone"`
	Type   string `json:"type" example:"phone"`
}

type StyleResponse struct {
	Key        string `json:"key" example:"green"`
	Header     string `json:"header,omitempty" example:"#1E7A4C"`
	HeaderText string `json:"headerText,omitempty" example:"#FFFFFF"`
	Band       string `json:"band,omitempty" example:"#F2F8F4"`
}

type OptionColorResponse struct {
	Key  string `json:"key" example:"green"`
	Fill string `json:"fill" example:"#1E8E3E"`
	Text string `json:"text" example:"#FFFFFF"`
}

type CatalogResponse struct {
	Types             []string              `json:"types"`
	Sources           []SourceResponse      `json:"sources"`
	Styles            []StyleResponse       `json:"styles"`
	OptionColors      []OptionColorResponse `json:"optionColors"`
	TabPeriods        []string              `json:"tabPeriods"`
	TabTokens         []string              `json:"tabTokens"`
	DefaultStyle      string                `json:"defaultStyle"`
	DefaultLinkLabel  string                `json:"defaultLinkLabel"`
	DefaultTimeZone   string                `json:"defaultTimeZone"`
	MaxHeader         int                   `json:"maxHeader"`
	MaxInstruction    int                   `json:"maxInstruction"`
	MaxOption         int                   `json:"maxOption"`
	MaxTitle          int                   `json:"maxTitle"`
	MaxTabName        int                   `json:"maxTabName"`
	MaxLinkLabel      int                   `json:"maxLinkLabel"`
	MaxRemovalReason  int                   `json:"maxRemovalReason"`
	RowErrorCodes     []string              `json:"rowErrorCodes"`
	AttentionReasons  []string              `json:"attentionReasons"`
	WrittenRetainDays int                   `json:"writtenRetainDays"`
	UniqueModes       []string              `json:"uniqueModes"`
	RepeatActions     []string              `json:"repeatActions"`
	UniqueSources     []string              `json:"uniqueSources"`
}

func catalog() CatalogResponse {
	out := CatalogResponse{
		DefaultTimeZone: googlesheet.DefaultTimeZone, MaxHeader: googlesheet.MaxHeaderRunes,
		MaxInstruction: googlesheet.MaxInstructionRunes, MaxOption: googlesheet.MaxOptionRunes,
		MaxTitle: googlesheet.MaxTitleRunes, MaxTabName: googlesheet.MaxTabRunes, MaxLinkLabel: googlesheet.MaxLinkLabelRunes,
		MaxRemovalReason: googlesheet.MaxRemovalReasonRunes, DefaultStyle: string(googlesheet.DefaultStyle), DefaultLinkLabel: googlesheet.DefaultLinkLabel,
		TabTokens:         googlesheet.TabTokens(),
		RowErrorCodes:     googlesheet.RowErrorCodes(),
		WrittenRetainDays: int(gsuc.WrittenRowRetention.Hours() / 24),
	}
	for _, style := range googlesheet.StylePresets() {
		p := style.Palette()
		out.Styles = append(out.Styles, StyleResponse{Key: string(style), Header: p.Header, HeaderText: p.HeaderText, Band: p.Band})
	}
	for _, color := range googlesheet.OptionColors() {
		fill, text := color.Hex()
		out.OptionColors = append(out.OptionColors, OptionColorResponse{Key: string(color), Fill: fill, Text: text})
	}
	for _, period := range googlesheet.TabPeriods() {
		out.TabPeriods = append(out.TabPeriods, string(period))
	}
	for _, t := range googlesheet.ColumnTypes() {
		out.Types = append(out.Types, string(t))
	}
	for _, s := range googlesheet.SystemSources() {
		t, _ := s.ColumnType()
		out.Sources = append(out.Sources, SourceResponse{Source: string(s), Type: string(t)})
	}
	for _, r := range googlesheet.AttentionReasons() {
		out.AttentionReasons = append(out.AttentionReasons, string(r))
	}
	uniqueCatalog(&out)
	return out
}

func removalOf(row *googlesheet.Row) string {
	kind, _ := row.Removal()
	return string(kind)
}
