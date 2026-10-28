package googlesheet

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Separator string

const (
	SeparatorComma     Separator = ","
	SeparatorSemicolon Separator = ";"
	MaxLinkRepairs               = 500
)

var ErrLocaleUnsupported = errors.New("googlesheet: the spreadsheet locale has no known formula argument separator")

var decimalCommaLocales = setOf(
	"pt", "pt_BR", "pt_PT",
	"de", "de_DE", "de_AT", "de_BE", "de_LU",
	"es", "es_ES", "es_AR", "es_BO", "es_CL", "es_CO", "es_CR", "es_EC", "es_PY", "es_UY", "es_VE",
	"fr", "fr_FR", "fr_BE", "fr_CA", "fr_LU",
	"it", "it_IT",
	"nl", "nl_NL", "nl_BE",
	"ca", "ca_ES", "eu", "eu_ES", "gl", "gl_ES",
	"da", "da_DK", "fi", "fi_FI", "is", "is_IS", "nb", "nb_NO", "nn", "nn_NO", "no", "no_NO", "sv", "sv_SE",
	"cs", "cs_CZ", "sk", "sk_SK", "pl", "pl_PL", "hu", "hu_HU", "ro", "ro_RO",
	"bg", "bg_BG", "hr", "hr_HR", "sl", "sl_SI", "sr", "sr_RS", "bs", "bs_BA", "mk", "mk_MK", "sq", "sq_AL", "el", "el_GR",
	"et", "et_EE", "lt", "lt_LT", "lv", "lv_LV",
	"ru", "ru_RU", "uk", "uk_UA", "be", "be_BY", "kk", "kk_KZ", "ka", "ka_GE", "hy", "hy_AM", "az", "az_AZ",
	"tr", "tr_TR", "id", "id_ID", "vi", "vi_VN",
)

var decimalPointLocales = setOf(
	"en", "en_US", "en_GB", "en_AU", "en_CA", "en_IE", "en_IN", "en_NZ", "en_PH", "en_SG",
	"es_MX", "es_US", "es_419",
	"de_CH", "it_CH",
	"ja", "ja_JP", "ko", "ko_KR", "zh", "zh_CN", "zh_TW", "zh_HK", "th", "th_TH",
	"he", "he_IL", "iw", "iw_IL", "hi", "hi_IN", "bn", "bn_BD", "ms", "ms_MY", "fil", "fil_PH", "tl", "tl_PH",
)

func setOf(items ...string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, item := range items {
		out[item] = true
	}
	return out
}

func FormulaSeparator(locale string) (Separator, error) {
	switch {
	case decimalCommaLocales[locale]:
		return SeparatorSemicolon, nil
	case decimalPointLocales[locale]:
		return SeparatorComma, nil
	}
	return "", fmt.Errorf("%w: %q", ErrLocaleUnsupported, locale)
}

func (s Separator) valid() bool {
	return s == SeparatorComma || s == SeparatorSemicolon
}

var linkFormulaPattern = regexp.MustCompile(`^=HYPERLINK\("#gid=(\d+)&range=B"&MATCH\("([0-9a-f-]{36})"([,;])'((?:[^']|'')+)'!\$A:\$A([,;])0\)([,;])"((?:[^"]|"")*)"\)$`)

func (f Formula) Relink(sep Separator) (Formula, bool) {
	match := linkFormulaPattern.FindStringSubmatch(string(f))
	if match == nil || !sep.valid() {
		return f, false
	}
	written := Separator(match[3])
	if Separator(match[5]) != written || Separator(match[6]) != written || written == sep {
		return f, false
	}
	id, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		return f, false
	}
	tab := TabRef{ID: id, Title: strings.ReplaceAll(match[4], "''", "'")}
	relinked, err := LinkFormula(tab, match[2], strings.ReplaceAll(match[7], `""`, `"`), sep)
	if err != nil {
		return f, false
	}
	return relinked, true
}

type LinkCell struct {
	Row     int
	Column  int
	RowRef  string
	Formula Formula
	Failed  bool
}

type FormulaCell struct {
	Row     int
	Column  int
	Formula Formula
}

func LinkRepairs(cells []LinkCell, sep Separator, budget int) ([]FormulaCell, bool) {
	var fixes []FormulaCell
	for _, cell := range cells {
		if !cell.Failed || !IsRowRef(cell.RowRef) {
			continue
		}
		relinked, changed := cell.Formula.Relink(sep)
		if !changed {
			continue
		}
		if len(fixes) == budget {
			return fixes, true
		}
		fixes = append(fixes, FormulaCell{Row: cell.Row, Column: cell.Column, Formula: relinked})
	}
	return fixes, false
}

func (s *Sheet) LinkGridColumns() []int {
	var out []int
	for i, col := range s.Columns {
		if !col.Archived && col.Source == SourceExportLink {
			out = append(out, i+1)
		}
	}
	return out
}

func ColumnBodyRange(index int) string {
	letter := columnLetter(index)
	return fmt.Sprintf("%s%d:%s", letter, firstBodyRow, letter)
}
