package googlesheet

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"vozko/domain/shared"
)

const (
	DefaultTimeZone = "America/Sao_Paulo"
	DefaultLocale   = "pt_BR"
	MaxTitleRunes   = 200
	MaxTabRunes     = 100
)

var localePattern = regexp.MustCompile(`^[a-z]{2}_[A-Z]{2}$`)

type Status string

const (
	StatusActive         Status = "active"
	StatusNeedsAttention Status = "needs_attention"
)

type AttentionReason string

const (
	ReasonHeaderMismatch    AttentionReason = "header_mismatch"
	ReasonAccessLost        AttentionReason = "access_lost"
	ReasonSpreadsheetGone   AttentionReason = "spreadsheet_missing"
	ReasonSheetFull         AttentionReason = "sheet_full"
	ReasonGoogleUnavailable AttentionReason = "google_unavailable"
	ReasonAccountChanged    AttentionReason = "account_changed"
)

type Sheet struct {
	ID            string
	WorkspaceID   string
	AccountID     string
	SpreadsheetID string
	TabID         int64
	TabName       string
	TabPeriod     TabPeriod
	TabPattern    string
	TabPeriodKey  string
	Style         StylePreset
	Title         string
	Description   string
	TimeZone      string
	Locale        string
	Columns       []Column
	Status        Status
	Reason        AttentionReason
	ReasonDetail  string
	Version       int
	CreatedBy     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	LastWrittenAt *time.Time
}

type NewSheetInput struct {
	WorkspaceID string
	AccountID   string
	Title       string
	TabName     string
	TabPeriod   TabPeriod
	TabPattern  string
	Style       StylePreset
	Description string
	TimeZone    string
	Locale      string
	CreatedBy   string
	Columns     []ColumnInput
}

func NewSheet(in NewSheetInput) (*Sheet, error) {
	return buildSheet(in, true)
}

func CheckDefinition(in NewSheetInput) error {
	_, err := buildSheet(in, false)
	return err
}

func buildSheet(in NewSheetInput, owned bool) (*Sheet, error) {
	s := &Sheet{
		WorkspaceID: strings.TrimSpace(in.WorkspaceID),
		AccountID:   strings.TrimSpace(in.AccountID),
		Title:       strings.TrimSpace(in.Title),
		TabName:     strings.TrimSpace(in.TabName),
		TabPeriod:   in.TabPeriod,
		TabPattern:  strings.TrimSpace(in.TabPattern),
		Style:       in.Style,
		Description: strings.TrimSpace(in.Description),
		TimeZone:    strings.TrimSpace(in.TimeZone),
		Locale:      strings.TrimSpace(in.Locale),
		CreatedBy:   in.CreatedBy,
		Status:      StatusActive,
		Version:     1,
	}
	if s.TabName == "" {
		s.TabName = s.Title
	}
	if s.TimeZone == "" {
		s.TimeZone = DefaultTimeZone
	}
	if s.Locale == "" {
		s.Locale = DefaultLocale
	}
	if s.TabPeriod == "" {
		s.TabPeriod = PeriodNone
	}
	if s.Style == "" {
		s.Style = DefaultStyle
	}
	if s.TabPeriod == PeriodMonth && s.TabPattern == "" && s.Title != "" {
		s.TabPattern = DefaultTabPattern(s.Title)
	}
	if owned {
		if err := s.validateOwner(); err != nil {
			return nil, err
		}
	}
	if err := s.validateFrame(); err != nil {
		return nil, err
	}
	for i, input := range in.Columns {
		col, err := newColumn(input, s.keys())
		if err != nil {
			return nil, atColumn(err, FieldColumns, i)
		}
		s.Columns = append(s.Columns, col)
	}
	if err := s.validateColumns(FieldColumns, 0); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Sheet) validateOwner() error {
	switch {
	case s.WorkspaceID == "":
		return refuse(FieldWorkspace, CodeWorkspaceRequired)
	case s.AccountID == "":
		return refuse(FieldAccount, CodeAccountRequired)
	}
	return nil
}

