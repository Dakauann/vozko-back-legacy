package scheduled_message

import (
	"errors"
	"testing"
	"time"
)

func TestParseFollowUpWhenTable(t *testing.T) {
	two := 2
	zero := 0
	cases := []struct {
		name   string
		mode   string
		values map[FollowUpField]string
		want   FollowUpWhen
		field  FollowUpField
	}{
		{"minutes", "in", map[FollowUpField]string{FieldDelayAmount: "4", FieldDelayUnit: "minutes"}, FollowUpWhen{InMinutes: 4}, ""},
		{"hours", "in", map[FollowUpField]string{FieldDelayAmount: " 3 ", FieldDelayUnit: "hours"}, FollowUpWhen{InMinutes: 180}, ""},
		{"days", "in", map[FollowUpField]string{FieldDelayAmount: "2", FieldDelayUnit: "days"}, FollowUpWhen{InMinutes: 2880}, ""},
		{"zero amount", "in", map[FollowUpField]string{FieldDelayAmount: "0", FieldDelayUnit: "hours"}, FollowUpWhen{}, FieldDelayAmount},
		{"negative amount", "in", map[FollowUpField]string{FieldDelayAmount: "-1", FieldDelayUnit: "hours"}, FollowUpWhen{}, FieldDelayAmount},
		{"fraction amount", "in", map[FollowUpField]string{FieldDelayAmount: "1.5", FieldDelayUnit: "hours"}, FollowUpWhen{}, FieldDelayAmount},
		{"word amount", "in", map[FollowUpField]string{FieldDelayAmount: "quatro", FieldDelayUnit: "hours"}, FollowUpWhen{}, FieldDelayAmount},
		{"unresolved token amount", "in", map[FollowUpField]string{FieldDelayAmount: "{{node.n1.tool_args.amount}}", FieldDelayUnit: "hours"}, FollowUpWhen{}, FieldDelayAmount},
		{"empty amount", "in", map[FollowUpField]string{FieldDelayAmount: "", FieldDelayUnit: "hours"}, FollowUpWhen{}, FieldDelayAmount},
		{"huge amount", "in", map[FollowUpField]string{FieldDelayAmount: "99999999999", FieldDelayUnit: "days"}, FollowUpWhen{}, FieldDelayAmount},
		{"unknown unit", "in", map[FollowUpField]string{FieldDelayAmount: "4", FieldDelayUnit: "weeks"}, FollowUpWhen{}, FieldDelayUnit},
		{"empty unit", "in", map[FollowUpField]string{FieldDelayAmount: "4", FieldDelayUnit: ""}, FollowUpWhen{}, FieldDelayUnit},
		{"tomorrow", "day", map[FollowUpField]string{FieldDaysFromToday: "2", FieldTime: "17:00"}, FollowUpWhen{DaysFromToday: &two, Time: "17:00"}, ""},
		{"today", "day", map[FollowUpField]string{FieldDaysFromToday: "0", FieldTime: "09:30"}, FollowUpWhen{DaysFromToday: &zero, Time: "09:30"}, ""},
		{"days negative", "day", map[FollowUpField]string{FieldDaysFromToday: "-1", FieldTime: "09:30"}, FollowUpWhen{}, FieldDaysFromToday},
		{"days empty", "day", map[FollowUpField]string{FieldDaysFromToday: "", FieldTime: "09:30"}, FollowUpWhen{}, FieldDaysFromToday},
		{"days word", "day", map[FollowUpField]string{FieldDaysFromToday: "amanhã", FieldTime: "09:30"}, FollowUpWhen{}, FieldDaysFromToday},
		{"bad hour", "day", map[FollowUpField]string{FieldDaysFromToday: "1", FieldTime: "24:00"}, FollowUpWhen{}, FieldTime},
		{"bad minute", "day", map[FollowUpField]string{FieldDaysFromToday: "1", FieldTime: "10:60"}, FollowUpWhen{}, FieldTime},
		{"short time", "day", map[FollowUpField]string{FieldDaysFromToday: "1", FieldTime: "9:00"}, FollowUpWhen{}, FieldTime},
		{"date iso", "date", map[FollowUpField]string{FieldDate: "2026-10-30", FieldTime: "10:00"}, FollowUpWhen{Date: "2026-10-30", Time: "10:00"}, ""},
		{"date br", "date", map[FollowUpField]string{FieldDate: "30/10/2026", FieldTime: "10:00"}, FollowUpWhen{Date: "2026-10-30", Time: "10:00"}, ""},
		{"date invalid", "date", map[FollowUpField]string{FieldDate: "31/02/2026", FieldTime: "10:00"}, FollowUpWhen{}, FieldDate},
		{"date empty", "date", map[FollowUpField]string{FieldDate: "", FieldTime: "10:00"}, FollowUpWhen{}, FieldDate},
		{"date time missing", "date", map[FollowUpField]string{FieldDate: "2026-10-30", FieldTime: ""}, FollowUpWhen{}, FieldTime},
		{"unknown mode", "later", nil, FollowUpWhen{}, FieldWhenMode},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseFollowUpWhen(tc.mode, tc.values)
			if tc.field == "" {
				if err != nil {
					t.Fatal(err)
				}
				if got.InMinutes != tc.want.InMinutes || got.Date != tc.want.Date || got.Time != tc.want.Time || (got.DaysFromToday == nil) != (tc.want.DaysFromToday == nil) || (got.DaysFromToday != nil && *got.DaysFromToday != *tc.want.DaysFromToday) {
					t.Fatalf("when = %+v, want %+v", got, tc.want)
				}
				return
			}
			var fieldErr *FollowUpFieldError
			if !errors.Is(err, ErrFollowUpFieldInvalid) || !errors.As(err, &fieldErr) || fieldErr.Field != tc.field {
				t.Fatalf("err = %v, want field %s", err, tc.field)
			}
			if fieldErr.Field.Label() == "" || fieldErr.Field.Label() == string(fieldErr.Field) {
				t.Fatalf("label = %q", fieldErr.Field.Label())
			}
			if FollowUpErrorCode(err) != "follow_up_field_invalid" {
				t.Fatalf("code = %q", FollowUpErrorCode(err))
			}
		})
	}
}

