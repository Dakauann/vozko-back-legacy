package googlesheet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/google/uuid"

	"vozko/domain/cache"
	"vozko/domain/googleaccount"
	"vozko/domain/googlesheet"
	"vozko/domain/metrics"
)

const (
	busyPressure = 0.7
	dispatchPage = 200
	leaseProbe   = 250 * time.Millisecond
	maxJitter    = time.Second
)

type WriterConfig struct {
	BatchRows   int
	BatchBytes  int
	Linger      time.Duration
	BusyLinger  time.Duration
	LeaseTTL    time.Duration
	DrainBudget time.Duration
	AccountWait time.Duration
	TabCheck    time.Duration
	Concurrency int
}

func DefaultWriterConfig(concurrency int) WriterConfig {
	return WriterConfig{
		BatchRows:   googlesheet.DefaultBatchRows,
		BatchBytes:  googlesheet.DefaultBatchBytes,
		Linger:      2 * time.Second,
		BusyLinger:  10 * time.Second,
		LeaseTTL:    2 * time.Minute,
		DrainBudget: 50 * time.Second,
		AccountWait: 5 * time.Minute,
		TabCheck:    10 * time.Minute,
		Concurrency: concurrency,
	}
}

type WriterDeps struct {
	Sheets  googlesheet.SheetRepository
	Rows    googlesheet.RowRepository
	API     googlesheet.SheetsAPI
	Errors  googlesheet.ErrorClassifier
	Access  googleaccount.AccessSource
	Limiter googlesheet.Limiter
	Leases  cache.Locker
	Marks   Marker
	Alerts  Alerts
	Metrics metrics.GoogleSheetsMetricsRecorder
	Config  WriterConfig
}

type Writer struct {
	sheets  googlesheet.SheetRepository
	rows    googlesheet.RowRepository
	api     googlesheet.SheetsAPI
	errs    googlesheet.ErrorClassifier
	access  googleaccount.AccessSource
	limiter googlesheet.Limiter
	leases  cache.Locker
	marks   Marker
	alerts  Alerts
	metrics metrics.GoogleSheetsMetricsRecorder
	cfg     WriterConfig
	now     Clock
	seed    func() int64
	jitter  func() time.Duration
	slots   chan struct{}
	nudged  sync.Map
}

var _ Nudger = (*Writer)(nil)

func NewWriter(d WriterDeps) *Writer {
	if d.Sheets == nil || d.Rows == nil || d.API == nil || d.Errors == nil || d.Access == nil || d.Limiter == nil ||
		d.Leases == nil || d.Marks == nil || d.Alerts == nil || d.Metrics == nil {
		panic("googlesheet: the writer requires sheets, rows, the sheets api, an error classifier, google access, a limiter, leases, marks, alerts and metrics")
	}
	if d.Config.Concurrency < 1 || d.Config.BatchRows < 1 || d.Config.BatchBytes < 1 || d.Config.LeaseTTL <= d.Config.DrainBudget {
		panic(fmt.Sprintf("googlesheet: invalid writer config %+v", d.Config))
	}
	return &Writer{
		sheets: d.Sheets, rows: d.Rows, api: d.API, errs: d.Errors, access: d.Access, limiter: d.Limiter,
		leases: d.Leases, marks: d.Marks, alerts: d.Alerts, metrics: d.Metrics, cfg: d.Config,
		now:    utcNow,
		seed:   rand.Int64,
		jitter: func() time.Duration { return time.Duration(rand.Int64N(int64(maxJitter))) },
		slots:  make(chan struct{}, d.Config.Concurrency),
	}
}

func (w *Writer) Nudge(sheetID string) {
	if _, scheduled := w.nudged.LoadOrStore(sheetID, struct{}{}); scheduled {
		return
	}
	time.AfterFunc(w.linger(context.Background()), func() {
		w.nudged.Delete(sheetID)
		w.run(context.Background(), sheetID)
	})
}

func (w *Writer) DispatchPending(ctx context.Context) error {
	now := w.now()
	pending, err := w.rows.PendingSheets(ctx, now, dispatchPage)
	if err != nil {
		return err
	}
	linger := w.linger(ctx)
	var wg sync.WaitGroup
	for _, p := range pending {
		if now.Sub(p.OldestPendingAt) < linger {
			continue
		}
		wg.Add(1)
		go func(sheetID string) {
			defer wg.Done()
			w.run(ctx, sheetID)
		}(p.SheetID)
	}
	wg.Wait()
	return nil
}