func (s *Sheet) validateFrame() error {
	switch {
	case s.Title == "":
		return refuse(FieldTitle, CodeTitleRequired)
	case utf8.RuneCountInString(s.Title) > MaxTitleRunes:
		return refuse(FieldTitle, CodeTitleTooLong).limit(MaxTitleRunes)
	case utf8.RuneCountInString(s.TabName) > MaxTabRunes:
		return refuse(FieldTabName, CodeTabNameTooLong).limit(MaxTabRunes)
	case strings.ContainsAny(s.TabName, tabForbidden):
		return refuse(FieldTabName, CodeTabNameForbidden).detail("%q", s.TabName)
	case !localePattern.MatchString(s.Locale):
		return refuse(FieldLocale, CodeLocaleInvalid).detail("%q", s.Locale)
	case !s.Style.Valid():
		return refuse(FieldStyle, CodeStyleUnknown).detail("%q", s.Style)
	case !s.TabPeriod.Valid():
		return refuse(FieldTabPeriod, CodeTabPeriodUnknown).detail("%q", s.TabPeriod)
	}
	if _, err := time.LoadLocation(s.TimeZone); err != nil {
		return refuse(FieldTimeZone, CodeTimeZoneUnknown).detail("%q", s.TimeZone)
	}
	return s.validatePattern()
}

func (s *Sheet) validatePattern() error {
	if !s.Periodic() {
		if s.TabPattern != "" {
			return refuse(FieldTabPattern, CodeTabPatternNotPeriodic)
		}
		return nil
	}
	return checkTabPattern(s.TabPattern)
}

func (s *Sheet) validateColumns(field string, firstNew int) error {
	if len(s.Columns) == 0 {
		return refuse(field, CodeNoColumns)
	}
	headers := map[string]bool{}
	for i, col := range s.Columns {
		folded := shared.FoldForMatch(col.Header)
		if headers[folded] {
			def := columnRefusal(CodeHeaderRepeated, col)
			def.Field, def.Column = field, i-firstNew
			if i < firstNew {
				def.Field, def.Column = FieldUpdate, -1
			}
			return def
		}
		headers[folded] = true
	}
	return nil
}

func (s *Sheet) keys() []string {
	keys := make([]string, len(s.Columns))
	for i, col := range s.Columns {
		keys[i] = col.Key
	}
	return keys
}

func (s *Sheet) Column(key string) (Column, bool) {
	for _, col := range s.Columns {
		if col.Key == key {
			return col, true
		}
	}
	return Column{}, false
}

func (s *Sheet) InputColumns() []Column {
	var out []Column
	for _, col := range s.Columns {
		if col.TakesInput() {
			out = append(out, col)
		}
	}
	return out
}

func (s *Sheet) TakesInput() bool { return len(s.InputColumns()) > 0 }

func (s *Sheet) CheckTakesInput() error {
	if !s.TakesInput() {
		return ErrNoInputColumns
	}
	return nil
}

func (s *Sheet) HeaderRow() []string {
	header := make([]string, 0, len(s.Columns)+1)
	header = append(header, RefHeader)
	for _, col := range s.Columns {
		header = append(header, col.Header)
	}
	return header
}

func (s *Sheet) Location() *time.Location {
	loc, err := time.LoadLocation(s.TimeZone)
	if err != nil {
		return time.UTC
	}
	return loc
}

func (s *Sheet) URL() string {
	return fmt.Sprintf("%s#gid=%d", SpreadsheetURL(s.SpreadsheetID), s.TabID)
}

func (s *Sheet) Writable() error {
	if s.Status != StatusActive {
		return fmt.Errorf("%w: %s", ErrSheetNotActive, s.Reason)
	}
	return nil
}

func (s *Sheet) NeedsAttention(reason AttentionReason, detail string) {
	s.Status, s.Reason, s.ReasonDetail = StatusNeedsAttention, reason, detail
}

func (s *Sheet) Activate() {
	s.Status, s.Reason, s.ReasonDetail = StatusActive, "", ""
}

type ColumnUpdate struct {
	Key         string
	Header      *string
	Instruction *string
	Required    *bool
	Archived    *bool
	Options     []string
	Colors      map[string]OptionColor
	LinkLabel   *string
}

type ColumnsEdit struct {
	Add    []ColumnInput
	Update []ColumnUpdate
}

func (e ColumnsEdit) Empty() bool { return len(e.Add) == 0 && len(e.Update) == 0 }

func (s *Sheet) EditColumns(edit ColumnsEdit) error {
	if edit.Empty() {
		return refuse(FieldEdit, CodeEditEmpty)
	}
	columns := slices.Clone(s.Columns)
	for i := range columns {
		columns[i].Options = slices.Clone(columns[i].Options)
		columns[i].Colors = maps.Clone(columns[i].Colors)
	}
	draft := *s
	draft.Columns = columns
	for _, update := range edit.Update {
		if err := draft.applyUpdate(update); err != nil {
			return err
		}
	}
	existing := len(draft.Columns)
	for i, input := range edit.Add {
		col, err := newColumn(input, draft.keys())
		if err != nil {
			return atColumn(err, FieldAdd, i)
		}
		draft.Columns = append(draft.Columns, col)
	}
	if err := draft.validateColumns(FieldAdd, existing); err != nil {
		return err
	}
	s.Columns = draft.Columns
	s.Version++
	return nil
}

