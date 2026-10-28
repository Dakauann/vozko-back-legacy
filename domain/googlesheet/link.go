package googlesheet

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	DefaultLinkLabel  = "Ver mensagens"
	MaxLinkLabelRunes = 60
	linkKeyPrefix     = "link:"
)

var (
	uuidPattern   = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	periodPattern = regexp.MustCompile(`^(\d{4}-\d{2})?$`)
)

type Formula string

type ExportAnchor struct {
	RowID  string
	Period string
}

type LinkRef struct {
	SheetID string
	Period  string
	RowID   string
}

func (r LinkRef) String() string { return r.SheetID + "/" + r.Period + "/" + r.RowID }

func ParseLinkRef(text string) (LinkRef, error) {
	parts := strings.Split(text, "/")
	if len(parts) != 3 {
		return LinkRef{}, fmt.Errorf("googlesheet: link reference %q is not sheet/period/row", text)
	}
	ref := LinkRef{SheetID: parts[0], Period: parts[1], RowID: parts[2]}
	if !uuidPattern.MatchString(ref.SheetID) || !uuidPattern.MatchString(ref.RowID) || !periodPattern.MatchString(ref.Period) {
		return LinkRef{}, fmt.Errorf("googlesheet: link reference %q is malformed", text)
	}
	return ref, nil
}

type TabRef struct {
	ID    int64
	Title string
}

type LinkTabs map[string]TabRef

type Links struct {
	Tabs   LinkTabs
	Locale string
}

func LinkTabKey(sheetID, period string) string { return sheetID + "/" + period }

func linkSource(sheetID string) Source { return Source(linkKeyPrefix + sheetID) }

func (v SystemValues) WithLink(sheetID string, anchor ExportAnchor) SystemValues {
	out := make(SystemValues, len(v)+1)
	for source, value := range v {
		out[source] = value
	}
	out[linkSource(sheetID)] = anchor.Period + "/" + anchor.RowID
	return out
}

func (v SystemValues) Link(sheetID string) (ExportAnchor, bool) {
	value, ok := v[linkSource(sheetID)]
	if !ok {
		return ExportAnchor{}, false
	}
	period, row, found := strings.Cut(value, "/")
	if !found || row == "" {
		return ExportAnchor{}, false
	}
	return ExportAnchor{RowID: row, Period: period}, true
}

func validLinkLabel(label string) bool {
	if label == "" || utf8.RuneCountInString(label) > MaxLinkLabelRunes {
		return false
	}
	for _, r := range label {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func LinkFormula(tab TabRef, rowID, label string, sep Separator) (Formula, error) {
	switch {
	case !sep.valid():
		return "", fmt.Errorf("googlesheet: link separator %q is not a formula argument separator", sep)
	case !uuidPattern.MatchString(rowID):
		return "", fmt.Errorf("googlesheet: link row %q is not a row id", rowID)
	case tab.ID < 0 || strings.TrimSpace(tab.Title) == "":
		return "", fmt.Errorf("googlesheet: link tab %+v is incomplete", tab)
	case !validLinkLabel(label):
		return "", fmt.Errorf("googlesheet: link label %q is invalid", label)
	}
	title := strings.ReplaceAll(tab.Title, "'", "''")
	text := strings.ReplaceAll(label, `"`, `""`)
	return Formula(`=HYPERLINK("#gid=` + strconv.FormatInt(tab.ID, 10) + `&range=B"&MATCH("` + rowID + `"` + string(sep) + `'` + title + `'!$A:$A` + string(sep) + `0)` + string(sep) + `"` + text + `")`), nil
}

func CheckLinkTarget(sheet *Sheet, col Column, target *Sheet) error {
	switch {
	case target == nil || target.ID == "" || target.ID != col.LinkSheetID:
		return fmt.Errorf("%w: coluna %q: planilha de destino não encontrada", ErrLinkTargetInvalid, col.Header)
	case target.ID == sheet.ID:
		return fmt.Errorf("%w: coluna %q: a planilha não pode apontar para ela mesma", ErrLinkTargetInvalid, col.Header)
	case target.WorkspaceID != sheet.WorkspaceID || target.SpreadsheetID != sheet.SpreadsheetID:
		return fmt.Errorf("%w: coluna %q: %q precisa ser uma aba do mesmo arquivo", ErrLinkTargetInvalid, col.Header, target.Title)
	case !target.HasMessageColumns():
		return fmt.Errorf("%w: coluna %q: %q não tem colunas preenchidas por mensagem", ErrLinkTargetInvalid, col.Header, target.Title)
	}
	return nil
}

func (s *Sheet) LinkColumns() []Column {
	var out []Column
	for _, col := range s.Columns {
		if !col.Archived && col.Source == SourceExportLink {
			out = append(out, col)
		}
	}
	return out
}

func (s *Sheet) LinkRefs(cells Cells) []LinkRef {
	var out []LinkRef
	for _, col := range s.LinkColumns() {
		text, ok := cells[col.Key].(string)
		if !ok {
			continue
		}
		if ref, err := ParseLinkRef(text); err == nil {
			out = append(out, ref)
		}
	}
	return out
}

func (s *Sheet) linkCell(col Column, value any, links Links) (any, error) {
	text, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("stored %T is not a link", value)
	}
	ref, err := ParseLinkRef(text)
	if err != nil {
		return nil, err
	}
	tab, ok := links.Tabs[LinkTabKey(ref.SheetID, ref.Period)]
	if !ok {
		return nil, ErrLinkTargetPending
	}
	sep, err := FormulaSeparator(links.Locale)
	if err != nil {
		return nil, err
	}
	return LinkFormula(tab, ref.RowID, col.LinkLabel, sep)
}

func (s *Sheet) DisplayCells(cells Cells) Cells {
	out := make(Cells, len(cells))
	for key, value := range cells {
		out[key] = value
	}
	for _, col := range s.Columns {
		if col.Source == SourceExportLink {
			if _, ok := out[col.Key]; ok {
				out[col.Key] = col.LinkLabel
			}
		}
	}
	return out
}