func (w *Writer) run(ctx context.Context, sheetID string) {
	select {
	case w.slots <- struct{}{}:
	case <-ctx.Done():
		return
	}
	defer func() { <-w.slots }()
	runCtx, cancel := context.WithTimeout(ctx, w.cfg.LeaseTTL)
	defer cancel()
	if err := w.WriteSheet(runCtx, sheetID); err != nil {
		w.metrics.AddGoogleSheetsEvent("write", "error")
		log.Printf("[googlesheet] writing sheet %s failed: %v", sheetID, err)
	}
}

func (w *Writer) linger(ctx context.Context) time.Duration {
	pressure, err := w.limiter.WritePressure(ctx)
	if err != nil || pressure >= busyPressure {
		return w.cfg.BusyLinger
	}
	return w.cfg.Linger
}

func (w *Writer) WriteSheet(ctx context.Context, sheetID string) error {
	release, busy, err := acquireSheet(ctx, w.leases, sheetID)
	if busy {
		w.metrics.AddGoogleSheetsEvent("lease", "busy")
		return nil
	}
	if err != nil {
		return err
	}
	defer release()
	sheet, err := w.sheets.Get(ctx, sheetID)
	if errors.Is(err, googlesheet.ErrSheetNotFound) {
		return w.rows.DeleteUnwritten(ctx, sheetID)
	}
	if err != nil {
		return err
	}
	if sheet.Writable() != nil {
		return nil
	}
	access, err := w.access.Access(ctx, sheet.WorkspaceID, googleaccount.ProductSheets)
	if errors.Is(err, googleaccount.ErrNotConnected) {
		w.metrics.AddGoogleSheetsEvent("access", "waiting_for_account")
		return nil
	}
	if err != nil {
		return err
	}
	if access.AccountID != sheet.AccountID {
		return w.attention(ctx, sheet, googlesheet.ReasonAccountChanged, "a conta Google conectada ao Planilhas não é a que criou esta planilha")
	}
	s := &session{w: w, sheet: sheet, access: access}
	if ok, err := s.recover(ctx); err != nil || !ok {
		return err
	}
	deadline := w.now().Add(w.cfg.DrainBudget)
	for w.now().Before(deadline) {
		more, err := s.writeNext(ctx)
		if err != nil || !more {
			return err
		}
	}
	return nil
}

func (w *Writer) attention(ctx context.Context, sheet *googlesheet.Sheet, reason googlesheet.AttentionReason, detail string) error {
	sheet.NeedsAttention(reason, detail)
	if err := w.sheets.SaveStatus(ctx, sheet); err != nil {
		return err
	}
	w.metrics.AddGoogleSheetsEvent("attention", string(reason))
	if err := w.alerts.NeedsAttention(ctx, sheet); err != nil {
		log.Printf("[googlesheet] attention alert of %s not published: %v", sheet.ID, err)
	}
	return nil
}

type session struct {
	w      *Writer
	sheet  *googlesheet.Sheet
	access googleaccount.Access
}

func (s *session) take(ctx context.Context, write bool) (bool, error) {
	take, stage := s.w.limiter.TakeRead, "read_wait"
	if write {
		take, stage = s.w.limiter.TakeWrite, "write_wait"
	}
	granted, err := take(ctx, s.access.Email)
	if err != nil {
		return false, err
	}
	if !granted {
		s.w.metrics.AddGoogleSheetsEvent("limiter", stage)
	}
	return granted, nil
}

func (s *session) recover(ctx context.Context) (bool, error) {
	w := s.w
	stuck, err := w.rows.StuckInFlight(ctx, s.sheet.ID, w.now().Add(-w.cfg.LeaseTTL))
	if err != nil || len(stuck) == 0 {
		return err == nil, err
	}
	byPeriod := map[string][]*googlesheet.Row{}
	var periods []string
	for _, row := range stuck {
		if _, seen := byPeriod[row.PeriodKey]; !seen {
			periods = append(periods, row.PeriodKey)
		}
		byPeriod[row.PeriodKey] = append(byPeriod[row.PeriodKey], row)
	}
	found := 0
	for _, period := range periods {
		ok, n, err := s.recoverPeriod(ctx, period, byPeriod[period])
		if err != nil || !ok {
			return false, err
		}
		found += n
	}
	w.metrics.AddGoogleSheetsEvent("recover", fmt.Sprintf("found_%d_of_%d", found, len(stuck)))
	return true, nil
}

