package siterender

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"math/rand"
	"reflect"
	"testing"
)

func TestSliceTopsCoverThePageWithoutGaps(t *testing.T) {
	cases := []struct {
		name      string
		page, vh  int
		max       int
		want      []int
		truncated bool
	}{
		{"short page", 600, 900, 10, []int{0}, false},
		{"exact fit", 1800, 900, 10, []int{0, 900}, false},
		{"last slice aligns to the bottom", 2000, 900, 10, []int{0, 900, 1100}, false},
		{"long page is cut at the cap", 9000, 900, 3, []int{0, 900, 1800}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, truncated := SliceTops(c.page, c.vh, c.max)
			if !reflect.DeepEqual(got, c.want) || truncated != c.truncated {
				t.Fatalf("SliceTops = %v %v, want %v %v", got, truncated, c.want, c.truncated)
			}
		})
	}
}

func TestLayoutSheetFitsEveryTileInsideTheBounds(t *testing.T) {
	desktop := image.Pt(1440, 900)
	phone := image.Pt(390, 844)
	cases := []struct {
		name   string
		groups [][]image.Point
	}{
		{"one slice", [][]image.Point{{desktop}}},
		{"two viewports", [][]image.Point{{desktop, desktop, desktop}, {phone, phone, phone, phone}}},
		{"very long page", [][]image.Point{repeat(desktop, 24), repeat(phone, 24)}},
		{"frames group", [][]image.Point{{desktop, desktop}, {phone}, {desktop, desktop, desktop}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			layout := layoutSheet(c.groups, sheetMaxWidth, sheetMaxHeight)
			if layout.Size.X > sheetMaxWidth || layout.Size.Y > sheetMaxHeight || layout.Size.X <= 0 || layout.Size.Y <= 0 {
				t.Fatalf("sheet size %v outside %dx%d", layout.Size, sheetMaxWidth, sheetMaxHeight)
			}
			bounds := image.Rect(0, 0, layout.Size.X, layout.Size.Y)
			var all []image.Rectangle
			for g, rects := range layout.Rects {
				if len(rects) != len(c.groups[g]) {
					t.Fatalf("group %d has %d rects for %d tiles", g, len(rects), len(c.groups[g]))
				}
				for _, r := range rects {
					if !r.In(bounds) || r.Empty() {
						t.Fatalf("tile %v outside %v", r, bounds)
					}
					for _, other := range all {
						if r.Overlaps(other) {
							t.Fatalf("tiles %v and %v overlap", r, other)
						}
					}
					all = append(all, r)
				}
			}
		})
	}
}

func TestLayoutSheetKeepsASingleSmallSliceAtNativeSize(t *testing.T) {
	layout := layoutSheet([][]image.Point{{image.Pt(390, 844)}}, sheetMaxWidth, sheetMaxHeight)
	if layout.Scale != 1 || layout.Rects[0][0].Dx() != 390 || layout.Rects[0][0].Dy() != 844 {
		t.Fatalf("a slice that fits is not shrunk: scale %v rect %v", layout.Scale, layout.Rects[0][0])
	}
}

func TestLayoutSheetStartsEachGroupOnANewRow(t *testing.T) {
	layout := layoutSheet([][]image.Point{{image.Pt(200, 100)}, {image.Pt(200, 100)}}, sheetMaxWidth, sheetMaxHeight)
	if layout.Rects[1][0].Min.Y <= layout.Rects[0][0].Min.Y {
		t.Fatalf("second group shares the first row: %v %v", layout.Rects[0][0], layout.Rects[1][0])
	}
}

func TestEncodeWithinStaysUnderTheVisionBudget(t *testing.T) {
	noisy := image.NewRGBA(image.Rect(0, 0, 2048, 2048))
	random := rand.New(rand.NewSource(7))
	for i := range noisy.Pix {
		noisy.Pix[i] = uint8(random.Intn(256))
	}
	budget := 400 << 10
	data, err := encodeWithin(noisy, budget)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if len(data) > budget {
		t.Fatalf("encoded %d bytes over a %d budget", len(data), budget)
	}
	if _, err := jpeg.Decode(bytes.NewReader(data)); err != nil {
		t.Fatalf("output is not a JPEG: %v", err)
	}
}

func TestComposeSheetProducesOneJPEG(t *testing.T) {
	tiles := []tile{
		{group: 0, image: solid(1440, 900, color.RGBA{200, 10, 10, 255}), label: "1440 1/2"},
		{group: 0, image: solid(1440, 900, color.RGBA{10, 200, 10, 255}), label: "1440 2/2"},
		{group: 1, image: solid(390, 844, color.RGBA{10, 10, 200, 255}), label: "390 1/1"},
	}
	data, err := composeSheet(tiles, MaxImageBytes)
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	if len(data) == 0 || len(data) > MaxImageBytes {
		t.Fatalf("sheet has %d bytes", len(data))
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("sheet is not a JPEG: %v", err)
	}
	if img.Bounds().Dx() > sheetMaxWidth || img.Bounds().Dy() > sheetMaxHeight {
		t.Fatalf("sheet %v exceeds the sheet bounds", img.Bounds())
	}
}

func repeat(p image.Point, n int) []image.Point {
	out := make([]image.Point, n)
	for i := range out {
		out[i] = p
	}
	return out
}

func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}
