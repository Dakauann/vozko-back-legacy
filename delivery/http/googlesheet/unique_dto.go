package googlesheet

import "vozko/domain/googlesheet"

type UniqueCreateRequest struct {
	Mode          string `json:"mode" example:"columns" enums:"none,conversation,columns"`
	ColumnIndexes []int  `json:"columnIndexes,omitempty" example:"2"`
	OnRepeat      string `json:"onRepeat,omitempty" example:"skip" enums:"skip"`
}

func (r *UniqueCreateRequest) input() googlesheet.UniqueInput {
	if r == nil {
		return googlesheet.UniqueInput{}
	}
	return googlesheet.UniqueInput{Mode: googlesheet.UniqueMode(r.Mode), Columns: r.ColumnIndexes, OnRepeat: googlesheet.RepeatAction(r.OnRepeat)}
}

type UniqueRuleRequest struct {
	Mode     string   `json:"mode" example:"columns" enums:"none,conversation,columns"`
	Columns  []string `json:"columns,omitempty" example:"matricula"`
	OnRepeat string   `json:"onRepeat,omitempty" example:"skip" enums:"skip"`
}

func (r *UniqueRuleRequest) rule() *googlesheet.UniqueRule {
	if r == nil {
		return nil
	}
	return &googlesheet.UniqueRule{Mode: googlesheet.UniqueMode(r.Mode), Columns: r.Columns, OnRepeat: googlesheet.RepeatAction(r.OnRepeat)}
}

type UniqueRuleResponse struct {
	Mode     string   `json:"mode" example:"conversation" enums:"none,conversation,columns"`
	Columns  []string `json:"columns"`
	OnRepeat string   `json:"onRepeat" example:"skip" enums:"skip"`
}

func toUniqueResponse(rule googlesheet.UniqueRule) UniqueRuleResponse {
	rule = rule.Normalized()
	return UniqueRuleResponse{Mode: string(rule.Mode), Columns: append([]string{}, rule.Columns...), OnRepeat: string(rule.OnRepeat)}
}

type PresetUniqueResponse struct {
	Mode    string   `json:"mode" example:"conversation" enums:"none,conversation,columns"`
	Columns []string `json:"columns"`
}

func toPresetUnique(rule googlesheet.UniqueRule) PresetUniqueResponse {
	rule = rule.Normalized()
	return PresetUniqueResponse{Mode: string(rule.Mode), Columns: append([]string{}, rule.Columns...)}
}

type repeatRefusal struct {
	Sheet  *int   `json:"sheet,omitempty"`
	Header string `json:"header,omitempty" example:"Matrícula"`
	RowID  string `json:"rowId" example:"8f8e5f7a-3c1d-4a77-9a55-0d6f0f3b2a11"`
}

func uniqueCatalog(out *CatalogResponse) {
	for _, mode := range googlesheet.UniqueModes() {
		out.UniqueModes = append(out.UniqueModes, string(mode))
	}
	for _, action := range googlesheet.RepeatActions() {
		out.RepeatActions = append(out.RepeatActions, string(action))
	}
	for _, source := range googlesheet.UniqueSources() {
		out.UniqueSources = append(out.UniqueSources, string(source))
	}
}
