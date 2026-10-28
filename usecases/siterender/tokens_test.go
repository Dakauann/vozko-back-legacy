package siterender

import (
	"reflect"
	"testing"
)

func TestDominantColorsMergeNearShadesAndShareByArea(t *testing.T) {
	got := dominantColors([]ColorSample{
		{RGB: [3]int{255, 255, 255}, Weight: 1_000_000},
		{RGB: [3]int{37, 99, 235}, Weight: 300_000},
		{RGB: [3]int{250, 250, 250}, Weight: 200_000},
		{RGB: [3]int{1, 2, 3}, Weight: 0},
	}, 6)
	want := []ColorShare{{Hex: "#fefefe", Share: 0.8}, {Hex: "#2563eb", Share: 0.2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dominantColors = %+v, want %+v", got, want)
	}
}

func TestDominantColorsRespectTheLimitAndDropSlivers(t *testing.T) {
	got := dominantColors([]ColorSample{
		{RGB: [3]int{0, 0, 0}, Weight: 500},
		{RGB: [3]int{255, 0, 0}, Weight: 300},
		{RGB: [3]int{0, 255, 0}, Weight: 196},
		{RGB: [3]int{0, 0, 255}, Weight: 4},
	}, 2)
	if len(got) != 2 || got[0].Hex != "#000000" || got[1].Hex != "#ff0000" {
		t.Fatalf("dominantColors = %+v", got)
	}
}

func TestFontSharesNormaliseFamilyNames(t *testing.T) {
	got := fontShares([]FontSample{
		{Family: `"Inter"`, Weight: 300},
		{Family: " inter ", Weight: 100},
		{Family: "Georgia", Weight: 100},
		{Family: "", Weight: 900},
	}, 4)
	want := []FontShare{{Family: "Inter", Share: 0.8}, {Family: "Georgia", Share: 0.2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fontShares = %+v, want %+v", got, want)
	}
}

func TestSpacingRhythmFindsTheBaseUnit(t *testing.T) {
	cases := []struct {
		name   string
		values []float64
		want   Spacing
	}{
		{"eight point grid", []float64{8, 16, 24, 32, 16, 48, 8, 64, 12}, Spacing{Base: 8, Scale: []int{8, 12, 16, 24, 32, 48, 64}}},
		{"no rhythm", []float64{4, 6, 10, 14, 18, 7, 9, 11}, Spacing{Scale: []int{4, 6, 7, 9, 10, 11, 14, 18}}},
		{"rounded fractions", []float64{15.6, 32.2, 47.9}, Spacing{Base: 16, Scale: []int{16, 32, 48}}},
		{"nothing measured", nil, Spacing{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := spacingRhythm(c.values); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("spacingRhythm = %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestExtractTokensCombinesEveryPart(t *testing.T) {
	tokens := ExtractTokens(TokenSamples{
		Backgrounds: []ColorSample{{RGB: [3]int{255, 255, 255}, Weight: 10}},
		Texts:       []ColorSample{{RGB: [3]int{17, 17, 17}, Weight: 10}},
		Fonts:       []FontSample{{Family: "Inter", Weight: 1}},
		Spacing:     []float64{8, 16},
	})
	if len(tokens.Backgrounds) != 1 || len(tokens.Texts) != 1 || len(tokens.Fonts) != 1 || tokens.Spacing.Base != 8 {
		t.Fatalf("tokens = %+v", tokens)
	}
}
