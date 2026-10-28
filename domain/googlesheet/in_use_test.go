package googlesheet

import (
	"errors"
	"testing"
)

func TestASpreadsheetInUseNamesTheCampaignsAndHasItsCode(t *testing.T) {
	err := error(&InUseError{Campaigns: []string{"Cobrança Centro", "Cobrança Norte"}})
	if !errors.Is(err, ErrSheetInUse) || ErrorCode(err) != CodeSheetInUse {
		t.Fatalf("err = %v code %s", err, ErrorCode(err))
	}
}
