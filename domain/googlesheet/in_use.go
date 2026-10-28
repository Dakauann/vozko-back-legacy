package googlesheet

import (
	"errors"
	"fmt"
	"strings"
)

const CodeSheetInUse = "google_sheet_in_use"

var ErrSheetInUse = errors.New("googlesheet: a running campaign writes to this spreadsheet")

type InUseError struct {
	Campaigns []string
}

func (e *InUseError) Error() string {
	return fmt.Sprintf("%v: %s", ErrSheetInUse, strings.Join(e.Campaigns, ", "))
}

func (e *InUseError) Unwrap() error { return ErrSheetInUse }