func (s *session) recoverPeriod(ctx context.Context, period string, stuck []*googlesheet.Row) (bool, int, error) {
	w := s.w
	target, known, err := s.knownTarget(ctx, period)
	if err != nil {
		return false, 0, err
	}
	if !known {
		return true, 0, w.rows.Release(ctx, rowIDs(stuck), w.now(), "")
	}
	if ok, err := s.take(ctx, false); !ok || err != nil {
		return false, 0, err
	}
	refs, err := w.api.ReadRange(ctx, s.access.Token, target, googlesheet.RefColumnRange)
	if err != nil {
		return false, 0, s.readFailed(ctx, err)
	}
	written, missing := alreadyWritten(googlesheet.TabSnapshot{Refs: refs}, stuck)
	if err := w.rows.MarkWritten(ctx, written, w.now()); err != nil {
		return false, 0, err
	}
	return true, len(written), w.rows.Release(ctx, rowIDs(missing), w.now(), "")
}

func alreadyWritten(snapshot googlesheet.TabSnapshot, rows []*googlesheet.Row) ([]googlesheet.WrittenRow, []*googlesheet.Row) {
	positions := snapshot.RefPositions()
	var written []googlesheet.WrittenRow
	var missing []*googlesheet.Row
	for _, row := range rows {
		if number, ok := positions[row.ID]; ok {
			written = append(written, googlesheet.WrittenRow{ID: row.ID, RowNumber: number})
			continue
		}
		missing = append(missing, row)
	}
	return written, missing
}

func (s *session) knownTarget(ctx context.Context, period string) (googlesheet.Target, bool, error) {
	return periodTarget(ctx, s.w.sheets, s.sheet, period)
}

func (s *session) target(ctx context.Context, period string) (googlesheet.Target, bool, error) {
	target, known, err := s.knownTarget(ctx, period)
	if err != nil || known {
		return target, known, err
	}
	return s.openTab(ctx, period)
}

func (s *session) openTab(ctx context.Context, period string) (googlesheet.Target, bool, error) {
	w := s.w
	title, err := s.sheet.TabTitleFor(period)
	if err != nil {
		return googlesheet.Target{}, false, err
	}
	if ok, err := s.take(ctx, false); !ok || err != nil {
		return googlesheet.Target{}, false, err
	}
	workbook, err := w.api.Workbook(ctx, s.access.Token, s.sheet.SpreadsheetID)
	if err != nil {
		return googlesheet.Target{}, false, s.readFailed(ctx, err)
	}
	target := googlesheet.Target{SpreadsheetID: s.sheet.SpreadsheetID, TabName: title}
	if existing, taken := workbook.Titled(title); taken {
		target.TabID, target.TabName = existing.ID, existing.Title
		if ok, err := s.adopt(ctx, target); err != nil || !ok {
			return googlesheet.Target{}, false, err
		}
	} else {
		if workbook.NearlyFull(googlesheet.NewTabRows * (len(s.sheet.Columns) + 1)) {
			return googlesheet.Target{}, false, w.attention(ctx, s.sheet, googlesheet.ReasonSheetFull, fmt.Sprintf("%d células no arquivo", workbook.Cells()))
		}
		if ok, err := s.take(ctx, true); !ok || err != nil {
			return googlesheet.Target{}, false, err
		}
		target.TabID = workbook.FreeTabID(w.seed())
		if err := w.api.AddTab(ctx, s.access.Token, s.sheet.SpreadsheetID, googlesheet.TabSpec{ID: target.TabID, Title: title, Layout: s.sheet.Layout(0)}); err != nil {
			return googlesheet.Target{}, false, s.layoutFailed(ctx, err)
		}
		w.metrics.AddGoogleSheetsEvent("tab", "opened")
	}
	tab := googlesheet.Tab{Period: period, ID: target.TabID, Title: target.TabName}
	if err := w.sheets.OpenTab(ctx, s.sheet.ID, tab); err != nil {
		return googlesheet.Target{}, false, err
	}
	if period > s.sheet.TabPeriodKey {
		s.sheet.TabID, s.sheet.TabName, s.sheet.TabPeriodKey = tab.ID, tab.Title, tab.Period
	}
	return target, true, nil
}

