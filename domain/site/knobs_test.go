package site

import (
	"errors"
	"testing"
)

func TestDecodeKnobsReadsRangeAndColorKnobs(t *testing.T) {
	knobs, err := DecodeKnobs([]byte(htmlProject().Files["knobs.json"].Content))
	if err != nil {
		t.Fatal(err)
	}
	if len(knobs.Knobs) != 2 {
		t.Fatalf("got %+v", knobs)
	}
	gap := knobs.Knobs[0]
	if gap.Name != "Espaço do topo" || gap.Variable != "--hero-gap" || gap.Kind != KnobRange || *gap.Min != 0 || *gap.Max != 8 || *gap.Step != 0.5 || gap.Unit != "rem" {
		t.Fatalf("got %+v", gap)
	}
	if found, ok := knobs.Find("Cor da marca"); !ok || found.Kind != KnobColor {
		t.Fatalf("got %+v %v", found, ok)
	}
}

func TestDecodeKnobsRefusesBrokenSchemas(t *testing.T) {
	cases := map[string]string{
		"unknown field":     `{"knobs":[{"name":"a","variable":"--a","kind":"color","default":"#fff"}]}`,
		"unknown kind":      `{"knobs":[{"name":"a","variable":"--a","kind":"toggle"}]}`,
		"no name":           `{"knobs":[{"name":" ","variable":"--a","kind":"color"}]}`,
		"bad variable":      `{"knobs":[{"name":"a","variable":"hero-gap","kind":"color"}]}`,
		"repeated name":     `{"knobs":[{"name":"a","variable":"--a","kind":"color"},{"name":"a","variable":"--b","kind":"color"}]}`,
		"repeated variable": `{"knobs":[{"name":"a","variable":"--a","kind":"color"},{"name":"b","variable":"--a","kind":"color"}]}`,
		"range without max": `{"knobs":[{"name":"a","variable":"--a","kind":"range","min":0,"step":1}]}`,
		"range inverted":    `{"knobs":[{"name":"a","variable":"--a","kind":"range","min":5,"max":1,"step":1}]}`,
		"range zero step":   `{"knobs":[{"name":"a","variable":"--a","kind":"range","min":0,"max":1,"step":0}]}`,
		"range bad unit":    `{"knobs":[{"name":"a","variable":"--a","kind":"range","min":0,"max":1,"step":1,"unit":"parsecs"}]}`,
		"color with range":  `{"knobs":[{"name":"a","variable":"--a","kind":"color","min":0}]}`,
		"color with unit":   `{"knobs":[{"name":"a","variable":"--a","kind":"color","unit":"px"}]}`,
		"no knobs key":      `{}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeKnobs([]byte(raw)); !errors.Is(err, ErrKnobsInvalid) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestKnobValuesStayInsideTheirSchema(t *testing.T) {
	knobs, err := DecodeKnobs([]byte(htmlProject().Files["knobs.json"].Content))
	if err != nil {
		t.Fatal(err)
	}
	gap, color := knobs.Knobs[0], knobs.Knobs[1]
	number := func(v float64) KnobValue { return KnobValue{Number: &v} }
	text := func(v string) KnobValue { return KnobValue{Text: &v} }
	accepted := []struct {
		knob  Knob
		value KnobValue
		css   string
	}{
		{gap, number(0), "0rem"},
		{gap, number(2.5), "2.5rem"},
		{gap, number(8), "8rem"},
		{color, text("#1a2b3c"), "#1a2b3c"},
		{color, text("#fff"), "#fff"},
		{color, text("oklch(0.62 0.19 250)"), "oklch(0.62 0.19 250)"},
		{color, text("rgb(10 20 30 / 50%)"), "rgb(10 20 30 / 50%)"},
	}
	for _, c := range accepted {
		css, err := c.knob.CSSValue(c.value)
		if err != nil || css != c.css {
			t.Fatalf("%s: got %q %v", c.knob.Name, css, err)
		}
	}
	refused := []struct {
		knob  Knob
		value KnobValue
	}{
		{gap, number(-1)},
		{gap, number(8.5)},
		{gap, number(0.3)},
		{gap, text("2rem")},
		{gap, KnobValue{}},
		{color, number(1)},
		{color, text("red; background: url(x)")},
		{color, text("#12")},
		{color, text("url(https://x)")},
	}
	for _, c := range refused {
		if _, err := c.knob.CSSValue(c.value); !errors.Is(err, ErrKnobValueInvalid) {
			t.Fatalf("%s %+v: got %v", c.knob.Name, c.value, err)
		}
	}
}
