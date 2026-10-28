package googlesheet

import (
	"context"
	"fmt"
	"log"
	"time"

	"vozko/domain/googlesheet"
)

const linkCheckTTL = 24 * time.Hour

func linkCheckKey(sheetID string, tabID int64, locale string) string {
	return fmt.Sprintf("gs:links:%s:%d:%s", sheetID, tabID, locale)
}

func (s *session) sawWorkbook(workbook googlesheet.Workbook) {
	s.workbook, s.localeRead = workbook, true
}

func (s *session) needsLocale() bool {
	return !s.localeRead && len(s.sheet.LinkGridColumns()) > 0
}

func (s *session) relink(ctx context.Context) error {
	columns := s.sheet.LinkGridColumns()
	if s.relinked || len(columns) == 0 || !s.localeRead {
		return nil
	}
	s.relinked = true
	locale := s.workbook.Locale
	sep, err := googlesheet.FormulaSeparator(locale)
	if err != nil {
		s.w.metrics.AddGoogleSheetsEvent("link", "locale_unsupported")
		return nil
	}
	targets, err := liveTabs(ctx, s.w.sheets, s.sheet, s.workbook)
	if err != nil {
		return err
	}
	budget := googlesheet.MaxLinkRepairs
	for _, target := range targets {
		key := linkCheckKey(s.sheet.ID, target.TabID, locale)
		due, err := s.w.marks.SetNX(key, "1", linkCheckTTL)
		if err != nil {
			return err
		}
		if !due {
			continue
		}
		fixed, done, err := s.relinkTab(ctx, target, columns, sep, budget)
		budget -= fixed
		if err != nil || !done {
			if delErr := s.w.marks.Del(key); delErr != nil {
				log.Printf("[googlesheet] link check mark of %s not cleared: %v", s.sheet.ID, delErr)
			}
			return err
		}
	}
	return nil
}

func (s *session) relinkTab(ctx context.Context, target googlesheet.Target, columns []int, sep googlesheet.Separator, budget int) (int, bool, error) {
	if ok, err := s.take(ctx, false); !ok || err != nil {
		return 0, false, err
	}
	cells, err := s.w.api.LinkCells(ctx, s.access.Token, target, columns)
	if err != nil {
		s.relinkRefused(target, err)
		return 0, false, nil
	}
	fixes, more := googlesheet.LinkRepairs(cells, sep, budget)
	if len(fixes) == 0 {
		return 0, !more, nil
	}
	if ok, err := s.take(ctx, true); !ok || err != nil {
		return 0, false, err
	}
	if err := s.w.api.WriteFormulas(ctx, s.access.Token, target, fixes); err != nil {
		s.relinkRefused(target, err)
		return 0, false, nil
	}
	s.w.metrics.AddGoogleSheetsEvent("link", "relinked")
	log.Printf("[googlesheet] rewrote %d link formulas of sheet %s tab %d with %q", len(fixes), s.sheet.ID, target.TabID, sep)
	return len(fixes), !more, nil
}

func (s *session) relinkRefused(target googlesheet.Target, err error) {
	failure := s.w.errs.Classify(err)
	s.w.metrics.AddGoogleSheetsEvent("link", "relink_"+string(failure.Kind))
	log.Printf("[googlesheet] link repair of sheet %s tab %d refused: %v", s.sheet.ID, target.TabID, err)
}
