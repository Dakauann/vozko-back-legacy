package googlesheet

import (
	"vozko/domain/googlesheet"
	gsuc "vozko/usecases/googlesheet"
)

type WorkbookSheetRequest struct {
	Ref string `json:"ref,omitempty" example:"pagamentos"`
	CreateSheetRequest
}

type WorkbookRequest struct {
	SpreadsheetID string                 `json:"spreadsheetId,omitempty" example:"1AbC"`
	Sheets        []WorkbookSheetRequest `json:"sheets"`
}

func (r WorkbookRequest) input(workspaceID, userID string) gsuc.WorkbookInput {
	in := gsuc.WorkbookInput{WorkspaceID: workspaceID, UserID: userID, SpreadsheetID: r.SpreadsheetID}
	for _, sheet := range r.Sheets {
		item := gsuc.WorkbookSheet{Ref: sheet.Ref, Input: sheet.CreateSheetRequest.input(workspaceID, userID, "")}
		for i, col := range sheet.Columns {
			if col.LinkSheetRef != "" {
				if item.LinkRefs == nil {
					item.LinkRefs = map[int]string{}
				}
				item.LinkRefs[i] = col.LinkSheetRef
			}
		}
		in.Sheets = append(in.Sheets, item)
	}
	return in
}

type WorkbookResponse struct {
	Sheets []SheetResponse `json:"sheets"`
}

type LayoutRequest struct {
	Version    int     `json:"version" example:"3"`
	Style      *string `json:"style,omitempty" example:"green" enums:"plain,graphite,blue,green,purple"`
	TabPattern *string `json:"tabPattern,omitempty" example:"Pagamentos · {mes} {ano}"`
}

func (r LayoutRequest) style() *googlesheet.StylePreset {
	if r.Style == nil {
		return nil
	}
	style := googlesheet.StylePreset(*r.Style)
	return &style
}

type TabResponse struct {
	Period string `json:"period" example:"2026-11"`
	Title  string `json:"title" example:"Pagamentos · novembro 2026"`
}

type SpreadsheetSheetResponse struct {
	ID            string        `json:"id"`
	Title         string        `json:"title"`
	TabName       string        `json:"tabName"`
	TabPeriod     string        `json:"tabPeriod" enums:"none,month"`
	Status        string        `json:"status" enums:"active,needs_attention"`
	TakesMessages bool          `json:"takesMessages"`
	Tabs          []TabResponse `json:"tabs"`
}

type SpreadsheetResponse struct {
	SpreadsheetID string                     `json:"spreadsheetId"`
	Title         string                     `json:"title"`
	URL           string                     `json:"url"`
	Usable        bool                       `json:"usable"`
	Sheets        []SpreadsheetSheetResponse `json:"sheets"`
}

type SpreadsheetListResponse struct {
	Items []SpreadsheetResponse `json:"items"`
}

func toSpreadsheets(views []gsuc.WorkbookView) SpreadsheetListResponse {
	out := SpreadsheetListResponse{Items: make([]SpreadsheetResponse, 0, len(views))}
	for _, view := range views {
		item := SpreadsheetResponse{SpreadsheetID: view.SpreadsheetID, Title: view.Title, URL: view.URL, Usable: view.Usable}
		for _, entry := range view.Sheets {
			s := entry.Sheet
			sheet := SpreadsheetSheetResponse{
				ID: s.ID, Title: s.Title, TabName: s.TabName, TabPeriod: string(s.TabPeriod), Status: string(s.Status),
				TakesMessages: s.HasMessageColumns(), Tabs: make([]TabResponse, 0, len(entry.Tabs)),
			}
			for _, tab := range entry.Tabs {
				sheet.Tabs = append(sheet.Tabs, TabResponse{Period: tab.Period, Title: tab.Title})
			}
			item.Sheets = append(item.Sheets, sheet)
		}
		out.Items = append(out.Items, item)
	}
	return out
}

type TabNamePreviewRequest struct {
	Pattern  string `json:"pattern" example:"Pagamentos · {mes} {ano}"`
	TimeZone string `json:"timeZone,omitempty" example:"America/Sao_Paulo"`
}

type TabNamePreviewResponse struct {
	Title  string `json:"title" example:"Pagamentos · novembro 2026"`
	Period string `json:"period" example:"2026-11"`
}

type PresetColumnResponse struct {
	Key         string            `json:"key" example:"baixa"`
	Type        string            `json:"type" example:"choice"`
	Source      string            `json:"source" example:"input"`
	Options     []string          `json:"options,omitempty"`
	Colors      map[string]string `json:"colors,omitempty" swaggertype:"object,string"`
	Required    bool              `json:"required"`
	Instruction bool              `json:"instruction"`
	LinkRef     string            `json:"linkRef,omitempty" example:"negociacoes"`
}

type PresetSheetResponse struct {
	Ref       string                 `json:"ref" example:"pagamentos"`
	TabPeriod string                 `json:"tabPeriod" enums:"none,month"`
	Style     string                 `json:"style" enums:"plain,graphite,blue,green,purple"`
	Columns   []PresetColumnResponse `json:"columns"`
}

type PresetResponse struct {
	Key    string                `json:"key" example:"cobranca"`
	Icon   string                `json:"icon" example:"Receipt"`
	Sheets []PresetSheetResponse `json:"sheets"`
}

type PresetListResponse struct {
	Items []PresetResponse `json:"items"`
}

func presets() PresetListResponse {
	out := PresetListResponse{}
	for _, preset := range googlesheet.Presets() {
		item := PresetResponse{Key: preset.Key, Icon: preset.Icon}
		for _, sheet := range preset.Sheets {
			entry := PresetSheetResponse{Ref: sheet.Ref, TabPeriod: string(sheet.TabPeriod), Style: string(sheet.Style)}
			for _, col := range sheet.Columns {
				var colors map[string]string
				if len(col.Colors) > 0 {
					colors = map[string]string{}
					for option, color := range col.Colors {
						colors[option] = string(color)
					}
				}
				entry.Columns = append(entry.Columns, PresetColumnResponse{
					Key: col.Key, Type: string(col.Type), Source: string(col.Source), Options: col.Options, Colors: colors,
					Required: col.Required, Instruction: col.Instruction, LinkRef: col.LinkRef,
				})
			}
			item.Sheets = append(item.Sheets, entry)
		}
		out.Items = append(out.Items, item)
	}
	return out
}
