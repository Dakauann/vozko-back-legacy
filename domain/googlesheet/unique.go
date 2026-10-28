package googlesheet

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"vozko/domain/shared"
)

type UniqueMode string

const (
	UniqueNone         UniqueMode = "none"
	UniqueConversation UniqueMode = "conversation"
	UniqueColumns      UniqueMode = "columns"
)

func UniqueModes() []UniqueMode { return []UniqueMode{UniqueNone, UniqueConversation, UniqueColumns} }

func (m UniqueMode) Valid() bool { return slices.Contains(UniqueModes(), m) }

type RepeatAction string

const RepeatSkip RepeatAction = "skip"

func RepeatActions() []RepeatAction { return []RepeatAction{RepeatSkip} }

func (a RepeatAction) Valid() bool { return slices.Contains(RepeatActions(), a) }

type UniqueRule struct {
	Mode     UniqueMode   `json:"mode"`
	Columns  []string     `json:"columns,omitempty"`
	OnRepeat RepeatAction `json:"on_repeat"`
}

type UniqueInput struct {
	Mode     UniqueMode
	Columns  []int
	OnRepeat RepeatAction
}

func (r UniqueRule) Normalized() UniqueRule {
	out := UniqueRule{Mode: r.Mode, OnRepeat: r.OnRepeat}
	if out.Mode == "" {
		out.Mode = UniqueNone
	}
	if out.OnRepeat == "" {
		out.OnRepeat = RepeatSkip
	}
	for _, key := range r.Columns {
		out.Columns = append(out.Columns, strings.TrimSpace(key))
	}
	return out
}

func (r UniqueRule) Active() bool { return r.Normalized().Mode != UniqueNone }

var nonKeySources = []Source{SourceWrittenAt, SourceExportLink}

func (s Source) UniqueKey() bool {
	return s.Valid() && !s.PerMessage() && !slices.Contains(nonKeySources, s)
}

func UniqueSources() []Source {
	out := []Source{SourceInput}
	for _, source := range SystemSources() {
		if source.UniqueKey() {
			out = append(out, source)
		}
	}
	return out
}

func (s *Sheet) resolveUnique(in UniqueInput) (UniqueRule, error) {
	rule := UniqueRule{Mode: in.Mode, OnRepeat: in.OnRepeat}
	for _, index := range in.Columns {
		if index < 0 || index >= len(s.Columns) {
			refusal := refuse(FieldUnique, CodeUniqueColumnUnknown).detail("coluna %d", index)
			refusal.Column = index
			return UniqueRule{}, refusal
		}
		rule.Columns = append(rule.Columns, s.Columns[index].Key)
	}
	return rule.Normalized(), nil
}

func (s *Sheet) validateUnique(field string) error {
	rule := s.Unique.Normalized()
	switch {
	case !rule.Mode.Valid():
		return refuse(field, CodeUniqueModeUnknown).detail("%q", rule.Mode)
	case !rule.OnRepeat.Valid():
		return refuse(field, CodeUniqueRepeatUnknown).detail("%q", rule.OnRepeat)
	case rule.Mode != UniqueColumns && len(rule.Columns) > 0:
		return refuse(field, CodeUniqueColumnsUnexpected)
	case rule.Mode == UniqueColumns && len(rule.Columns) == 0:
		return refuse(field, CodeUniqueColumnsRequired)
	}
	seen := map[string]bool{}
	for _, key := range rule.Columns {
		index := slices.IndexFunc(s.Columns, func(col Column) bool { return col.Key == key })
		if index < 0 {
			refusal := refuse(field, CodeUniqueColumnUnknown).detail("%q", key)
			refusal.Key = key
			return refusal
		}
		col := s.Columns[index]
		code := ""
		switch {
		case seen[key]:
			code = CodeUniqueColumnRepeated
		case col.Archived:
			code = CodeUniqueColumnArchived
		case !col.Source.UniqueKey():
			code = CodeUniqueColumnNotAllowed
		}
		if code != "" {
			refusal := columnRefusal(code, col)
			refusal.Field, refusal.Column = field, index
			if field == FieldUpdate {
				refusal.Column = -1
			}
			return refusal
		}
		seen[key] = true
	}
	return nil
}

func (s *Sheet) ChangeUnique(rule UniqueRule) error {
	draft := *s
	draft.Unique = rule.Normalized()
	if err := draft.validateUnique(FieldUnique); err != nil {
		return err
	}
	s.Unique = draft.Unique
	s.Version++
	return nil
}

type UniqueKey struct {
	Period string
	Hash   string
}

func (k UniqueKey) Empty() bool { return k.Hash == "" }

