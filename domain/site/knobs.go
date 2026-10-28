package site

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"vozko/domain/strictjson"
)

type KnobKind string

const (
	KnobRange KnobKind = "range"
	KnobColor KnobKind = "color"

	maxKnobNameRunes = 60
)

var (
	ErrKnobsInvalid     = errors.New("site: knobs.json is invalid")
	ErrKnobValueInvalid = errors.New("site: the knob value is outside its schema")
)

var (
	cssVariable = regexp.MustCompile(`^--[a-z0-9]+(-[a-z0-9]+)*$`)
	hexColor    = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
	fnColor     = regexp.MustCompile(`^(oklch|oklab|rgb|rgba|hsl|hsla|lab|lch)\([0-9a-z.%/ ,+-]+\)$`)
)

var knobUnits = map[string]bool{"": true, "px": true, "rem": true, "em": true, "%": true, "vh": true, "vw": true, "deg": true, "ms": true, "s": true}

type Knob struct {
	Name     string   `json:"name"`
	Variable string   `json:"variable"`
	Kind     KnobKind `json:"kind"`
	Min      *float64 `json:"min,omitempty"`
	Max      *float64 `json:"max,omitempty"`
	Step     *float64 `json:"step,omitempty"`
	Unit     string   `json:"unit,omitempty"`
}

type Knobs struct {
	Knobs []Knob `json:"knobs"`
}

type KnobValue struct {
	Number *float64
	Text   *string
}

func knobsError(reason string) error {
	return fmt.Errorf("%w: %s", ErrKnobsInvalid, reason)
}

func DecodeKnobs(raw []byte) (Knobs, error) {
	var k Knobs
	if err := strictjson.Decode(raw, &k); err != nil {
		return Knobs{}, fmt.Errorf("%w: %w", ErrKnobsInvalid, err)
	}
	if k.Knobs == nil {
		return Knobs{}, knobsError("the knobs list is required")
	}
	names, variables := map[string]bool{}, map[string]bool{}
	for _, knob := range k.Knobs {
		if err := knob.validate(); err != nil {
			return Knobs{}, err
		}
		if names[knob.Name] || variables[knob.Variable] {
			return Knobs{}, knobsError("knob " + knob.Name + " repeats a name or variable")
		}
		names[knob.Name], variables[knob.Variable] = true, true
	}
	return k, nil
}

func (k Knob) validate() error {
	if _, reason := k.problem(); reason != "" {
		return knobsError(reason)
	}
	return nil
}

func (k Knob) problem() (string, string) {
	name := strings.TrimSpace(k.Name)
	if name == "" || name != k.Name || utf8.RuneCountInString(name) > maxKnobNameRunes {
		return "name", "a knob name must be 1 to 60 characters"
	}
	if !cssVariable.MatchString(k.Variable) {
		return "variable", "knob " + k.Name + " must bind a css variable like --hero-gap"
	}
	switch k.Kind {
	case KnobRange:
		switch {
		case k.Min == nil:
			return "min", "range knob " + k.Name + " needs min below max and a positive step"
		case k.Max == nil || *k.Min >= *k.Max:
			return "max", "range knob " + k.Name + " needs min below max and a positive step"
		case k.Step == nil || *k.Step <= 0:
			return "step", "range knob " + k.Name + " needs min below max and a positive step"
		case !knobUnits[k.Unit]:
			return "unit", "range knob " + k.Name + " has an unknown unit"
		}
	case KnobColor:
		if k.Min != nil || k.Max != nil || k.Step != nil || k.Unit != "" {
			return "kind", "color knob " + k.Name + " takes no min, max, step or unit"
		}
	default:
		return "kind", "knob " + k.Name + " kind must be range or color"
	}
	return "", ""
}

func (k Knobs) Encode() (string, error) {
	list := k.Knobs
	if list == nil {
		list = []Knob{}
	}
	raw, err := json.MarshalIndent(Knobs{Knobs: list}, "", "  ")
	if err != nil {
		return "", err
	}
	return string(raw) + "\n", nil
}

func (k Knobs) Find(name string) (Knob, bool) {
	for _, knob := range k.Knobs {
		if knob.Name == name {
			return knob, true
		}
	}
	return Knob{}, false
}

func (k Knob) CSSValue(v KnobValue) (string, error) {
	switch k.Kind {
	case KnobRange:
		if v.Number == nil || v.Text != nil {
			return "", ErrKnobValueInvalid
		}
		n := *v.Number
		steps := (n - *k.Min) / *k.Step
		if n < *k.Min || n > *k.Max || math.Abs(steps-math.Round(steps)) > 1e-9 {
			return "", ErrKnobValueInvalid
		}
		return strconv.FormatFloat(n, 'f', -1, 64) + k.Unit, nil
	case KnobColor:
		if v.Text == nil || v.Number != nil || !ValidColor(*v.Text) {
			return "", ErrKnobValueInvalid
		}
		return *v.Text, nil
	}
	return "", ErrKnobValueInvalid
}

func ValidColor(value string) bool {
	return hexColor.MatchString(value) || fnColor.MatchString(value)
}
