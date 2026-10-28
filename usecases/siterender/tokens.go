package siterender

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

const (
	colorMergeDistance = 24
	minColorShare      = 0.01
	backgroundColors   = 6
	textColors         = 4
	fontFamilies       = 4
	spacingSteps       = 8
	maxSpacingPx       = 256
	rhythmTolerance    = 0.85
	minRhythmScore     = 0.5
)

var spacingBases = []int{4, 5, 6, 8, 10, 12, 16}

type ColorShare struct {
	Hex   string  `json:"hex"`
	Share float64 `json:"share"`
}

type FontShare struct {
	Family string  `json:"family"`
	Share  float64 `json:"share"`
}

type Spacing struct {
	Base  int   `json:"base,omitempty"`
	Scale []int `json:"scale,omitempty"`
}

type Tokens struct {
	Backgrounds []ColorShare `json:"backgrounds"`
	Texts       []ColorShare `json:"texts"`
	Fonts       []FontShare  `json:"fonts"`
	Spacing     Spacing      `json:"spacing"`
}

func ExtractTokens(samples TokenSamples) Tokens {
	return Tokens{
		Backgrounds: dominantColors(samples.Backgrounds, backgroundColors),
		Texts:       dominantColors(samples.Texts, textColors),
		Fonts:       fontShares(samples.Fonts, fontFamilies),
		Spacing:     spacingRhythm(samples.Spacing),
	}
}

type colorCluster struct {
	sum    [3]float64
	weight float64
}

func (c colorCluster) mean() [3]float64 {
	return [3]float64{c.sum[0] / c.weight, c.sum[1] / c.weight, c.sum[2] / c.weight}
}

func clampChannel(v float64) int {
	return int(math.Max(0, math.Min(255, math.Round(v))))
}

func roundShare(v float64) float64 {
	return math.Round(v*100) / 100
}

func dominantColors(samples []ColorSample, limit int) []ColorShare {
	ordered := make([]ColorSample, 0, len(samples))
	total := 0.0
	for _, s := range samples {
		if s.Weight > 0 && !math.IsInf(s.Weight, 0) && !math.IsNaN(s.Weight) {
			ordered = append(ordered, s)
			total += s.Weight
		}
	}
	if total == 0 {
		return []ColorShare{}
	}
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Weight > ordered[j].Weight })
	var clusters []colorCluster
	for _, s := range ordered {
		rgb := [3]float64{float64(clampChannel(float64(s.RGB[0]))), float64(clampChannel(float64(s.RGB[1]))), float64(clampChannel(float64(s.RGB[2])))}
		merged := false
		for i := range clusters {
			m := clusters[i].mean()
			if math.Sqrt(sq(m[0]-rgb[0])+sq(m[1]-rgb[1])+sq(m[2]-rgb[2])) < colorMergeDistance {
				for k := range rgb {
					clusters[i].sum[k] += rgb[k] * s.Weight
				}
				clusters[i].weight += s.Weight
				merged = true
				break
			}
		}
		if !merged {
			clusters = append(clusters, colorCluster{sum: [3]float64{rgb[0] * s.Weight, rgb[1] * s.Weight, rgb[2] * s.Weight}, weight: s.Weight})
		}
	}
	sort.SliceStable(clusters, func(i, j int) bool { return clusters[i].weight > clusters[j].weight })
	out := make([]ColorShare, 0, limit)
	for _, c := range clusters {
		share := c.weight / total
		if share < minColorShare || len(out) == limit {
			break
		}
		m := c.mean()
		out = append(out, ColorShare{Hex: fmt.Sprintf("#%02x%02x%02x", clampChannel(m[0]), clampChannel(m[1]), clampChannel(m[2])), Share: roundShare(share)})
	}
	return out
}

func sq(v float64) float64 { return v * v }

func fontShares(samples []FontSample, limit int) []FontShare {
	type family struct {
		name   string
		weight float64
		order  int
	}
	families := map[string]*family{}
	total := 0.0
	for _, s := range samples {
		name := strings.Trim(strings.TrimSpace(s.Family), `"'`)
		name = strings.TrimSpace(name)
		if name == "" || s.Weight <= 0 {
			continue
		}
		key := strings.ToLower(name)
		f, ok := families[key]
		if !ok {
			f = &family{name: name, order: len(families)}
			families[key] = f
		}
		f.weight += s.Weight
		total += s.Weight
	}
	list := make([]*family, 0, len(families))
	for _, f := range families {
		list = append(list, f)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].weight != list[j].weight {
			return list[i].weight > list[j].weight
		}
		return list[i].order < list[j].order
	})
	out := make([]FontShare, 0, limit)
	for _, f := range list {
		if len(out) == limit {
			break
		}
		out = append(out, FontShare{Family: f.name, Share: roundShare(f.weight / total)})
	}
	return out
}

func spacingRhythm(values []float64) Spacing {
	counts := map[int]int{}
	var kept []int
	for _, v := range values {
		px := int(math.Round(v))
		if px < 1 || px > maxSpacingPx {
			continue
		}
		counts[px]++
		kept = append(kept, px)
	}
	if len(kept) == 0 {
		return Spacing{}
	}
	scores := make([]float64, len(spacingBases))
	best := 0.0
	for i, base := range spacingBases {
		hits := 0
		for _, px := range kept {
			if px%base == 0 {
				hits++
			}
		}
		scores[i] = float64(hits) / float64(len(kept))
		best = math.Max(best, scores[i])
	}
	result := Spacing{}
	if best >= minRhythmScore {
		for i, base := range spacingBases {
			if scores[i] >= best*rhythmTolerance {
				result.Base = base
			}
		}
	}
	distinct := make([]int, 0, len(counts))
	for px := range counts {
		distinct = append(distinct, px)
	}
	sort.Slice(distinct, func(i, j int) bool {
		if counts[distinct[i]] != counts[distinct[j]] {
			return counts[distinct[i]] > counts[distinct[j]]
		}
		return distinct[i] < distinct[j]
	})
	if len(distinct) > spacingSteps {
		distinct = distinct[:spacingSteps]
	}
	sort.Ints(distinct)
	result.Scale = distinct
	return result
}