func (s *Sheet) UniqueKeyOf(cells Cells, origin Origin, period string) (UniqueKey, error) {
	rule := s.Unique.Normalized()
	switch rule.Mode {
	case UniqueNone:
		return UniqueKey{}, nil
	case UniqueConversation:
		entry, ok := origin.Conversation()
		if !ok {
			return UniqueKey{}, ErrUniqueNeedsConversation
		}
		return uniqueKey(period, string(UniqueConversation), string(entry.EntryType), entry.EntryID), nil
	case UniqueColumns:
		parts := []string{string(UniqueColumns)}
		for _, key := range rule.Columns {
			col, ok := s.Column(key)
			if !ok || col.Archived {
				return UniqueKey{}, &ValueError{Column: key, Header: key, Reason: ErrUnknownColumn}
			}
			text, ok := uniqueText(cells[key])
			if !ok {
				return UniqueKey{}, &ValueError{Column: key, Header: col.Header, Reason: ErrUniqueValueMissing}
			}
			parts = append(parts, key, text)
		}
		return uniqueKey(period, parts...), nil
	}
	return UniqueKey{}, refuse(FieldUnique, CodeUniqueModeUnknown).detail("%q", rule.Mode)
}

func uniqueText(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		folded := shared.FoldForMatch(v)
		return folded, folded != ""
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case bool:
		return strconv.FormatBool(v), true
	}
	return "", false
}

func uniqueKey(period string, parts ...string) UniqueKey {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return UniqueKey{Period: period, Hash: hex.EncodeToString(sum[:])}
}

var ErrAlreadyRecorded = errors.New("googlesheet: this tab already has a row for this record")

type RepeatError struct {
	Code    string
	RowID   string
	Headers []string
}

func (e *RepeatError) Error() string {
	return ErrAlreadyRecorded.Error() + ": " + e.Code + ": " + e.RowID
}

func (e *RepeatError) Unwrap() error { return ErrAlreadyRecorded }

func (s *Sheet) Repeat(rowID string) *RepeatError {
	rule := s.Unique.Normalized()
	repeat := &RepeatError{RowID: rowID, Code: repeatCode(rule.Mode, s.Periodic())}
	if rule.Mode == UniqueColumns {
		for _, key := range rule.Columns {
			header := key
			if col, ok := s.Column(key); ok {
				header = col.Header
			}
			repeat.Headers = append(repeat.Headers, header)
		}
	}
	return repeat
}

func repeatCode(mode UniqueMode, monthly bool) string {
	switch {
	case mode == UniqueColumns && monthly:
		return CodeAlreadyRecordedColumnsMonth
	case mode == UniqueColumns:
		return CodeAlreadyRecordedColumns
	case monthly:
		return CodeAlreadyRecordedConversationMonth
	}
	return CodeAlreadyRecordedConversation
}

func RepeatCodes() []string {
	return []string{CodeAlreadyRecordedConversation, CodeAlreadyRecordedConversationMonth, CodeAlreadyRecordedColumns, CodeAlreadyRecordedColumnsMonth}
}

type Inserted struct {
	Row       *Row
	Duplicate bool
	RepeatOf  string
}

type UniqueClaim struct {
	Row *Row
	Key UniqueKey
}

type LedgerCursor struct {
	CreatedAt time.Time
	ID        string
}

func (r *Row) LedgerCursor() LedgerCursor { return LedgerCursor{CreatedAt: r.CreatedAt, ID: r.ID} }

func (s *Sheet) ClaimsOf(rows []*Row) []UniqueClaim {
	var claims []UniqueClaim
	for _, row := range rows {
		key, err := s.UniqueKeyOf(row.Cells, row.Origin, row.PeriodKey)
		if err != nil || key.Empty() {
			continue
		}
		claims = append(claims, UniqueClaim{Row: row, Key: key})
	}
	return claims
}

func (r UniqueRule) Same(other UniqueRule) bool {
	a, b := r.Normalized(), other.Normalized()
	return a.Mode == b.Mode && a.OnRepeat == b.OnRepeat && slices.Equal(a.Columns, b.Columns)
}

func (e *RepeatError) Message() string {
	scope := "nesta aba"
	if e.Code == CodeAlreadyRecordedConversationMonth || e.Code == CodeAlreadyRecordedColumnsMonth {
		scope = "neste mês"
	}
	if len(e.Headers) > 0 {
		return "Já existe uma linha com o mesmo valor em " + e.HeaderList() + " " + scope + "."
	}
	return "Já existe uma linha para esta conversa " + scope + "."
}

func (e *RepeatError) HeaderList() string { return strings.Join(e.Headers, ", ") }
