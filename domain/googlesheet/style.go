package googlesheet

type StylePreset string

const (
	StylePlain    StylePreset = "plain"
	StyleGraphite StylePreset = "graphite"
	StyleBlue     StylePreset = "blue"
	StyleGreen    StylePreset = "green"
	StylePurple   StylePreset = "purple"
	DefaultStyle              = StyleGraphite
)

type Palette struct {
	Header     string
	HeaderText string
	Band       string
}

var palettes = map[StylePreset]Palette{
	StylePlain:    {},
	StyleGraphite: {Header: "#37414B", HeaderText: "#FFFFFF", Band: "#F4F5F7"},
	StyleBlue:     {Header: "#1F5FBF", HeaderText: "#FFFFFF", Band: "#F2F6FC"},
	StyleGreen:    {Header: "#1E7A4C", HeaderText: "#FFFFFF", Band: "#F2F8F4"},
	StylePurple:   {Header: "#6A3FA0", HeaderText: "#FFFFFF", Band: "#F6F3FA"},
}

func StylePresets() []StylePreset {
	return []StylePreset{StylePlain, StyleGraphite, StyleBlue, StyleGreen, StylePurple}
}

func (p StylePreset) Valid() bool {
	_, ok := palettes[p]
	return ok
}

func (p StylePreset) Palette() Palette { return palettes[p] }

type OptionColor string

const (
	ColorGreen  OptionColor = "green"
	ColorRed    OptionColor = "red"
	ColorAmber  OptionColor = "amber"
	ColorBlue   OptionColor = "blue"
	ColorPurple OptionColor = "purple"
)

var optionColors = map[OptionColor][2]string{
	ColorGreen:  {"#1E8E3E", "#FFFFFF"},
	ColorRed:    {"#C5221F", "#FFFFFF"},
	ColorAmber:  {"#F9AB00", "#202124"},
	ColorBlue:   {"#1A73E8", "#FFFFFF"},
	ColorPurple: {"#8E24AA", "#FFFFFF"},
}

func OptionColors() []OptionColor {
	return []OptionColor{ColorGreen, ColorRed, ColorAmber, ColorBlue, ColorPurple}
}

func (c OptionColor) Valid() bool {
	_, ok := optionColors[c]
	return ok
}

func (c OptionColor) Hex() (fill, text string) {
	pair := optionColors[c]
	return pair[0], pair[1]
}

type Layout struct {
	Columns  []Column
	Style    StylePreset
	SizeFrom int
}

func (s *Sheet) Layout(sizeFrom int) Layout {
	return Layout{Columns: s.Columns, Style: s.Style, SizeFrom: sizeFrom}
}

const NewTabRows = 100
