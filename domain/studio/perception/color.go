package perception

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
)

type Rgba struct {
	R, G, B, A float64
}

var hexColor = regexp.MustCompile(`^#([0-9a-fA-F]{6})([0-9a-fA-F]{2})?$`)

func parseColor(value string) *Rgba {
	match := hexColor.FindStringSubmatch(value)
	if match == nil {
		return nil
	}
	n, _ := strconv.ParseUint(match[1], 16, 32)
	color := Rgba{R: float64((n >> 16) & 255), G: float64((n >> 8) & 255), B: float64(n & 255), A: 1}
	if match[2] != "" {
		alpha, _ := strconv.ParseUint(match[2], 16, 32)
		color.A = float64(alpha) / 255
	}
	return &color
}

func clampUnit(v float64) float64 {
	return math.Max(0, math.Min(1, v))
}

func mixColors(base, over Rgba, amount float64) Rgba {
	t := clampUnit(amount)
	channel := func(from, to float64) float64 { return jsRound(from + (to-from)*t) }
	return Rgba{R: channel(base.R, over.R), G: channel(base.G, over.G), B: channel(base.B, over.B), A: 1}
}

func linearChannel(channel float64) float64 {
	c := channel / 255
	if c <= 0.03928 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

func luminance(c Rgba) float64 {
	return 0.2126*linearChannel(c.R) + 0.7152*linearChannel(c.G) + 0.0722*linearChannel(c.B)
}

func contrastRatio(a, b Rgba) float64 {
	pair := []float64{luminance(a), luminance(b)}
	sort.Float64s(pair)
	return (pair[1] + 0.05) / (pair[0] + 0.05)
}

func hexOf(c Rgba) string {
	part := func(v float64) int { return int(jsRound(math.Min(255, math.Max(0, v)))) }
	return fmt.Sprintf("#%02x%02x%02x", part(c.R), part(c.G), part(c.B))
}
