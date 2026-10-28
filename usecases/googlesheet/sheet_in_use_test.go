package googlesheet

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"vozko/domain/googlesheet"
)

type campaignUsage struct {
	names []string
	err   error
	asked string
}

func (c *campaignUsage) CampaignsUsing(_ context.Context, _, spreadsheetID string) ([]string, error) {
	c.asked = spreadsheetID
	return c.names, c.err
}

func TestATabOfASpreadsheetARunningCampaignWritesToIsNotDeleted(t *testing.T) {
	w := newWorld(t)
	w.usage.names = []string{"Cobrança Centro"}
	err := w.manager.Delete(context.Background(), testWorkspace, w.sheet.ID)
	var inUse *googlesheet.InUseError
	if !errors.As(err, &inUse) || !errors.Is(err, googlesheet.ErrSheetInUse) || !reflect.DeepEqual(inUse.Campaigns, []string{"Cobrança Centro"}) || w.usage.asked != "sp-1" {
		t.Fatalf("err = %v, asked %q", err, w.usage.asked)
	}
	if _, err := w.sheets.Get(context.Background(), w.sheet.ID); err != nil {
		t.Fatalf("the tab was deleted: %v", err)
	}
	w.usage.names, w.usage.err = nil, errors.New("db down")
	if err := w.manager.Delete(context.Background(), testWorkspace, w.sheet.ID); err == nil {
		t.Fatal("an unreadable usage must not delete")
	}
	w.usage.err = nil
	if err := w.manager.Delete(context.Background(), testWorkspace, w.sheet.ID); err != nil {
		t.Fatalf("an unused tab = %v", err)
	}
	if err := w.manager.Delete(context.Background(), testWorkspace, "gone"); !errors.Is(err, googlesheet.ErrSheetNotFound) {
		t.Fatalf("an unknown tab = %v", err)
	}
}
