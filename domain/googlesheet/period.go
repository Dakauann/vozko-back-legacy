package googlesheet

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type TabPeriod string

const (
	PeriodNone  TabPeriod = "none"
	PeriodMonth TabPeriod = "month"
)

func TabPeriods() []TabPeriod { return []TabPeriod{PeriodNone, PeriodMonth} }

func (p TabPeriod) Valid() bool { return p == PeriodNone || p == PeriodMonth }

const (
	TokenMonthName = "{mes}"
	TokenMonth     = "{mm}"
	TokenYear      = "{ano}"
	tabForbidden   = "'[]*?/\\:"
	periodLayout   = "2006-01"
)

func TabTokens() []string { return []string{TokenMonthName, TokenMonth, TokenYear} }

var monthNames = [...]string{"janeiro", "fevereiro", "março", "abril", "maio", "junho", "julho", "agosto", "setembro", "outubro", "novembro", "dezembro"}

func DefaultTabPattern(title string) string {
	return strings.TrimSpace(title) + " · " + TokenMonthName + " " + TokenYear
}

func checkTabPattern(pattern string) error {
	invalid := func(format string, args ...any) error {
		return refuse(FieldTabPattern, CodeTabPatternInvalid).detail(format, args...)
	}
	stripped := pattern
	for _, token := range TabTokens() {
		stripped = strings.ReplaceAll(stripped, token, "")
	}
	switch {
	case strings.TrimSpace(pattern) == "":
		return invalid("o padrão do nome da aba está vazio")
	case strings.ContainsAny(stripped, "{}"):
		return invalid("use só %s, %s e %s", TokenMonthName, TokenMonth, TokenYear)
	case !strings.Contains(pattern, TokenYear):
		return invalid("inclua %s", TokenYear)
	case !strings.Contains(pattern, TokenMonthName) && !strings.Contains(pattern, TokenMonth):
		return invalid("inclua %s ou %s", TokenMonthName, TokenMonth)
	}
	longest := renderTabPattern(pattern, time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC))
	switch {
	case utf8.RuneCountInString(longest) > MaxTabRunes:
		return refuse(FieldTabPattern, CodeTabPatternInvalid).limit(MaxTabRunes).detail("o nome da aba passa de %d caracteres", MaxTabRunes)
	case strings.ContainsAny(longest, tabForbidden):
		return invalid("o nome da aba tem um caractere que o Google recusa")
	}
	return nil
}

func renderTabPattern(pattern string, month time.Time) string {
	return strings.NewReplacer(
		TokenMonthName, monthNames[month.Month()-1],
		TokenMonth, fmt.Sprintf("%02d", int(month.Month())),
		TokenYear, strconv.Itoa(month.Year()),
	).Replace(pattern)
}

func (s *Sheet) Periodic() bool { return s.TabPeriod == PeriodMonth }

func (s *Sheet) TabBaseName() string {
	if !s.Periodic() {
		return s.TabName
	}
	stripped := s.TabPattern
	for _, token := range TabTokens() {
		stripped = strings.ReplaceAll(stripped, token, " ")
	}
	var words []string
	for _, word := range strings.Fields(stripped) {
		if strings.IndexFunc(word, isWordRune) >= 0 {
			words = append(words, word)
		}
	}
	base := strings.TrimFunc(strings.Join(words, " "), func(r rune) bool { return !isWordRune(r) })
	if base == "" {
		return s.Title
	}
	return base
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

func (s *Sheet) PeriodOf(at time.Time) string {
	if !s.Periodic() {
		return ""
	}
	return at.In(s.Location()).Format(periodLayout)
}

func (s *Sheet) TabTitleFor(period string) (string, error) {
	if !s.Periodic() {
		return s.TabName, nil
	}
	month, err := time.Parse(periodLayout, period)
	if err != nil {
		return "", fmt.Errorf("googlesheet: period %q is not like 2026-11", period)
	}
	return renderTabPattern(s.TabPattern, month), nil
}

func (s *Sheet) OpenPeriod(now time.Time) error {
	if !s.Periodic() {
		return nil
	}
	period := s.PeriodOf(now)
	title, err := s.TabTitleFor(period)
	if err != nil {
		return err
	}
	s.TabName, s.TabPeriodKey = title, period
	return nil
}

func PreviewTabTitle(pattern, timeZone string, now time.Time) (title, period string, err error) {
	pattern = strings.TrimSpace(pattern)
	if err := checkTabPattern(pattern); err != nil {
		return "", "", err
	}
	if strings.TrimSpace(timeZone) == "" {
		timeZone = DefaultTimeZone
	}
	loc, err := time.LoadLocation(timeZone)
	if err != nil {
		return "", "", refuse(FieldTimeZone, CodeTimeZoneUnknown).detail("%q", timeZone)
	}
	local := now.In(loc)
	return renderTabPattern(pattern, local), local.Format(periodLayout), nil
}

type TabInfo struct {
	ID      int64
	Title   string
	Rows    int
	Columns int
}

type Workbook struct {
	Tabs []TabInfo
}

const MaxCellsPerSpreadsheet = 10_000_000

func (w Workbook) Tab(id int64) (TabInfo, bool) {
	for _, tab := range w.Tabs {
		if tab.ID == id {
			return tab, true
		}
	}
	return TabInfo{}, false
}

func (w Workbook) Titled(title string) (TabInfo, bool) {
	for _, tab := range w.Tabs {
		if strings.EqualFold(strings.TrimSpace(tab.Title), strings.TrimSpace(title)) {
			return tab, true
		}
	}
	return TabInfo{}, false
}

func (w Workbook) Cells() int {
	total := 0
	for _, tab := range w.Tabs {
		total += tab.Rows * tab.Columns
	}
	return total
}

func (w Workbook) NearlyFull(extraCells int) bool {
	return w.Cells()+extraCells >= MaxCellsPerSpreadsheet*95/100
}

func (w Workbook) FreeTabID(seed int64) int64 {
	id := seed & 0x3fffffff
	if id == 0 {
		id = 1
	}
	for {
		if _, taken := w.Tab(id); !taken {
			return id
		}
		id = id%0x3fffffff + 1
	}
}

type Tab struct {
	Period string
	ID     int64
	Title  string
}
