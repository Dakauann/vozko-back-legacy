package googlesheet

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"vozko/domain/shared"
)

const (
	MaxColumnKeyLength  = 40
	MaxHeaderRunes      = 200
	MaxOptionRunes      = 200
	MaxInstructionRunes = 500
	RefHeader           = "ref"
)

type ColumnType string

const (
	TypeText     ColumnType = "text"
	TypeLongText ColumnType = "long_text"
	TypeNumber   ColumnType = "number"
	TypeMoney    ColumnType = "money"
	TypeDate     ColumnType = "date"
	TypeDateTime ColumnType = "datetime"
	TypeBoolean  ColumnType = "boolean"
	TypeChoice   ColumnType = "choice"
	TypeEmail    ColumnType = "email"
	TypePhone    ColumnType = "phone"
	TypeURL      ColumnType = "url"
	TypeLink     ColumnType = "link"
)

func ColumnTypes() []ColumnType {
	return []ColumnType{TypeText, TypeLongText, TypeNumber, TypeMoney, TypeDate, TypeDateTime, TypeBoolean, TypeChoice, TypeEmail, TypePhone, TypeURL}
}

func (t ColumnType) Valid() bool {
	if t == TypeLink {
		return true
	}
	for _, known := range ColumnTypes() {
		if t == known {
			return true
		}
	}
	return false
}

type Source string

const (
	SourceInput            Source = "input"
	SourceWrittenAt        Source = "written_at"
	SourceLeadName         Source = "lead_name"
	SourceLeadPhone        Source = "lead_phone"
	SourceLeadEmail        Source = "lead_email"
	SourceConversationLink Source = "conversation_link"
	SourceOrigin           Source = "origin"
	SourceMessageSentAt    Source = "message_sent_at"
	SourceMessageSender    Source = "message_sender"
	SourceMessageDirection Source = "message_direction"
	SourceMessageText      Source = "message_text"
	SourceMessageMediaURL  Source = "message_media_url"
	SourceExportLink       Source = "export_link"
)

var systemSourceTypes = map[Source]ColumnType{
	SourceWrittenAt:        TypeDateTime,
	SourceLeadName:         TypeText,
	SourceLeadPhone:        TypePhone,
	SourceLeadEmail:        TypeEmail,
	SourceConversationLink: TypeURL,
	SourceOrigin:           TypeText,
	SourceMessageSentAt:    TypeDateTime,
	SourceMessageSender:    TypeText,
	SourceMessageDirection: TypeText,
	SourceMessageText:      TypeLongText,
	SourceMessageMediaURL:  TypeURL,
	SourceExportLink:       TypeLink,
}

func SystemSources() []Source {
	return []Source{
		SourceWrittenAt, SourceLeadName, SourceLeadPhone, SourceLeadEmail, SourceConversationLink, SourceOrigin,
		SourceMessageSentAt, SourceMessageSender, SourceMessageDirection, SourceMessageText, SourceMessageMediaURL,
		SourceExportLink,
	}
}

func (s Source) Valid() bool {
	if s == SourceInput {
		return true
	}
	_, ok := systemSourceTypes[s]
	return ok
}

func (s Source) IsSystem() bool { return s != SourceInput }

func PerMessageSources() []Source {
	return []Source{SourceMessageSentAt, SourceMessageSender, SourceMessageDirection, SourceMessageText, SourceMessageMediaURL}
}

func (s Source) PerMessage() bool {
	return slices.Contains(PerMessageSources(), s)
}

func (s Source) ColumnType() (ColumnType, bool) {
	t, ok := systemSourceTypes[s]
	return t, ok
}

type Column struct {
	Key         string                 `json:"key"`
	Header      string                 `json:"header"`
	Type        ColumnType             `json:"type"`
	Options     []string               `json:"options,omitempty"`
	Colors      map[string]OptionColor `json:"colors,omitempty"`
	Required    bool                   `json:"required"`
	Instruction string                 `json:"instruction,omitempty"`
	Source      Source                 `json:"source"`
	LinkSheetID string                 `json:"link_sheet_id,omitempty"`
	LinkLabel   string                 `json:"link_label,omitempty"`
	Archived    bool                   `json:"archived"`
}

func (c Column) TakesInput() bool { return !c.Archived && c.Source == SourceInput }

type ColumnInput struct {
	Header       string
	Type         ColumnType
	Options      []string
	Colors       map[string]OptionColor
	Required     bool
	Instruction  string
	Source       Source
	LinkSheetID  string
	LinkLabel    string
	PreferredKey string
}

func newColumn(in ColumnInput, taken []string) (Column, error) {
	col := Column{
		Header:      strings.TrimSpace(in.Header),
		Type:        in.Type,
		Options:     trimmedOptions(in.Options),
		Colors:      trimmedColors(in.Colors),
		Required:    in.Required,
		Instruction: strings.TrimSpace(in.Instruction),
		Source:      in.Source,
		LinkSheetID: strings.TrimSpace(in.LinkSheetID),
		LinkLabel:   strings.TrimSpace(in.LinkLabel),
	}
	if col.Source == "" {
		col.Source = SourceInput
	}
	if col.Source == SourceExportLink && col.LinkLabel == "" {
		col.LinkLabel = DefaultLinkLabel
	}
	if err := col.validate(); err != nil {
		return Column{}, err
	}
	seed := strings.TrimSpace(in.PreferredKey)
	if seed == "" {
		seed = col.Header
	}
	col.Key = KeyFromHeader(seed, taken)
	return col, nil
}

