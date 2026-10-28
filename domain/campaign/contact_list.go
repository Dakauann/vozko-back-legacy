package campaign

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	"vozko/domain/agent"
	"vozko/domain/customfield"
	"vozko/domain/emailaddress"
	"vozko/domain/shared"
	"vozko/domain/sheet"
)

type ContactKind string

const (
	ContactPhone ContactKind = "phone"
	ContactEmail ContactKind = "email"
)

func (k ContactKind) Valid() bool {
	return k == ContactPhone || k == ContactEmail
}

type ListIssueReason string

const (
	ListInvalidNumber ListIssueReason = "invalid_number"
	ListMissingNumber ListIssueReason = "missing_number"
	ListInvalidEmail  ListIssueReason = "invalid_email"
	ListMissingEmail  ListIssueReason = "missing_email"
	ListDuplicate     ListIssueReason = "duplicate"
)

const MaxListColumns = 200

var (
	ErrListEmpty         = errors.New("campaign list: the file has no contact line")
	ErrListContactColumn = errors.New("campaign list: the header has no phone or email column")
	ErrListTooWide       = fmt.Errorf("campaign list: a line has more than %d columns", MaxListColumns)
	ErrListTooLarge      = fmt.Errorf("campaign list: one campaign takes at most %d contacts", MaxEntries)
	ErrListContactKind   = errors.New("campaign list: the contact must be a phone or an email")
)

type ListIssue struct {
	Line        int             `json:"line"`
	Reason      ListIssueReason `json:"reason"`
	Raw         string          `json:"raw"`
	DuplicateOf int             `json:"duplicateOf,omitempty"`
}

type ListLayout struct {
	Header  bool     `json:"header"`
	Headers []string `json:"headers,omitempty"`
	Width   int      `json:"width"`
	Contact int      `json:"contact"`
	Name    int      `json:"name"`
}

func (l ListLayout) label(i int) string {
	if !l.Header || i < 0 || i >= len(l.Headers) {
		return ""
	}
	return strings.TrimSpace(l.Headers[i])
}

func (l ListLayout) ContactLabel() string { return l.label(l.Contact) }

func (l ListLayout) NameLabel() string { return l.label(l.Name) }

func (l ListLayout) VariableColumns() int {
	if !l.Header {
		return max(l.Width-2, 0)
	}
	count := 0
	for _, h := range l.Headers {
		if n, ok := VariableColumn(h); ok {
			count = max(count, n)
		}
	}
	return count
}

type ListRow struct {
	Line      int
	Contact   string
	Name      string
	Cells     []string
	Variables []string
	Metadata  map[string]any
}

type ContactList struct {
	Layout      ListLayout
	Lines       int
	Rows        []ListRow
	Issues      []ListIssue
	IssueCounts map[ListIssueReason]int
}

func (l ContactList) SkippedTotal() int {
	total := 0
	for _, n := range l.IssueCounts {
		total += n
	}
	return total
}

var emailTarget = sheet.Target{Field: "email", Aliases: []string{"email", "e-mail", "e mail", "endereco de email", "endereco de e-mail", "correio eletronico"}}

var fieldColumns = map[BindingSource][]string{
	BindFirstName: {"primeiro_nome", "primeiro nome", "first name"},
	BindName:      {"nome_completo", "nome completo", "full name"},
	BindNickname:  {"apelido", "nickname"},
	BindDistrict:  {"bairro", "district"},
	BindCity:      {"cidade", "city"},
	BindOwnerName: {"responsavel", "responsavel pelo lead", "owner"},
}

var nameFields = map[BindingSource]bool{BindFirstName: true, BindName: true}

func ReadContactList(rows []sheet.Row, kind ContactKind) (ContactList, error) {
	if !kind.Valid() {
		return ContactList{}, ErrListContactKind
	}
	if len(rows) == 0 {
		return ContactList{}, ErrListEmpty
	}
	width := 0
	for _, row := range rows {
		if len(row.Cells) > MaxListColumns {
			return ContactList{}, ErrListTooWide
		}
		width = max(width, len(row.Cells))
	}
	layout, err := layoutOf(rows[0].Cells, kind)
	if err != nil {
		return ContactList{}, err
	}
	layout.Width = width
	data := rows
	if layout.Header {
		data = rows[1:]
	}
	if len(data) == 0 {
		return ContactList{}, ErrListEmpty
	}
	if len(data) > MaxEntries {
		return ContactList{}, ErrListTooLarge
	}
	list := ContactList{Layout: layout, Lines: len(data), IssueCounts: map[ListIssueReason]int{}}
	seen := map[string]int{}
	for _, row := range data {
		parsed, issue := layout.read(row, kind, seen)
		if issue != nil {
			list.IssueCounts[issue.Reason]++
			if len(list.Issues) < MaxImportIssues {
				list.Issues = append(list.Issues, *issue)
			}
			continue
		}
		seen[parsed.Contact] = row.Line
		list.Rows = append(list.Rows, parsed)
	}
	return list, nil
}

