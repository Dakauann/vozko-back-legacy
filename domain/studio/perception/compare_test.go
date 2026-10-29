package perception

import (
	"fmt"
	"math"
	"sort"
)

const coordinateTolerance = 1e-9

func differences(path string, want, have any, out *[]string) {
	if len(*out) >= 6 {
		return
	}
	switch w := want.(type) {
	case float64:
		h, ok := have.(float64)
		if !ok || math.Abs(w-h) > coordinateTolerance*math.Max(1, math.Abs(w)) {
			*out = append(*out, fmt.Sprintf("%s: want %v have %v", path, want, have))
		}
	case []any:
		h, ok := have.([]any)
		if !ok || len(h) != len(w) {
			*out = append(*out, fmt.Sprintf("%s: want %d items %v\n    have %v", path, len(w), want, have))
			return
		}
		for i := range w {
			differences(fmt.Sprintf("%s[%d]", path, i), w[i], h[i], out)
		}
	case map[string]any:
		h, ok := have.(map[string]any)
		if !ok {
			*out = append(*out, fmt.Sprintf("%s: want object have %v", path, have))
			return
		}
		keys := make([]string, 0, len(w)+len(h))
		for k := range w {
			keys = append(keys, k)
		}
		for k := range h {
			if _, seen := w[k]; !seen {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			differences(path+"."+k, w[k], h[k], out)
		}
	default:
		if want != have {
			*out = append(*out, fmt.Sprintf("%s: want %v\n    have %v", path, want, have))
		}
	}
}

func samePoint(a, b []any) bool {
	for i := range a {
		x, _ := a[i].(float64)
		y, _ := b[i].(float64)
		if math.Abs(x-y) > coordinateTolerance*math.Max(1, math.Abs(x)) {
			return false
		}
	}
	return true
}

func ring(points []any) []any {
	var kept []any
	for _, p := range points {
		point, _ := p.([]any)
		if len(kept) > 0 && samePoint(kept[len(kept)-1].([]any), point) {
			continue
		}
		kept = append(kept, point)
	}
	for len(kept) > 1 && samePoint(kept[0].([]any), kept[len(kept)-1].([]any)) {
		kept = kept[:len(kept)-1]
	}
	start := 0
	for i, p := range kept {
		a, b := p.([]any), kept[start].([]any)
		if a[0].(float64) < b[0].(float64)-coordinateTolerance || (math.Abs(a[0].(float64)-b[0].(float64)) <= coordinateTolerance && a[1].(float64) < b[1].(float64)) {
			start = i
		}
	}
	return append(append([]any(nil), kept[start:]...), kept[:start]...)
}

func normalizedAreas(value any) any {
	switch v := value.(type) {
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = normalizedAreas(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			out[k] = normalizedAreas(item)
		}
		if points, ok := out["points"].([]any); ok && v["kind"] == "area" {
			out["points"] = ring(points)
		}
		return out
	}
	return value
}