func (s *session) adopt(ctx context.Context, target googlesheet.Target) (bool, error) {
	w := s.w
	if ok, err := s.take(ctx, false); !ok || err != nil {
		return false, err
	}
	header, err := w.api.ReadRange(ctx, s.access.Token, target, googlesheet.HeaderRange)
	if err != nil {
		return false, s.readFailed(ctx, err)
	}
	if !googlesheet.HeaderIsBlank(header) {
		if difference := googlesheet.HeaderDifference(s.sheet.HeaderRow(), header); difference != "" {
			return false, w.attention(ctx, s.sheet, googlesheet.ReasonHeaderMismatch, fmt.Sprintf("a aba %q já existe com outro conteúdo; %s", target.TabName, difference))
		}
		return true, nil
	}
	if ok, err := s.take(ctx, true); !ok || err != nil {
		return false, err
	}
	if err := w.api.ApplyLayout(ctx, s.access.Token, target, s.sheet.Layout(0)); err != nil {
		return false, s.layoutFailed(ctx, err)
	}
	return true, nil
}

func (s *session) layoutFailed(ctx context.Context, err error) error {
	failure := s.w.errs.Classify(err)
	switch failure.Kind {
	case googlesheet.FailurePermission, googlesheet.FailureNotFound:
		return s.w.attention(ctx, s.sheet, failure.AttentionReason(), failure.Detail)
	case googlesheet.FailureInvalid:
		return s.w.attention(ctx, s.sheet, googlesheet.ReasonGoogleUnavailable, failure.Detail)
	}
	s.w.metrics.AddGoogleSheetsEvent("tab", string(failure.Kind))
	return nil
}

func (s *session) verify(ctx context.Context, target googlesheet.Target) (bool, error) {
	w := s.w
	if target.TabID != s.sheet.TabID {
		return s.checkRoom(ctx, target)
	}
	key := tabCheckKey(s.sheet.ID)
	due, err := w.marks.SetNX(key, "1", w.cfg.TabCheck)
	if err != nil || !due {
		return err == nil, err
	}
	ok, err := s.checkRoom(ctx, target)
	if !ok {
		if delErr := w.marks.Del(key); delErr != nil {
			log.Printf("[googlesheet] tab check mark of %s not cleared: %v", s.sheet.ID, delErr)
		}
	}
	return ok, err
}

func (s *session) reads() tabReads {
	return tabReads{
		api: s.w.api, token: s.access.Token,
		take:   func(ctx context.Context) (bool, error) { return s.take(ctx, false) },
		failed: s.readFailed,
	}
}

type cursor struct {
	target  googlesheet.Target
	next    int
	present googlesheet.TabSnapshot
}

func (s *session) position(ctx context.Context, target googlesheet.Target) (*cursor, bool, error) {
	inspection, ok, err := s.reads().inspect(ctx, s.sheet, target)
	if err != nil || !ok {
		return nil, false, err
	}
	if place := inspection.place; !place.InPlace() {
		return nil, false, s.w.attention(ctx, s.sheet, place.Reason, tabDetail(s.sheet, target, place.Detail))
	}
	return &cursor{target: target, next: inspection.snapshot.NextFreeRow(), present: inspection.snapshot}, true, nil
}

func (s *session) checkRoom(ctx context.Context, target googlesheet.Target) (bool, error) {
	w := s.w
	if ok, err := s.take(ctx, false); !ok || err != nil {
		return false, err
	}
	workbook, err := w.api.Workbook(ctx, s.access.Token, target.SpreadsheetID)
	if err != nil {
		return false, s.readFailed(ctx, err)
	}
	if _, ok := workbook.Tab(target.TabID); !ok {
		return false, w.attention(ctx, s.sheet, googlesheet.ReasonSpreadsheetGone, tabGoneDetail)
	}
	if workbook.NearlyFull(0) {
		return false, w.attention(ctx, s.sheet, googlesheet.ReasonSheetFull, fmt.Sprintf("%d células no arquivo", workbook.Cells()))
	}
	return true, nil
}

func (s *session) readFailed(ctx context.Context, err error) error {
	failure := s.w.errs.Classify(err)
	switch failure.Kind {
	case googlesheet.FailurePermission, googlesheet.FailureNotFound:
		return s.w.attention(ctx, s.sheet, failure.AttentionReason(), failure.Detail)
	}
	s.w.metrics.AddGoogleSheetsEvent("read", string(failure.Kind))
	return nil
}

