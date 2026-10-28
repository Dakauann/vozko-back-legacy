package siterender

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"math"

	"github.com/disintegration/imaging"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"vozko/domain/copilot"
)

const (
	MaxImageBytes  = copilot.MaxScreenImageBytes
	sheetMaxWidth  = 2048
	sheetMaxHeight = 2048
	sheetGap       = 12
	minSheetScale  = 0.02
	scaleSearches  = 32
	minEncodeEdge  = 16
)

var (
	encodeQualities = []int{82, 68, 55, 42}
	sheetBackground = color.RGBA{R: 0x2b, G: 0x2b, B: 0x2b, A: 0xff}
	labelBackground = color.RGBA{A: 0xd8}
	labelInk        = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	errEncodeBudget = errors.New("siterender: the image does not fit the vision budget")
)

func SliceTops(pageHeight, viewportHeight, maxSlices int) ([]int, bool) {
	if viewportHeight <= 0 || maxSlices <= 0 {
		return nil, false
	}
	pageHeight = max(pageHeight, viewportHeight)
	last := pageHeight - viewportHeight
	var tops []int
	for top := 0; top < pageHeight || len(tops) == 0; top += viewportHeight {
		clamped := min(top, last)
		if len(tops) > 0 && tops[len(tops)-1] == clamped {
			break
		}
		tops = append(tops, clamped)
		if len(tops) == maxSlices && top+viewportHeight < pageHeight {
			return tops, true
		}
	}
	return tops, false
}

type sheetLayout struct {
	Size  image.Point
	Scale float64
	Rects [][]image.Rectangle
}

func placeTiles(groups [][]image.Point, scale float64, maxWidth int) sheetLayout {
	layout := sheetLayout{Scale: scale, Rects: make([][]image.Rectangle, len(groups))}
	x, y, rowHeight, width := 0, 0, 0, 0
	for g, group := range groups {
		if g > 0 && rowHeight > 0 {
			y += rowHeight + sheetGap
			x, rowHeight = 0, 0
		}
		for _, size := range group {
			w := max(1, int(math.Round(float64(size.X)*scale)))
			h := max(1, int(math.Round(float64(size.Y)*scale)))
			if x > 0 && x+w > maxWidth {
				y += rowHeight + sheetGap
				x, rowHeight = 0, 0
			}
			layout.Rects[g] = append(layout.Rects[g], image.Rect(x, y, x+w, y+h))
			width = max(width, x+w)
			rowHeight = max(rowHeight, h)
			x += w + sheetGap
		}
	}
	layout.Size = image.Pt(width, y+rowHeight)
	return layout
}

func layoutSheet(groups [][]image.Point, maxWidth, maxHeight int) sheetLayout {
	fits := func(l sheetLayout) bool {
		return l.Size.X <= maxWidth && l.Size.Y <= maxHeight
	}
	if best := placeTiles(groups, 1, maxWidth); fits(best) {
		return best
	}
	low, high := minSheetScale, 1.0
	best := placeTiles(groups, low, maxWidth)
	for range scaleSearches {
		mid := (low + high) / 2
		candidate := placeTiles(groups, mid, maxWidth)
		if fits(candidate) {
			best, low = candidate, mid
			continue
		}
		high = mid
	}
	return best
}

type tile struct {
	group int
	image image.Image
	label string
}

type sheetCanvas struct {
	layout sheetLayout
	canvas *image.RGBA
}

func newSheetCanvas(groups [][]image.Point) *sheetCanvas {
	layout := layoutSheet(groups, sheetMaxWidth, sheetMaxHeight)
	canvas := image.NewRGBA(image.Rect(0, 0, max(layout.Size.X, 1), max(layout.Size.Y, 1)))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: sheetBackground}, image.Point{}, draw.Src)
	return &sheetCanvas{layout: layout, canvas: canvas}
}

func (s *sheetCanvas) place(group, index int, img image.Image, label string) {
	if group >= len(s.layout.Rects) || index >= len(s.layout.Rects[group]) {
		return
	}
	rect := s.layout.Rects[group][index]
	scaled := img
	if rect.Size() != img.Bounds().Size() {
		scaled = imaging.Resize(img, rect.Dx(), rect.Dy(), imaging.Linear)
	}
	draw.Draw(s.canvas, rect, scaled, scaled.Bounds().Min, draw.Src)
	drawLabel(s.canvas, rect.Min.Add(image.Pt(4, 4)), label)
}

func (s *sheetCanvas) encode(maxBytes int) ([]byte, error) {
	return encodeWithin(s.canvas, maxBytes)
}

func composeSheet(tiles []tile, maxBytes int) ([]byte, error) {
	if len(tiles) == 0 {
		return nil, errEncodeBudget
	}
	slot := map[int]int{}
	var groups [][]image.Point
	positions := make([][2]int, len(tiles))
	for i, t := range tiles {
		g, ok := slot[t.group]
		if !ok {
			g = len(groups)
			slot[t.group] = g
			groups = append(groups, nil)
		}
		positions[i] = [2]int{g, len(groups[g])}
		groups[g] = append(groups[g], t.image.Bounds().Size())
	}
	sheet := newSheetCanvas(groups)
	for i, t := range tiles {
		sheet.place(positions[i][0], positions[i][1], t.image, t.label)
	}
	return sheet.encode(maxBytes)
}

func drawLabel(dst draw.Image, at image.Point, text string) {
	if text == "" {
		return
	}
	face := basicfont.Face7x13
	width := font.MeasureString(face, text).Ceil()
	box := image.Rect(at.X, at.Y, at.X+width+8, at.Y+face.Height+4).Intersect(dst.Bounds())
	draw.Draw(dst, box, &image.Uniform{C: labelBackground}, image.Point{}, draw.Over)
	drawer := font.Drawer{
		Dst:  dst,
		Src:  &image.Uniform{C: labelInk},
		Face: face,
		Dot:  fixed.P(at.X+4, at.Y+2+face.Ascent),
	}
	drawer.DrawString(text)
}

func encodeWithin(img image.Image, maxBytes int) ([]byte, error) {
	current := img
	for {
		for _, quality := range encodeQualities {
			var buf bytes.Buffer
			if err := jpeg.Encode(&buf, current, &jpeg.Options{Quality: quality}); err != nil {
				return nil, err
			}
			if buf.Len() <= maxBytes {
				return buf.Bytes(), nil
			}
		}
		size := current.Bounds().Size()
		if size.X/2 < minEncodeEdge || size.Y/2 < minEncodeEdge {
			return nil, errEncodeBudget
		}
		current = imaging.Resize(current, size.X/2, size.Y/2, imaging.Linear)
	}
}
