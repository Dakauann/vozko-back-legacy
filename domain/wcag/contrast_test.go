package wcag

import (
	"math"
	"testing"
)

func TestContrastRatioFollowsTheWCAGFormula(t *testing.T) {
	cases := []struct {
		name   string
		fg, bg [3]uint8
		want   float64
	}{
		{"black on white", [3]uint8{0, 0, 0}, [3]uint8{255, 255, 255}, 21},
		{"same colour", [3]uint8{120, 30, 200}, [3]uint8{120, 30, 200}, 1},
		{"order does not matter", [3]uint8{255, 255, 255}, [3]uint8{0, 0, 0}, 21},
		{"mid grey on white", [3]uint8{0x76, 0x76, 0x76}, [3]uint8{255, 255, 255}, 4.54},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ContrastRatio(c.fg, c.bg)
			if math.Abs(got-c.want) > 0.01 {
				t.Fatalf("ratio %.3f, want %.2f", got, c.want)
			}
		})
	}
}

func TestMinimumDependsOnTextSize(t *testing.T) {
	cases := []struct {
		px     float64
		weight int
		want   float64
	}{
		{16, 400, NormalTextMinimum},
		{24, 400, LargeTextMinimum},
		{18.7, 700, LargeTextMinimum},
		{18.7, 600, NormalTextMinimum},
		{14, 900, NormalTextMinimum},
	}
	for _, c := range cases {
		if got := Minimum(c.px, c.weight); got != c.want {
			t.Fatalf("Minimum(%v, %d) = %v, want %v", c.px, c.weight, got, c.want)
		}
	}
}
