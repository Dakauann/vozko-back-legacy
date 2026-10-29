package perception

import (
	"errors"
	"math"
)

type Level string

const (
	LevelError   Level = "erro"
	LevelWarning Level = "aviso"
)

var ErrUnmeasured = errors.New("perception: a text element arrived without its measured layout")

type UnitPoint [2]float64

type Tone string

const (
	ToneOK  Tone = "ok"
	ToneBad Tone = "bad"
)

type Shape struct {
	Kind   string      `json:"kind"`
	From   *UnitPoint  `json:"from,omitempty"`
	To     *UnitPoint  `json:"to,omitempty"`
	Points []UnitPoint `json:"points,omitempty"`
	At     *UnitPoint  `json:"at,omitempty"`
	Tone   Tone        `json:"tone"`
}

type Finding struct {
	Kind  string   `json:"kind"`
	Level Level    `json:"level"`
	IDs   []string `json:"ids"`
	Text  string   `json:"text"`
}

func line(kind string, from, to UnitPoint, tone Tone) Shape {
	return Shape{Kind: kind, From: &from, To: &to, Tone: tone}
}

func findingLines(findings []Finding) []string {
	lines := make([]string, 0, len(findings))
	for _, level := range []Level{LevelError, LevelWarning} {
		for _, f := range findings {
			if f.Level == level {
				lines = append(lines, string(f.Level)+": "+f.Text)
			}
		}
	}
	return lines
}

var directions = []string{"para a direita", "para baixo e à direita", "para baixo", "para baixo e à esquerda", "para a esquerda", "para cima e à esquerda", "para cima", "para cima e à direita"}

func directionWord(angle float64) string {
	sector := int(math.Mod(jsRound(math.Mod(math.Mod(angle, 360)+360, 360)/45), 8))
	return directions[sector]
}
