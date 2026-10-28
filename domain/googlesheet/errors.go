package googlesheet

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidSheet            = errors.New("googlesheet: invalid sheet definition")
	ErrSheetNotFound           = errors.New("googlesheet: sheet not found")
	ErrSheetNotActive          = errors.New("googlesheet: the sheet needs attention before it accepts rows")
	ErrStaleSheet              = errors.New("googlesheet: the sheet changed since it was read")
	ErrUnknownColumn           = errors.New("googlesheet: the sheet has no such column")
	ErrColumnArchived          = errors.New("googlesheet: the column is archived")
	ErrSystemColumn            = errors.New("googlesheet: the column is filled by the system")
	ErrRequiredColumnMissing   = errors.New("googlesheet: a required column has no value")
	ErrInvalidValue            = errors.New("googlesheet: the value does not fit the column type")
	ErrValueNotInOptions       = errors.New("googlesheet: the value is not one of the column options")
	ErrValueTooLong            = errors.New("googlesheet: the value is longer than a cell holds")
	ErrRowTooLarge             = errors.New("googlesheet: the row is larger than one request holds")
	ErrUnresolvedVariable      = errors.New("googlesheet: a variable in the mapping has no value")
	ErrIdempotencyKeyRequired  = errors.New("googlesheet: an idempotency key is required")
	ErrOriginRequired          = errors.New("googlesheet: the origin of the row is required")
	ErrAccountChanged          = errors.New("googlesheet: the sheet was created by a google account that is no longer connected for sheets")
	ErrRowNotFound             = errors.New("googlesheet: row not found")
	ErrGoogleBusy              = errors.New("googlesheet: the google quota for this minute is used up, try again in a minute")
	ErrNoMessageColumns        = errors.New("googlesheet: the sheet has no column filled per message")
	ErrNeedsConversation       = errors.New("googlesheet: exporting messages needs a conversation")
	ErrNoInputColumns          = errors.New("googlesheet: the sheet has no column filled by agents or workflows")
	ErrSpreadsheetNotFound     = errors.New("googlesheet: no sheet of this workspace lives in that spreadsheet")
	ErrTabNameTaken            = errors.New("googlesheet: the spreadsheet already has a tab with that name")
	ErrLinkTargetInvalid       = errors.New("googlesheet: the link column must point at another sheet with message columns in the same spreadsheet")
	ErrLinkExportMissing       = errors.New("googlesheet: this conversation was not exported to the linked sheet yet")
	ErrPresetNotFound          = errors.New("googlesheet: preset not found")
	ErrLinkTargetPending       = errors.New("googlesheet: the tab the link points at does not exist yet")
	ErrUniqueNeedsConversation = errors.New("googlesheet: this tab keeps one row per conversation and the write has no conversation")
	ErrUniqueValueMissing      = errors.New("googlesheet: a column that keeps rows unique has no value")
)

const (
	CodeInvalidSheet                     = "google_sheet_invalid_definition"
	CodeSheetNotFound                    = "google_sheet_not_found"
	CodeSheetNotActive                   = "google_sheet_not_active"
	CodeStaleSheet                       = "google_sheet_changed"
	CodeUnknownColumn                    = "google_sheet_unknown_column"
	CodeColumnArchived                   = "google_sheet_column_archived"
	CodeSystemColumn                     = "google_sheet_system_column"
	CodeRequiredColumnMissing            = "google_sheet_required_column_missing"
	CodeInvalidValue                     = "google_sheet_invalid_value"
	CodeValueNotInOptions                = "google_sheet_value_not_in_options"
	CodeValueTooLong                     = "google_sheet_value_too_long"
	CodeRowTooLarge                      = "google_sheet_row_too_large"
	CodeUnresolvedVariable               = "google_sheet_unresolved_variable"
	CodeIdempotencyKeyRequired           = "google_sheet_idempotency_key_required"
	CodeOriginRequired                   = "google_sheet_origin_required"
	CodeAccountChanged                   = "google_sheet_account_changed"
	CodeRowNotFound                      = "google_sheet_row_not_found"
	CodeGoogleBusy                       = "google_sheet_google_busy"
	CodeNoMessageColumns                 = "google_sheet_no_message_columns"
	CodeNeedsConversation                = "google_sheet_needs_conversation"
	CodeNoInputColumns                   = "google_sheet_no_input_columns"
	CodeSpreadsheetNotFound              = "google_sheet_spreadsheet_not_found"
	CodeTabNameTaken                     = "google_sheet_tab_name_taken"
	CodeLinkTargetInvalid                = "google_sheet_link_target_invalid"
	CodeLinkExportMissing                = "google_sheet_link_export_missing"
	CodePresetNotFound                   = "google_sheet_preset_not_found"
	CodeUniqueNeedsConversation          = "google_sheet_unique_needs_conversation"
	CodeUniqueValueMissing               = "google_sheet_unique_value_missing"
	CodeAlreadyRecordedConversation      = "google_sheet_already_recorded_conversation"
	CodeAlreadyRecordedConversationMonth = "google_sheet_already_recorded_conversation_month"
	CodeAlreadyRecordedColumns           = "google_sheet_already_recorded_columns"
	CodeAlreadyRecordedColumnsMonth      = "google_sheet_already_recorded_columns_month"
	CodeInternal                         = "google_sheet_failed"
)