func TestFollowUpFieldsOfEachMode(t *testing.T) {
	if got := FollowUpFieldsOf("in"); len(got) != 2 || got[0] != FieldDelayAmount || got[1] != FieldDelayUnit {
		t.Fatalf("in = %v", got)
	}
	if got := FollowUpFieldsOf("day"); len(got) != 2 || got[0] != FieldDaysFromToday || got[1] != FieldTime {
		t.Fatalf("day = %v", got)
	}
	if got := FollowUpFieldsOf("date"); len(got) != 2 || got[0] != FieldDate || got[1] != FieldTime {
		t.Fatalf("date = %v", got)
	}
}

func TestADateInThePastIsRefused(t *testing.T) {
	loc := time.FixedZone("BRT", -3*3600)
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, loc)
	if _, err := (FollowUpWhen{Date: "2026-10-27", Time: "10:00"}).Resolve(now, loc); !errors.Is(err, ErrFollowUpWhenPast) || FollowUpErrorCode(err) != "follow_up_when_past" {
		t.Fatalf("err = %v", err)
	}
	if _, err := (FollowUpWhen{Date: "2026-10-28", Time: "11:59"}).Resolve(now, loc); !errors.Is(err, ErrFollowUpWhenPast) {
		t.Fatalf("earlier today err = %v", err)
	}
	at, err := (FollowUpWhen{Date: "2026-10-30", Time: "10:00"}).Resolve(now, loc)
	if err != nil || at.Day() != 30 || at.Hour() != 10 {
		t.Fatalf("at = %v, err = %v", at, err)
	}
}

func TestCheckFollowUpFieldIsTheSameRuleFieldByField(t *testing.T) {
	ok := map[FollowUpField]string{FieldDelayAmount: "4", FieldDelayUnit: "days", FieldDaysFromToday: "0", FieldDate: "30/10/2026", FieldTime: "23:59", FieldWhenMode: "date"}
	bad := map[FollowUpField]string{FieldDelayAmount: "0", FieldDelayUnit: "anos", FieldDaysFromToday: "-2", FieldDate: "2026-02-30", FieldTime: "7h", FieldWhenMode: "agora"}
	for field, text := range ok {
		if err := CheckFollowUpField(field, text); err != nil {
			t.Errorf("%s %q: %v", field, text, err)
		}
	}
	for field, text := range bad {
		if err := CheckFollowUpField(field, text); !errors.Is(err, ErrFollowUpFieldInvalid) {
			t.Errorf("%s %q: err = %v", field, text, err)
		}
	}
}
