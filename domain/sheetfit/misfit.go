package sheetfit

import "fmt"

const (
	CodeFileNotFound     = "sheet_fit_file_not_found"
	CodeFileOtherAccount = "sheet_fit_file_other_account"
	CodeModelFile        = "sheet_fit_model_file"
	CodeModelMissing     = "sheet_fit_model_missing"
	CodeUsesRequired     = "sheet_fit_uses_required"
	CodeUseInvalid       = "sheet_fit_use_invalid"
	CodeTabMissing       = "sheet_fit_tab_missing"
	CodeTabAmbiguous     = "sheet_fit_tab_ambiguous"
	CodeTabShared        = "sheet_fit_tab_shared"
	CodeColumnMissing    = "sheet_fit_column_missing"
	CodeColumnArchived   = "sheet_fit_column_archived"
	CodeColumnSystem     = "sheet_fit_column_system"
	CodeRequiredUnmapped = "sheet_fit_required_unmapped"
	CodeColumnType       = "sheet_fit_column_type"
	CodeColumnOptions    = "sheet_fit_column_options"
	CodeNoMessageColumns = "sheet_fit_no_message_columns"
	CodeLinkMismatch     = "sheet_fit_link_mismatch"
)

var messages = []struct {
	code    string
	message string
}{
	{CodeFileNotFound, "O arquivo não está nas planilhas deste workspace"},
	{CodeFileOtherAccount, "O arquivo foi criado por outra conta Google e não recebe gravações"},
	{CodeModelFile, "Este é o arquivo modelo; escolha o arquivo que recebe as linhas"},
	{CodeModelMissing, "Uma aba modelo do fluxo foi removida"},
	{CodeUsesRequired, "Informe onde o fluxo grava para conferir o arquivo"},
	{CodeUseInvalid, "Um uso da planilha está incompleto"},
	{CodeTabMissing, "Nenhuma aba do arquivo serve para uma aba modelo"},
	{CodeTabAmbiguous, "Mais de uma aba do arquivo serve para uma aba modelo"},
	{CodeTabShared, "Duas abas modelo precisam de abas diferentes no arquivo"},
	{CodeColumnMissing, "A aba não tem uma coluna que o fluxo preenche"},
	{CodeColumnArchived, "Uma coluna que o fluxo preenche está arquivada"},
	{CodeColumnSystem, "Uma coluna que o fluxo preenche é preenchida pelo sistema nesta aba"},
	{CodeRequiredUnmapped, "A aba tem uma coluna obrigatória que o fluxo não preenche"},
	{CodeColumnType, "Uma coluna tem outro tipo nesta aba"},
	{CodeColumnOptions, "Uma coluna de escolha não tem todas as opções do modelo"},
	{CodeNoMessageColumns, "A aba não tem colunas de mensagem para a exportação"},
	{CodeLinkMismatch, "Um link da aba precisa apontar para a aba de negociações do mesmo arquivo"},
}

func Codes() []string {
	out := make([]string, len(messages))
	for i, entry := range messages {
		out[i] = entry.code
	}
	return out
}

func Message(code string) string {
	for _, entry := range messages {
		if entry.code == code {
			return entry.message
		}
	}
	return ""
}

type Tab struct {
	SheetID string
	Title   string
}

type Problem struct {
	Code     string
	SheetID  string
	Tab      string
	Column   string
	Option   string
	ModelTab string
}

type Misfit struct {
	Code         string
	ModelSheetID string
	ModelTab     string
	Problems     []Problem
	Candidates   []Tab
}

func (m *Misfit) Error() string {
	if m.ModelTab == "" {
		return Message(m.Code)
	}
	return fmt.Sprintf("%s: %s", Message(m.Code), m.ModelTab)
}

func refuse(code string) *Misfit { return &Misfit{Code: code} }
