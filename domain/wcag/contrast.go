package wcag

import "math"

const (
	NormalTextMinimum = 4.5
	LargeTextMinimum  = 3.0

	largeTextPx     = 24
	largeBoldTextPx = 18.66
	boldWeight      = 700
)

func channel(value uint8) float64 {
	c := float64(value) / 255
	if c <= 0.03928 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

func RelativeLuminance(r, g, b uint8) float64 {
	return 0.2126*channel(r) + 0.7152*channel(g) + 0.0722*channel(b)
}

func Ratio(a, b float64) float64 {
	light, dark := math.Max(a, b), math.Min(a, b)
	return (light + 0.05) / (dark + 0.05)
}

func ContrastRatio(foreground, background [3]uint8) float64 {
	return Ratio(
		RelativeLuminance(foreground[0], foreground[1], foreground[2]),
		RelativeLuminance(background[0], background[1], background[2]),
	)
}

func LargeText(px float64, weight int) bool {
	return px >= largeTextPx || (px >= largeBoldTextPx && weight >= boldWeight)
}

func Minimum(px float64, weight int) float64 {
	if LargeText(px, weight) {
		return LargeTextMinimum
	}
	return NormalTextMinimum
}