var errorCodes = []struct {
	err  error
	code string
}{
	{ErrInvalidSheet, CodeInvalidSheet},
	{ErrSheetNotFound, CodeSheetNotFound},
	{ErrSheetNotActive, CodeSheetNotActive},
	{ErrStaleSheet, CodeStaleSheet},
	{ErrUnknownColumn, CodeUnknownColumn},
	{ErrColumnArchived, CodeColumnArchived},
	{ErrSystemColumn, CodeSystemColumn},
	{ErrRequiredColumnMissing, CodeRequiredColumnMissing},
	{ErrInvalidValue, CodeInvalidValue},
	{ErrValueNotInOptions, CodeValueNotInOptions},
	{ErrValueTooLong, CodeValueTooLong},
	{ErrRowTooLarge, CodeRowTooLarge},
	{ErrUnresolvedVariable, CodeUnresolvedVariable},
	{ErrIdempotencyKeyRequired, CodeIdempotencyKeyRequired},
	{ErrOriginRequired, CodeOriginRequired},
	{ErrAccountChanged, CodeAccountChanged},
	{ErrRowNotFound, CodeRowNotFound},
	{ErrGoogleBusy, CodeGoogleBusy},
	{ErrNoMessageColumns, CodeNoMessageColumns},
	{ErrNeedsConversation, CodeNeedsConversation},
	{ErrNoInputColumns, CodeNoInputColumns},
	{ErrSpreadsheetNotFound, CodeSpreadsheetNotFound},
	{ErrTabNameTaken, CodeTabNameTaken},
	{ErrLinkTargetInvalid, CodeLinkTargetInvalid},
	{ErrLinkExportMissing, CodeLinkExportMissing},
	{ErrPresetNotFound, CodePresetNotFound},
	{ErrUniqueNeedsConversation, CodeUniqueNeedsConversation},
	{ErrUniqueValueMissing, CodeUniqueValueMissing},
	{ErrRowBeingWritten, CodeRowBeingWritten},
	{ErrRowStatusChanged, CodeRowStatusChanged},
	{ErrRowNotInSheet, CodeRowNotInSheet},
	{ErrRowRefRepeated, CodeRowRefRepeated},
	{ErrRemovalReasonRequired, CodeRemovalReasonRequired},
	{ErrRemovalReasonTooLong, CodeRemovalReasonTooLong},
	{ErrRemovalActorRequired, CodeRemovalActorRequired},
	{ErrSheetBusy, CodeSheetBusy},
}

func AllErrors() []error {
	out := make([]error, len(errorCodes))
	for i, entry := range errorCodes {
		out[i] = entry.err
	}
	return out
}

func Codes() []string {
	out := make([]string, 0, len(errorCodes)+len(definitionCodes)+len(RepeatCodes()))
	for _, entry := range errorCodes {
		out = append(out, entry.code)
	}
	return append(append(out, DefinitionCodes()...), RepeatCodes()...)
}

func ErrorCode(err error) string {
	var def *DefinitionError
	if errors.As(err, &def) {
		return def.Code
	}
	var repeat *RepeatError
	if errors.As(err, &repeat) {
		return repeat.Code
	}
	for _, entry := range errorCodes {
		if errors.Is(err, entry.err) {
			return entry.code
		}
	}
	return CodeInternal
}

type ValueError struct {
	Column string
	Header string
	Reason error
	Detail string
}

func (e *ValueError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("coluna %q (%s): %v", e.Header, e.Column, e.Reason)
	}
	return fmt.Sprintf("coluna %q (%s): %v: %s", e.Header, e.Column, e.Reason, e.Detail)
}

func (e *ValueError) Unwrap() error { return e.Reason }