func layoutOf(headers []string, kind ContactKind) (ListLayout, error) {
	layout := ListLayout{Contact: -1, Name: -1}
	header := false
	for i, field := range sheet.Guess(headers, contactTargets(kind), nil) {
		switch field {
		case contactField(kind):
			layout.Contact, header = i, true
		case "name":
			layout.Name, header = i, true
		}
	}
	if kind == ContactPhone && !header {
		header = namesAField(headers)
	}
	if !header {
		return ListLayout{Contact: 0, Name: 1}, nil
	}
	if layout.Contact < 0 {
		return ListLayout{}, ErrListContactColumn
	}
	layout.Header = true
	layout.Headers = append([]string(nil), headers...)
	return layout, nil
}

func namesAField(headers []string) bool {
	for _, h := range headers {
		if _, ok := VariableColumn(h); ok {
			return true
		}
		for _, aliases := range fieldColumns {
			if matchesAlias(h, aliases) {
				return true
			}
		}
	}
	return false
}

func matchesAlias(header string, aliases []string) bool {
	key := sheet.HeaderKey(header)
	if key == "" {
		return false
	}
	for _, alias := range aliases {
		if sheet.HeaderKey(alias) == key {
			return true
		}
	}
	return false
}

func contactTargets(kind ContactKind) []sheet.Target {
	name := importTargets[1]
	if kind == ContactEmail {
		return []sheet.Target{emailTarget, name}
	}
	return []sheet.Target{importTargets[0], name}
}

func contactField(kind ContactKind) string {
	if kind == ContactEmail {
		return emailTarget.Field
	}
	return importTargets[0].Field
}

func (l ListLayout) read(row sheet.Row, kind ContactKind, seen map[string]int) (ListRow, *ListIssue) {
	raw := strings.Join(row.Cells, ";")
	contact, reason := normalizedContact(cellAt(row.Cells, l.Contact), kind)
	if reason != "" {
		return ListRow{}, &ListIssue{Line: row.Line, Reason: reason, Raw: raw}
	}
	if first, dup := seen[contact]; dup {
		return ListRow{}, &ListIssue{Line: row.Line, Reason: ListDuplicate, Raw: raw, DuplicateOf: first}
	}
	parsed := ListRow{Line: row.Line, Contact: contact, Name: cellAt(row.Cells, l.Name)}
	if kind == ContactPhone {
		parsed.Cells = trimmedCells(row.Cells)
	}
	return parsed, nil
}

func cellAt(cells []string, i int) string {
	if i < 0 || i >= len(cells) {
		return ""
	}
	return strings.TrimSpace(cells[i])
}

func trimmedCells(cells []string) []string {
	out := make([]string, len(cells))
	for i, c := range cells {
		out[i] = strings.TrimSpace(c)
	}
	return out
}

func normalizedContact(raw string, kind ContactKind) (string, ListIssueReason) {
	if kind == ContactEmail {
		if raw == "" {
			return "", ListMissingEmail
		}
		address, err := emailaddress.Normalize(raw)
		if err != nil {
			return "", ListInvalidEmail
		}
		return address.String(), ""
	}
	if !strings.ContainsAny(raw, "0123456789") {
		return "", ListMissingNumber
	}
	number := shared.NormalizePhone(raw)
	if number == "" {
		return "", ListInvalidNumber
	}
	return number, ""
}

func NormalizeListContact(raw string, kind ContactKind) (string, ListIssueReason) {
	return normalizedContact(strings.TrimSpace(raw), kind)
}

type FillKind string

const (
	FillColumn FillKind = "column"
	FillName   FillKind = "name"
	FillLead   FillKind = "lead"
	FillFixed  FillKind = "fixed"
	FillNone   FillKind = "none"
)

