package scheduled_message

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"vozko/domain/shared"
)

type FollowUpField string

const (
	FieldWhenMode      FollowUpField = "when_mode"
	FieldDelayAmount   FollowUpField = "delay_amount"
	FieldDelayUnit     FollowUpField = "delay_unit"
	FieldDaysFromToday FollowUpField = "days_from_today"
	FieldDate          FollowUpField = "date"
	FieldTime          FollowUpField = "time"

	FollowUpModeIn   = "in"
	FollowUpModeDay  = "day"
	FollowUpModeDate = "date"

	maxFollowUpMinutes = 366 * 24 * 60
)

var (
	ErrFollowUpFieldInvalid = errors.New("follow-up: a scheduling field has an invalid value")
	ErrFollowUpWhenPast     = errors.New("follow-up: the date and time are already in the past")
)

var followUpFieldRules = map[FollowUpField]struct {
	label string
	rule  string
}{
	FieldWhenMode:      {"Quando", "precisa ser daqui a um tempo, daqui a alguns dias ou numa data"},
	FieldDelayAmount:   {"Quanto tempo", "precisa ser um número inteiro maior que zero"},
	FieldDelayUnit:     {"Unidade", "precisa ser minutes, hours ou days"},
	FieldDaysFromToday: {"Dias a partir de hoje", "precisa ser um número inteiro, 0 ou mais"},
	FieldDate:          {"Data", "precisa ser uma data válida, AAAA-MM-DD ou DD/MM/AAAA"},
	FieldTime:          {"Horário", "precisa estar no formato HH:MM, de 00:00 a 23:59"},
}

var FollowUpUnits = []string{"minutes", "hours", "days"}

var followUpUnitMinutes = map[string]int{"minutes": 1, "hours": 60, "days": 24 * 60}

var (
	wholeNumber = regexp.MustCompile(`^\d{1,9}$`)
)

func (f FollowUpField) Label() string {
	if rule, ok := followUpFieldRules[f]; ok {
		return rule.label
	}
	return string(f)
}

func (f FollowUpField) Rule() string {
	return followUpFieldRules[f].rule
}

type FollowUpFieldError struct {
	Field FollowUpField
	Value string
}

func (e *FollowUpFieldError) Error() string {
	return fmt.Sprintf("%v: %s %s", ErrFollowUpFieldInvalid, e.Field.Label(), e.Field.Rule())
}

func (e *FollowUpFieldError) Unwrap() error { return ErrFollowUpFieldInvalid }

func (e *FollowUpFieldError) Sentence() string {
	return e.Field.Label() + " " + e.Field.Rule() + "."
}

func FollowUpFieldsOf(mode string) []FollowUpField {
	switch mode {
	case FollowUpModeIn:
		return []FollowUpField{FieldDelayAmount, FieldDelayUnit}
	case FollowUpModeDay:
		return []FollowUpField{FieldDaysFromToday, FieldTime}
	case FollowUpModeDate:
		return []FollowUpField{FieldDate, FieldTime}
	}
	return nil
}

func CheckFollowUpField(field FollowUpField, text string) error {
	text = strings.TrimSpace(text)
	ok := false
	switch field {
	case FieldDelayAmount:
		n, whole := whole(text)
		ok = whole && n > 0 && n <= maxFollowUpMinutes
	case FieldDelayUnit:
		_, ok = followUpUnitMinutes[text]
	case FieldDaysFromToday:
		_, ok = whole(text)
	case FieldDate:
		_, ok = isoDate(text)
	case FieldTime:
		ok = shared.IsClockTime(text)
	case FieldWhenMode:
		ok = FollowUpFieldsOf(text) != nil
	}
	if !ok {
		return &FollowUpFieldError{Field: field, Value: text}
	}
	return nil
}

func ParseFollowUpWhen(mode string, values map[FollowUpField]string) (FollowUpWhen, error) {
	text := func(f FollowUpField) string { return strings.TrimSpace(values[f]) }
	switch mode {
	case FollowUpModeIn:
		amount, ok := whole(text(FieldDelayAmount))
		if !ok || amount <= 0 {
			return FollowUpWhen{}, invalid(FieldDelayAmount, values)
		}
		perUnit, known := followUpUnitMinutes[text(FieldDelayUnit)]
		if !known {
			return FollowUpWhen{}, invalid(FieldDelayUnit, values)
		}
		if amount > maxFollowUpMinutes/perUnit {
			return FollowUpWhen{}, invalid(FieldDelayAmount, values)
		}
		return FollowUpWhen{InMinutes: amount * perUnit}, nil
	case FollowUpModeDay:
		days, ok := whole(text(FieldDaysFromToday))
		if !ok {
			return FollowUpWhen{}, invalid(FieldDaysFromToday, values)
		}
		if !shared.IsClockTime(text(FieldTime)) {
			return FollowUpWhen{}, invalid(FieldTime, values)
		}
		return FollowUpWhen{DaysFromToday: &days, Time: text(FieldTime)}, nil
	case FollowUpModeDate:
		date, ok := isoDate(text(FieldDate))
		if !ok {
			return FollowUpWhen{}, invalid(FieldDate, values)
		}
		if !shared.IsClockTime(text(FieldTime)) {
			return FollowUpWhen{}, invalid(FieldTime, values)
		}
		return FollowUpWhen{Date: date, Time: text(FieldTime)}, nil
	}
	return FollowUpWhen{}, &FollowUpFieldError{Field: FieldWhenMode, Value: mode}
}

func invalid(field FollowUpField, values map[FollowUpField]string) error {
	return &FollowUpFieldError{Field: field, Value: values[field]}
}

func whole(text string) (int, bool) {
	if !wholeNumber.MatchString(text) {
		return 0, false
	}
	n, err := strconv.Atoi(text)
	return n, err == nil
}

func isoDate(text string) (string, bool) {
	for _, layout := range []string{"2006-01-02", "02/01/2006"} {
		if parsed, err := time.Parse(layout, text); err == nil {
			return parsed.Format("2006-01-02"), true
		}
	}
	return "", false
}
