package googlesheet

import (
	"regexp"
	"testing"
)

var hexColor = regexp.MustCompile(`^#[0-9A-F]{6}$`)

func TestEveryStyleAndOptionColourHasSolidHexColours(t *testing.T) {
	for _, style := range StylePresets() {
		p := style.Palette()
		if style == StylePlain {
			if p.Header != "" || p.Band != "" {
				t.Fatalf("plain has colours: %+v", p)
			}
			continue
		}
		for _, c := range []string{p.Header, p.HeaderText, p.Band} {
			if !hexColor.MatchString(c) {
				t.Fatalf("%s palette colour %q is not #RRGGBB", style, c)
			}
		}
	}
	for _, color := range OptionColors() {
		fill, text := color.Hex()
		if !hexColor.MatchString(fill) || !hexColor.MatchString(text) {
			t.Fatalf("%s = %q on %q", color, text, fill)
		}
	}
}

func TestNewSheetsGetTheDefaultStyle(t *testing.T) {
	s, _ := NewSheet(validInput())
	if s.Style != DefaultStyle || DefaultStyle == StylePlain {
		t.Fatalf("style = %q", s.Style)
	}
	in := validInput()
	in.Style = StylePlain
	s, _ = NewSheet(in)
	if s.Style != StylePlain {
		t.Fatalf("explicit plain became %q", s.Style)
	}
}

func TestOptionColoursFollowRenamesOfNothingAndCanBeChanged(t *testing.T) {
	s, _ := NewSheet(paymentsInput())
	err := s.EditColumns(ColumnsEdit{Update: []ColumnUpdate{{Key: "baixa", Colors: map[string]OptionColor{"SIM": ColorBlue}}}})
	if err != nil {
		t.Fatalf("colour edit: %v", err)
	}
	baixa, _ := s.Column("baixa")
	if len(baixa.Colors) != 1 || baixa.Colors["SIM"] != ColorBlue {
		t.Fatalf("colors = %v", baixa.Colors)
	}
	err = s.EditColumns(ColumnsEdit{Update: []ColumnUpdate{{Key: "baixa", Colors: map[string]OptionColor{"TALVEZ": ColorBlue}}}})
	if ErrorCode(err) != CodeColorNotOption {
		t.Fatalf("colour of no option = %v", err)
	}
	label := "Abrir conversa"
	if err := s.EditColumns(ColumnsEdit{Update: []ColumnUpdate{{Key: "negociacao", LinkLabel: &label}}}); err != nil {
		t.Fatalf("label edit: %v", err)
	}
	link, _ := s.Column("negociacao")
	if link.LinkLabel != label {
		t.Fatalf("label = %q", link.LinkLabel)
	}
	if err := s.EditColumns(ColumnsEdit{Update: []ColumnUpdate{{Key: "valor", LinkLabel: &label}}}); ErrorCode(err) != CodeLinkFieldsNotLink {
		t.Fatalf("label on money = %v", err)
	}
}

func TestLayoutCarriesStyleAndWhereSizingStarts(t *testing.T) {
	s, _ := NewSheet(paymentsInput())
	layout := s.Layout(2)
	if layout.Style != s.Style || len(layout.Columns) != 3 || layout.SizeFrom != 2 {
		t.Fatalf("layout = %+v", layout)
	}
}