func (c Column) validate() error {
	switch {
	case c.Header == "":
		return columnRefusal(CodeHeaderRequired, c)
	case utf8.RuneCountInString(c.Header) > MaxHeaderRunes:
		return columnRefusal(CodeHeaderTooLong, c).limit(MaxHeaderRunes)
	case !c.Source.Valid():
		return columnRefusal(CodeSourceUnknown, c).detail("%q", c.Source)
	case !c.Type.Valid() || (c.Type == TypeLink && c.Source != SourceExportLink):
		return columnRefusal(CodeTypeUnknown, c).detail("%q", c.Type)
	case utf8.RuneCountInString(c.Instruction) > MaxInstructionRunes:
		return columnRefusal(CodeInstructionTooLong, c).limit(MaxInstructionRunes)
	}
	if want, system := c.Source.ColumnType(); system && want != c.Type {
		return columnRefusal(CodeSourceTypeMismatch, c).detail("%s exige %s", c.Source, want)
	}
	if c.Required && c.Source.PerMessage() {
		return columnRefusal(CodePerMessageRequired, c)
	}
	if err := c.validateLink(); err != nil {
		return err
	}
	if err := c.validateOptions(); err != nil {
		return err
	}
	return c.validateColors()
}

func (c Column) validateLink() error {
	if c.Source != SourceExportLink {
		if c.LinkSheetID != "" || c.LinkLabel != "" {
			return columnRefusal(CodeLinkFieldsNotLink, c)
		}
		return nil
	}
	switch {
	case c.Required:
		return columnRefusal(CodeLinkRequired, c)
	case !uuidPattern.MatchString(c.LinkSheetID):
		return columnRefusal(CodeLinkTargetRequired, c)
	case !validLinkLabel(c.LinkLabel):
		return columnRefusal(CodeLinkLabelInvalid, c).limit(MaxLinkLabelRunes)
	}
	return nil
}

func (c Column) validateOptions() error {
	if c.Type != TypeChoice {
		if len(c.Options) > 0 {
			return columnRefusal(CodeOptionsNotChoice, c)
		}
		return nil
	}
	if len(c.Options) == 0 {
		return columnRefusal(CodeOptionsRequired, c)
	}
	seen := map[string]bool{}
	for _, option := range c.Options {
		folded := shared.FoldForMatch(option)
		switch {
		case option == "":
			return columnRefusal(CodeOptionBlank, c)
		case utf8.RuneCountInString(option) > MaxOptionRunes:
			return columnRefusal(CodeOptionTooLong, c).limit(MaxOptionRunes)
		case seen[folded]:
			return columnRefusal(CodeOptionRepeated, c).option(option)
		}
		seen[folded] = true
	}
	return nil
}

func (c Column) validateColors() error {
	if len(c.Colors) == 0 {
		return nil
	}
	if c.Type != TypeChoice {
		return columnRefusal(CodeColorsNotChoice, c)
	}
	for option, color := range c.Colors {
		if !color.Valid() {
			return columnRefusal(CodeColorUnknown, c).option(option).detail("%q", color)
		}
		known := false
		for _, existing := range c.Options {
			known = known || existing == option
		}
		if !known {
			return columnRefusal(CodeColorNotOption, c).option(option)
		}
	}
	return nil
}

func trimmedOptions(options []string) []string {
	if len(options) == 0 {
		return nil
	}
	out := make([]string, len(options))
	for i, option := range options {
		out[i] = strings.TrimSpace(option)
	}
	return out
}

func trimmedColors(colors map[string]OptionColor) map[string]OptionColor {
	if len(colors) == 0 {
		return nil
	}
	out := make(map[string]OptionColor, len(colors))
	for option, color := range colors {
		out[strings.TrimSpace(option)] = color
	}
	return out
}

var ordinalFolds = strings.NewReplacer("º", "o", "ª", "a")

func KeyFromHeader(header string, taken []string) string {
	folded := ordinalFolds.Replace(shared.FoldForMatch(header))
	var b strings.Builder
	underscore := false
	for _, r := range folded {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			underscore = false
			continue
		}
		if !underscore && b.Len() > 0 {
			b.WriteByte('_')
			underscore = true
		}
	}
	base := strings.Trim(b.String(), "_")
	switch {
	case base == "":
		base = "coluna"
	case base[0] >= '0' && base[0] <= '9':
		base = "c_" + base
	}
	base = clipKey(base, MaxColumnKeyLength)
	used := map[string]bool{}
	for _, key := range taken {
		used[key] = true
	}
	key := base
	for n := 2; used[key]; n++ {
		suffix := "_" + strconv.Itoa(n)
		key = clipKey(base, MaxColumnKeyLength-len(suffix)) + suffix
	}
	return key
}

func clipKey(key string, max int) string {
	if len(key) <= max {
		return key
	}
	return strings.TrimRight(key[:max], "_")
}
