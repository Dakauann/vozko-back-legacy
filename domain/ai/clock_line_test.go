package ai

import (
	"testing"
	"time"
)

func saoPaulo(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestTheClockLineNamesTheWeekdayDateTimeAndZone(t *testing.T) {
	at := time.Date(2026, 10, 26, 21, 40, 59, 0, time.UTC)

	got := ClockLine(at, saoPaulo(t))

	if want := "Agora: segunda-feira, 26/10/2026, 18:40 (America/Sao_Paulo)"; got != want {
		t.Fatalf("ClockLine = %q, want %q", got, want)
	}
}

func TestTheClockLineFallsBackToALabeledUTC(t *testing.T) {
	at := time.Date(2026, 10, 26, 21, 40, 0, 0, saoPaulo(t))

	got := ClockLine(at, nil)

	if want := "Agora: terça-feira, 27/10/2026, 00:40 (UTC)"; got != want {
		t.Fatalf("ClockLine = %q, want %q", got, want)
	}
}

func TestTheClockLineSpellsEveryWeekdayInPortuguese(t *testing.T) {
	want := []string{"domingo", "segunda-feira", "terça-feira", "quarta-feira", "quinta-feira", "sexta-feira", "sábado"}
	sunday := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	for offset, name := range want {
		got := ClockLine(sunday.AddDate(0, 0, offset), time.UTC)
		if wantLine := "Agora: " + name + ", " + sunday.AddDate(0, 0, offset).Format("02/01/2006") + ", 12:00 (UTC)"; got != wantLine {
			t.Errorf("ClockLine = %q, want %q", got, wantLine)
		}
	}
}