func (s *session) writeNext(ctx context.Context) (bool, error) {
	w := s.w
	if ok, err := s.take(ctx, true); !ok || err != nil {
		return false, err
	}
	claimed, err := w.rows.Claim(ctx, s.sheet.ID, uuid.NewString(), w.cfg.BatchRows, w.now())
	if err != nil || len(claimed) == 0 {
		return false, err
	}
	run := samePeriod(claimed)
	if err := w.rows.Unclaim(ctx, rowIDs(claimed[len(run):])); err != nil {
		return false, err
	}
	target, ok, err := s.target(ctx, run[0].PeriodKey)
	if err == nil && ok {
		ok, err = s.verify(ctx, target)
	}
	var at *cursor
	if err == nil && ok {
		at, ok, err = s.position(ctx, target)
	}
	if err != nil || !ok {
		return false, errors.Join(err, w.rows.Unclaim(ctx, rowIDs(run)))
	}
	found, run := alreadyWritten(at.present, run)
	if len(found) > 0 {
		if err := w.rows.MarkWritten(ctx, found, w.now()); err != nil {
			return false, errors.Join(err, w.rows.Unclaim(ctx, rowIDs(run)))
		}
		w.metrics.AddGoogleSheetsEvent("write", "already_in_sheet")
	}
	if len(run) == 0 {
		return true, nil
	}
	batch, values, sizes, err := s.encode(ctx, run)
	if err != nil {
		return false, err
	}
	fits := googlesheet.PlanBatch(sizes, w.cfg.BatchRows, w.cfg.BatchBytes)
	if fits < len(batch) {
		if err := w.rows.Unclaim(ctx, rowIDs(batch[fits:])); err != nil {
			return false, err
		}
		batch, values = batch[:fits], values[:fits]
	}
	if len(batch) == 0 {
		return true, nil
	}
	return s.send(ctx, at, batch, values)
}

func samePeriod(rows []*googlesheet.Row) []*googlesheet.Row {
	for i, row := range rows {
		if row.PeriodKey != rows[0].PeriodKey {
			return rows[:i]
		}
	}
	return rows
}

func (s *session) linkTabs(ctx context.Context, rows []*googlesheet.Row) (googlesheet.LinkTabs, map[string]bool, error) {
	tabs := googlesheet.LinkTabs{}
	gone := map[string]bool{}
	seen := map[string]bool{}
	for _, row := range rows {
		for _, ref := range s.sheet.LinkRefs(row.Cells) {
			key := googlesheet.LinkTabKey(ref.SheetID, ref.Period)
			if seen[key] {
				continue
			}
			seen[key] = true
			target, err := s.w.sheets.Get(ctx, ref.SheetID)
			if errors.Is(err, googlesheet.ErrSheetNotFound) || (err == nil && target.WorkspaceID != s.sheet.WorkspaceID) {
				gone[ref.SheetID] = true
				continue
			}
			if err != nil {
				return nil, nil, err
			}
			if !target.Periodic() {
				tabs[key] = googlesheet.TabRef{ID: target.TabID, Title: target.TabName}
				continue
			}
			tab, found, err := s.w.sheets.FindTab(ctx, target.ID, ref.Period)
			if err != nil {
				return nil, nil, err
			}
			if found {
				tabs[key] = googlesheet.TabRef{ID: tab.ID, Title: tab.Title}
			}
		}
	}
	return tabs, gone, nil
}

func (s *session) encode(ctx context.Context, claimed []*googlesheet.Row) ([]*googlesheet.Row, [][]any, []int, error) {
	tabs, gone, err := s.linkTabs(ctx, claimed)
	if err != nil {
		return nil, nil, nil, errors.Join(err, s.w.rows.Unclaim(ctx, rowIDs(claimed)))
	}
	batch := make([]*googlesheet.Row, 0, len(claimed))
	values := make([][]any, 0, len(claimed))
	sizes := make([]int, 0, len(claimed))
	for i, row := range claimed {
		encoded, err := s.sheet.EncodeRow(row.ID, row.Cells, tabs)
		if errors.Is(err, googlesheet.ErrLinkTargetPending) {
			rest := claimed[i:]
			if len(batch) == 0 {
				if err := s.waitForLink(ctx, row, gone); err != nil {
					return nil, nil, nil, err
				}
				rest = claimed[i+1:]
			}
			return batch, values, sizes, s.w.rows.Unclaim(ctx, rowIDs(rest))
		}
		if err == nil {
			var raw []byte
			raw, err = json.Marshal(encoded)
			if err == nil {
				batch, values, sizes = append(batch, row), append(values, encoded), append(sizes, len(raw))
				continue
			}
		}
		if failErr := s.w.rows.Fail(ctx, row.ID, googlesheet.ErrorCode(err), err.Error()); failErr != nil {
			log.Printf("[googlesheet] row %s could not be encoded and not marked failed: %v", row.ID, failErr)
		}
	}
	return batch, values, sizes, nil
}

