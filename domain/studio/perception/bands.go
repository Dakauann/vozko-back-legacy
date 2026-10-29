package perception

import "math"

type emptyBand struct {
	axis     string
	from, to float64
}

type filledBox struct {
	id  string
	box Box
}

var (
	emptyBandShare = 0.25
	faintOpacity   = 0.1
	focalReach     = 0.03
)

func onCanvas(b Box) (Box, bool) {
	clipped := Box{Left: math.Max(0, b.Left), Right: math.Min(1, b.Right), Top: math.Max(0, b.Top), Bottom: math.Min(1, b.Bottom)}
	return clipped, clipped.Right > clipped.Left && clipped.Bottom > clipped.Top
}

func filledBoxes(scene *Scene) []filledBox {
	var filled []filledBox
	for _, el := range scene.Elements {
		if el.Opacity < faintOpacity {
			continue
		}
		source := el.Box
		if isText(el) {
			source = inkBox(scene.Canvas, el)
		}
		box, inside := onCanvas(source)
		if !inside || (!el.Media && area(box) >= backgroundShare) {
			continue
		}
		filled = append(filled, filledBox{id: el.ID, box: box})
	}
	return filled
}

func spanOf(axis string, b Box) (float64, float64) {
	if axis == "x" {
		return b.Left, b.Right
	}
	return b.Top, b.Bottom
}

func openGaps(spans [][2]float64) [][2]float64 {
	stableSort(spans, func(a, b [2]float64) bool { return a[0] < b[0] })
	var found [][2]float64
	reached := 0.0
	for _, s := range spans {
		if s[0] > reached {
			found = append(found, [2]float64{reached, s[0]})
		}
		reached = math.Max(reached, s[1])
	}
	if reached < 1 {
		found = append(found, [2]float64{reached, 1})
	}
	return found
}

func framesFocal(gap [2]float64, filled []filledBox, focal map[string]bool, axis string) bool {
	for _, f := range filled {
		if !focal[f.id] {
			continue
		}
		low, high := spanOf(axis, f.box)
		if math.Abs(high-gap[0]) <= focalReach || math.Abs(low-gap[1]) <= focalReach {
			return true
		}
	}
	return false
}

func emptyBands(scene *Scene, focal map[string]bool) []emptyBand {
	filled := filledBoxes(scene)
	if len(filled) == 0 {
		return nil
	}
	var bands []emptyBand
	for _, axis := range []string{"x", "y"} {
		spans := make([][2]float64, len(filled))
		for i, f := range filled {
			low, high := spanOf(axis, f.box)
			spans[i] = [2]float64{low, high}
		}
		var widest *[2]float64
		for _, gap := range openGaps(spans) {
			if gap[1]-gap[0] < emptyBandShare || framesFocal(gap, filled, focal, axis) {
				continue
			}
			if widest == nil || gap[1]-gap[0] > widest[1]-widest[0] {
				chosen := gap
				widest = &chosen
			}
		}
		if widest != nil {
			bands = append(bands, emptyBand{axis: axis, from: widest[0], to: widest[1]})
		}
	}
	return bands
}