func (s *Sheet) applyUpdate(update ColumnUpdate) error {
	index := -1
	for i, col := range s.Columns {
		if col.Key == update.Key {
			index = i
		}
	}
	if index < 0 {
		return fmt.Errorf("%w: %q", ErrUnknownColumn, update.Key)
	}
	col := s.Columns[index]
	if update.Header != nil {
		col.Header = strings.TrimSpace(*update.Header)
	}
	if update.Instruction != nil {
		col.Instruction = strings.TrimSpace(*update.Instruction)
	}
	if update.Required != nil {
		col.Required = *update.Required
	}
	if update.Archived != nil {
		col.Archived = *update.Archived
	}
	if update.LinkLabel != nil {
		col.LinkLabel = strings.TrimSpace(*update.LinkLabel)
	}
	if update.Options != nil {
		if col.Type != TypeChoice {
			return updateRefusal(columnRefusal(CodeOptionsNotChoice, col))
		}
		next := trimmedOptions(update.Options)
		for _, existing := range col.Options {
			if !slices.Contains(next, existing) {
				return updateRefusal(columnRefusal(CodeOptionRemoved, col).option(existing))
			}
		}
		col.Options = next
	}
	if update.Colors != nil {
		col.Colors = trimmedColors(update.Colors)
	}
	if err := col.validate(); err != nil {
		return updateRefusal(err)
	}
	s.Columns[index] = col
	return nil
}

func updateRefusal(err error) error { return atColumn(err, FieldUpdate, -1) }

func (s *Sheet) Rename(title, description *string) error {
	draft := *s
	if title != nil {
		draft.Title = strings.TrimSpace(*title)
	}
	if description != nil {
		draft.Description = strings.TrimSpace(*description)
	}
	if err := draft.validateFrame(); err != nil {
		return err
	}
	s.Title, s.Description = draft.Title, draft.Description
	s.Version++
	return nil
}

func (s *Sheet) ChangeLayout(style *StylePreset, pattern *string) error {
	if style == nil && pattern == nil {
		return refuse(FieldEdit, CodeEditEmpty)
	}
	draft := *s
	if style != nil {
		draft.Style = *style
	}
	if pattern != nil {
		draft.TabPattern = strings.TrimSpace(*pattern)
	}
	if err := draft.validateFrame(); err != nil {
		return err
	}
	s.Style, s.TabPattern = draft.Style, draft.TabPattern
	s.Version++
	return nil
}

func HeaderDifference(expected []string, rows [][]any) string {
	var actual []any
	if len(rows) > 0 {
		actual = rows[0]
	}
	for i, want := range expected {
		got := ""
		if i < len(actual) {
			got = strings.TrimSpace(fmt.Sprint(actual[i]))
		}
		if got != want {
			return fmt.Sprintf("coluna %s: esperado %q, encontrado %q", columnLetter(i), want, got)
		}
	}
	return ""
}

func HeaderIsBlank(rows [][]any) bool {
	for _, row := range rows {
		for _, cell := range row {
			if strings.TrimSpace(fmt.Sprint(cell)) != "" {
				return false
			}
		}
	}
	return true
}

func columnLetter(index int) string {
	letters := ""
	for n := index + 1; n > 0; n = (n - 1) / 26 {
		letters = string(rune('A'+(n-1)%26)) + letters
	}
	return letters
}

const DashboardPath = "/dashboard/integrations/google-sheets"

func (s *Sheet) DashboardLink() string { return DashboardPath + "/" + s.ID }

func (s *Sheet) CheckMapping(mapping map[string]string) error {
	for key := range mapping {
		col, ok := s.Column(key)
		switch {
		case !ok:
			return &ValueError{Column: key, Header: key, Reason: ErrUnknownColumn}
		case col.Archived:
			return &ValueError{Column: key, Header: col.Header, Reason: ErrColumnArchived}
		case col.Source.IsSystem():
			return &ValueError{Column: key, Header: col.Header, Reason: ErrSystemColumn}
		}
	}
	for _, col := range s.InputColumns() {
		if col.Required && strings.TrimSpace(mapping[col.Key]) == "" {
			return &ValueError{Column: col.Key, Header: col.Header, Reason: ErrRequiredColumnMissing}
		}
	}
	return nil
}
