package variabledecl

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	KindText   Kind = "text"
	KindNumber Kind = "number"
	KindDate   Kind = "date"
	KindChoice Kind = "choice"

	MaxOptions     = 50
	MaxOptionRunes = 100
)

var numberPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^-?\d+([.,]\d+)?$`),
	regexp.MustCompile(`^-?\d{1,3}(\.\d{3})+(,\d+)?$`),
	regexp.MustCompile(`^-?\d{1,3}(,\d{3})+(\.\d+)?$`),
}

var dateLayouts = []string{"02/01/2006", "2006-01-02"}

func Builtin() []Rule {
	return []Rule{textRule{}, numberRule{}, dateRule{}, choiceRule{}}
}

func plainType(t Type) error {
	if len(t.Options) > 0 || len(t.Settings) > 0 {
		return fmt.Errorf("%w: %s takes no options", ErrTypeInvalid, t.Kind)
	}
	return nil
}

func refuse(c Check) error {
	return fmt.Errorf("%w: %s is not a %s", ErrValueInvalid, c.Name, c.Type.Kind)
}

func textValue(value string) (string, bool) {
	value = strings.TrimSpace(value)
	n := utf8.RuneCountInString(value)
	return value, n > 0 && n <= MaxValueRunes
}

type textRule struct{}

func (textRule) Kind() Kind             { return KindText }
func (textRule) Shape() Shape           { return ShapeText }
func (textRule) Sample(Type) string     { return "Texto de exemplo" }
func (textRule) CheckType(t Type) error { return plainType(t) }

func (textRule) Accepts(_ context.Context, c Check) error {
	if _, ok := textValue(c.Value); !ok {
		return refuse(c)
	}
	return nil
}

type numberRule struct{}

func (numberRule) Kind() Kind             { return KindNumber }
func (numberRule) Shape() Shape           { return ShapeNumber }
func (numberRule) Sample(Type) string     { return "42" }
func (numberRule) CheckType(t Type) error { return plainType(t) }

func (numberRule) Accepts(_ context.Context, c Check) error {
	value := strings.TrimSpace(c.Value)
	for _, pattern := range numberPatterns {
		if pattern.MatchString(value) {
			return nil
		}
	}
	return refuse(c)
}

type dateRule struct{}

func (dateRule) Kind() Kind             { return KindDate }
func (dateRule) Shape() Shape           { return ShapeDate }
func (dateRule) Sample(Type) string     { return "28/10/2026" }
func (dateRule) CheckType(t Type) error { return plainType(t) }

func (dateRule) Accepts(_ context.Context, c Check) error {
	value := strings.TrimSpace(c.Value)
	for _, layout := range dateLayouts {
		if _, err := time.Parse(layout, value); err == nil {
			return nil
		}
	}
	return refuse(c)
}

type choiceRule struct{}

func (choiceRule) Kind() Kind   { return KindChoice }
func (choiceRule) Shape() Shape { return ShapeChoice }

func (choiceRule) Sample(t Type) string {
	if len(t.Options) == 0 {
		return ""
	}
	return t.Options[0]
}

func (choiceRule) CheckType(t Type) error {
	if len(t.Settings) > 0 {
		return fmt.Errorf("%w: choice takes options only", ErrTypeInvalid)
	}
	if len(t.Options) == 0 || len(t.Options) > MaxOptions {
		return fmt.Errorf("%w: a choice has 1 to %d options", ErrTypeInvalid, MaxOptions)
	}
	seen := make(map[string]bool, len(t.Options))
	for _, option := range t.Options {
		if n := utf8.RuneCountInString(option); n == 0 || n > MaxOptionRunes {
			return fmt.Errorf("%w: an option has 1 to %d characters", ErrTypeInvalid, MaxOptionRunes)
		}
		if seen[option] {
			return fmt.Errorf("%w: option %q repeats", ErrTypeInvalid, option)
		}
		seen[option] = true
	}
	return nil
}

func (choiceRule) Accepts(_ context.Context, c Check) error {
	value := strings.TrimSpace(c.Value)
	for _, option := range c.Type.Options {
		if option == value {
			return nil
		}
	}
	return refuse(c)
}