func (s *session) waitForLink(ctx context.Context, row *googlesheet.Row, gone map[string]bool) error {
	w := s.w
	for _, ref := range s.sheet.LinkRefs(row.Cells) {
		if gone[ref.SheetID] {
			return w.rows.Fail(ctx, row.ID, googlesheet.CodeWriteLinkTargetMissing, "a planilha de destino do link foi removida")
		}
	}
	if row.Attempts >= googlesheet.MaxTransientAttempts {
		return w.rows.Fail(ctx, row.ID, googlesheet.CodeWriteLinkTargetMissing, "a aba de destino do link não apareceu a tempo")
	}
	w.metrics.AddGoogleSheetsEvent("link", "pending")
	return w.rows.Release(ctx, []string{row.ID}, w.now().Add(googlesheet.Backoff(row.Attempts, w.jitter())), googlesheet.CodeWriteLinkPending)
}

func (s *session) send(ctx context.Context, at *cursor, batch []*googlesheet.Row, values [][]any) (bool, error) {
	w := s.w
	started := time.Now()
	err := w.api.InsertRows(ctx, s.access.Token, at.target, at.next, values)
	w.metrics.ObserveGoogleSheetsBatch(len(batch), time.Since(started).Seconds())
	if err == nil {
		first := at.next
		at.next += len(batch)
		return true, s.written(ctx, batch, first)
	}
	failure := w.errs.Classify(err)
	w.metrics.AddGoogleSheetsEvent("write", string(failure.Kind))
	ids := rowIDs(batch)
	switch googlesheet.Decide(failure, batch[0].Attempts) {
	case googlesheet.OutcomeRetry:
		wait := googlesheet.Backoff(batch[0].Attempts, w.jitter())
		if failure.RetryAfter > wait {
			wait = failure.RetryAfter
		}
		return false, w.rows.Release(ctx, ids, w.now().Add(wait), failure.Code())
	case googlesheet.OutcomeWaitForAccount:
		return false, w.rows.Release(ctx, ids, w.now().Add(w.cfg.AccountWait), failure.Code())
	case googlesheet.OutcomeHold:
		if err := w.rows.Hold(ctx, ids, failure.Code(), failure.Detail); err != nil {
			return false, err
		}
		return false, w.attention(ctx, s.sheet, failure.AttentionReason(), failure.Detail)
	}
	return s.isolate(ctx, at, batch, values, failure)
}

func (s *session) written(ctx context.Context, batch []*googlesheet.Row, first int) error {
	w := s.w
	rows := make([]googlesheet.WrittenRow, len(batch))
	for i, row := range batch {
		rows[i] = googlesheet.WrittenRow{ID: row.ID, RowNumber: first + i}
	}
	at := w.now()
	if err := w.rows.MarkWritten(ctx, rows, at); err != nil {
		return err
	}
	w.metrics.AddGoogleSheetsEvent("write", "written")
	return w.sheets.TouchWritten(ctx, s.sheet.ID, at)
}

func (s *session) isolate(ctx context.Context, at *cursor, batch []*googlesheet.Row, values [][]any, failure googlesheet.Failure) (bool, error) {
	w := s.w
	if len(batch) == 1 {
		return true, w.rows.Fail(ctx, batch[0].ID, failure.Code(), failure.Detail)
	}
	half := len(batch) / 2
	for _, part := range [][2]int{{0, half}, {half, len(batch)}} {
		start, end := part[0], part[1]
		if ok, err := s.take(ctx, true); !ok || err != nil {
			return false, errors.Join(err, w.rows.Unclaim(ctx, rowIDs(batch[start:])))
		}
		more, err := s.send(ctx, at, batch[start:end], values[start:end])
		if err != nil || !more {
			return false, errors.Join(err, w.rows.Unclaim(ctx, rowIDs(batch[end:])))
		}
	}
	return true, nil
}

func rowIDs(rows []*googlesheet.Row) []string {
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	return ids
}