type VariableColumnMapping struct {
	Slot   int           `json:"slot"`
	Column string        `json:"column,omitempty"`
	Field  BindingSource `json:"field,omitempty"`
	Fill   FillKind      `json:"fill"`
}

type ListTarget struct {
	Field  BindingSource
	Key    string
	Column string
}

func TemplateTargets(fields []BindingSource) []ListTarget {
	out := make([]ListTarget, 0, len(fields))
	for _, field := range fields {
		out = append(out, ListTarget{Field: field})
	}
	return out
}

type MetadataColumn struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type metadataCell struct {
	index int
	key   string
}

type listCell struct {
	index int
	slot  int
	key   string
}

type ListMapping struct {
	Variables       []VariableColumnMapping
	MetadataColumns []MetadataColumn
	cells           []listCell
	metadata        []metadataCell
}

func MapList(layout ListLayout, kind ContactKind, targets []ListTarget, defs []*customfield.Definition) ListMapping {
	if kind != ContactPhone {
		return ListMapping{}
	}
	mapping := ListMapping{}
	used := map[int]bool{}
	slotOfColumn := map[int]int{}
	var templateIndex []int
	for _, target := range targets {
		if target.Key != "" {
			continue
		}
		slot := len(mapping.Variables) + 1
		index := layout.variableIndex(slot, target.Field, defs)
		v := VariableColumnMapping{Slot: slot, Field: target.Field, Fill: fillOf(index, target.Field, layout)}
		if index >= 0 {
			v.Column = columnLabel(layout, index, slot)
			used[index] = true
			slotOfColumn[index] = slot
		}
		mapping.Variables = append(mapping.Variables, v)
		templateIndex = append(templateIndex, index)
	}
	slot := 0
	for _, target := range targets {
		if target.Key == "" {
			mapping.cells = append(mapping.cells, listCell{index: templateIndex[slot], slot: slot + 1})
			slot++
			continue
		}
		index := layout.columnIndex(target.Column)
		mapping.cells = append(mapping.cells, listCell{index: index, slot: slotOfColumn[index], key: target.Key})
	}
	if layout.Header {
		mapping.metadata, mapping.MetadataColumns = metadataCellsOf(layout, used)
	}
	mapping.MetadataColumns = mapping.withKeyColumns()
	return mapping
}

func (m ListMapping) withKeyColumns() []MetadataColumn {
	columns := m.MetadataColumns
	for _, c := range m.cells {
		if c.key == "" || slices.ContainsFunc(columns, func(known MetadataColumn) bool { return known.Key == c.key }) {
			continue
		}
		columns = append(columns, MetadataColumn{Key: c.key, Label: c.key})
	}
	return columns
}

func (l ListLayout) Columns() []string {
	if !l.Header {
		return nil
	}
	out := make([]string, 0, len(l.Headers))
	for _, h := range l.Headers {
		if label := strings.TrimSpace(h); label != "" && !slices.Contains(out, label) {
			out = append(out, label)
		}
	}
	return out
}

func (l ListLayout) HasColumn(header string) bool {
	return l.columnIndex(header) >= 0
}

func (l ListLayout) columnIndex(header string) int {
	header = strings.TrimSpace(header)
	if !l.Header || header == "" {
		return -1
	}
	for i, h := range l.Headers {
		if strings.TrimSpace(h) == header {
			return i
		}
	}
	return -1
}

func (l ListLayout) ColumnNamed(key string) string {
	if key == "" {
		return ""
	}
	for _, h := range l.Columns() {
		if MetadataKey(h) == key {
			return h
		}
	}
	return ""
}

func (l ListLayout) variableIndex(slot int, field BindingSource, defs []*customfield.Definition) int {
	if !l.Header {
		if index := slot + 1; index < l.Width {
			return index
		}
		return -1
	}
	if aliases := fieldAliases(field, defs); len(aliases) > 0 {
		for i, h := range l.Headers {
			if i != l.Contact && i != l.Name && matchesAlias(h, aliases) {
				return i
			}
		}
	}
	for i, h := range l.Headers {
		if n, ok := VariableColumn(h); ok && n == slot {
			return i
		}
	}
	return -1
}

