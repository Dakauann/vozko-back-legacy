package perception

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

func jsRound(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	r := math.Floor(x)
	if x-r >= 0.5 {
		r++
	}
	if r == 0 {
		return 0
	}
	return r
}

func rounded(value float64, digits int) float64 {
	scale := math.Pow(10, float64(digits))
	return jsRound(value*scale) / scale
}

func jsHypot(values ...float64) float64 {
	largest := 0.0
	for _, v := range values {
		if a := math.Abs(v); a > largest {
			largest = a
		}
	}
	if largest == 0 {
		return 0
	}
	sum, compensation := 0.0, 0.0
	for _, v := range values {
		n := math.Abs(v) / largest
		summand := n*n - compensation
		preliminary := sum + summand
		compensation = (preliminary - sum) - summand
		sum = preliminary
	}
	return math.Sqrt(sum) * largest
}

func num(x float64) string {
	switch {
	case math.IsNaN(x):
		return "NaN"
	case math.IsInf(x, 1):
		return "Infinity"
	case math.IsInf(x, -1):
		return "-Infinity"
	case x == 0:
		return "0"
	}
	abs := math.Abs(x)
	if abs >= 1e-6 && abs < 1e21 {
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	text := strconv.FormatFloat(x, 'e', -1, 64)
	mantissa, exponent, _ := strings.Cut(text, "e")
	sign := exponent[0]
	digits := strings.TrimLeft(exponent[1:], "0")
	if digits == "" {
		digits = "0"
	}
	return mantissa + "e" + string(sign) + digits
}

func pxNum(x float64) string {
	return num(jsRound(x))
}

func jsSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0x00a0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

func jsTrim(s string) string {
	return strings.TrimFunc(s, jsSpace)
}

func jsWords(s string) []string {
	return strings.FieldsFunc(s, jsSpace)
}

func jsCollapse(s string) string {
	var b strings.Builder
	inSpace := false
	for _, r := range s {
		if jsSpace(r) {
			if !inSpace {
				b.WriteByte(' ')
			}
			inSpace = true
			continue
		}
		inSpace = false
		b.WriteRune(r)
	}
	return b.String()
}

func jsLength(s string) int {
	return len(utf16.Encode([]rune(s)))
}

func jsHead(s string, units int) string {
	encoded := utf16.Encode([]rune(s))
	if units >= len(encoded) {
		return s
	}
	return string(utf16.Decode(encoded[:units]))
}

func stableSort[T any](items []T, less func(a, b T) bool) {
	sort.SliceStable(items, func(i, j int) bool { return less(items[i], items[j]) })
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func joinIDs(ids []string) string {
	return strings.Join(ids, ", ")
}