func fillOf(index int, field BindingSource, layout ListLayout) FillKind {
	switch {
	case index >= 0:
		return FillColumn
	case nameFields[field] && layout.Name >= 0 && (layout.Header || layout.Width > layout.Name):
		return FillName
	case field == BindLiteral:
		return FillFixed
	case field != "":
		return FillLead
	}
	return FillNone
}

func columnLabel(layout ListLayout, index, slot int) string {
	if layout.Header {
		return layout.label(index)
	}
	return "var" + strconv.Itoa(slot)
}

func metadataCellsOf(layout ListLayout, used map[int]bool) ([]metadataCell, []MetadataColumn) {
	var cells []metadataCell
	var columns []MetadataColumn
	named := map[string]bool{}
	for i, h := range layout.Headers {
		if i == layout.Contact || i == layout.Name || used[i] {
			continue
		}
		if _, variable := VariableColumn(h); variable {
			continue
		}
		key := MetadataKey(h)
		if key == "" {
			continue
		}
		cells = append(cells, metadataCell{index: i, key: key})
		if !named[key] {
			named[key] = true
			columns = append(columns, MetadataColumn{Key: key, Label: strings.TrimSpace(h)})
		}
	}
	return cells, columns
}

func (m ListMapping) Cells(row ListRow) []string {
	out := make([]string, len(m.cells))
	for i, c := range m.cells {
		out[i] = c.read(row)
	}
	return out
}

func (c listCell) read(row ListRow) string {
	if c.key != "" {
		if value, typed := row.Metadata[c.key]; typed {
			return strings.TrimSpace(agent.MetadataToVars(map[string]any{c.key: value})[c.key])
		}
	}
	if c.slot > 0 && row.Variables != nil {
		return cellAt(row.Variables, c.slot-1)
	}
	return cellAt(row.Cells, c.index)
}

func (m ListMapping) Metadata(row ListRow) map[string]any {
	out := m.baseMetadata(row)
	for _, c := range m.cells {
		if c.key == "" {
			continue
		}
		value := c.read(row)
		if value == "" {
			delete(out, c.key)
			continue
		}
		if out == nil {
			out = map[string]any{}
		}
		out[c.key] = value
	}
	return out
}

func (m ListMapping) baseMetadata(row ListRow) map[string]any {
	if row.Metadata != nil {
		return maps.Clone(row.Metadata)
	}
	var out map[string]any
	for _, c := range m.metadata {
		value := cellAt(row.Cells, c.index)
		if value == "" {
			continue
		}
		if out == nil {
			out = map[string]any{}
		}
		out[c.key] = MetadataValue(value)
	}
	return out
}

func FieldColumn(source BindingSource, defs []*customfield.Definition) string {
	aliases := fieldAliases(source, defs)
	if len(aliases) == 0 {
		return ""
	}
	return aliases[0]
}

func fieldAliases(source BindingSource, defs []*customfield.Definition) []string {
	if aliases, fixed := fieldColumns[source]; fixed {
		return aliases
	}
	key, custom := strings.CutPrefix(string(source), customBindingPrefix)
	if !custom {
		return nil
	}
	for _, def := range BindableCustomFields(defs) {
		if def.Key == key {
			return []string{def.Key, def.Label}
		}
	}
	return nil
}

func MissingSlots(values []string) []int {
	var missing []int
	for i, value := range values {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, i+1)
		}
	}
	return missing
}

func MetadataKey(label string) string {
	folded := shared.FoldForMatch(label)
	var b strings.Builder
	pending := false
	for _, r := range folded {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if pending && b.Len() > 0 {
				b.WriteByte('_')
			}
			pending = false
			b.WriteRune(r)
			continue
		}
		pending = true
	}
	return b.String()
}

func MetadataValue(raw string) any {
	value := strings.TrimSpace(raw)
	switch strings.ToLower(value) {
	case "true":
		return true
	case "false":
		return false
	}
	if n, err := strconv.ParseFloat(value, 64); err == nil && !math.IsInf(n, 0) && !math.IsNaN(n) {
		return n
	}
	return value
}

var variablePrefixes = []string{"variavel", "variable", "var"}

func VariableColumn(header string) (int, bool) {
	key := sheet.HeaderKey(header)
	for _, prefix := range variablePrefixes {
		rest, found := strings.CutPrefix(key, prefix)
		if !found {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(rest))
		if err != nil || n < 1 {
			return 0, false
		}
		return n, true
	}
	return 0, false
}
