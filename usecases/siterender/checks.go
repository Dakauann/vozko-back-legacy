package siterender

import (
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"

	"vozko/domain/site"
	"vozko/domain/wcag"
)

const (
	MaxCheckLines       = 12
	maxPerCode          = 3
	maxDetailRunes      = 80
	maxOverflowNodes    = 3
	phoneMaxWidth       = 480
	phoneDensity        = 2
	lcpSlowMS           = 2500
	clsHigh             = 0.1
	tbtHighMS           = 200
	pageWeightHighBytes = 2_500_000
	megabyte            = 1_000_000
)

var checkOrder = []string{
	site.CheckConsoleError,
	site.CheckRequestFailed,
	site.CheckOverflowHorizontal,
	site.CheckAxeViolation,
	site.CheckContrastLow,
	site.CheckImageTooSmall,
	site.CheckH1Missing,
	site.CheckH1Repeated,
	site.CheckHeadingOrder,
	site.CheckLinkUnnamed,
	site.CheckLCPSlow,
	site.CheckCLSHigh,
	site.CheckTBTHigh,
	site.CheckPageWeightHigh,
}

var impactRank = map[string]int{"critical": 0, "serious": 1, "moderate": 2, "minor": 3}

func rankOf(code string) int {
	for i, c := range checkOrder {
		if c == code {
			return i
		}
	}
	return len(checkOrder)
}

func phone(v Viewport) bool {
	return v.Width <= phoneMaxWidth
}

func channels(rgb [3]int) [3]uint8 {
	var out [3]uint8
	for i, c := range rgb {
		out[i] = uint8(min(max(c, 0), 255))
	}
	return out
}

func FactChecks(route string, v Viewport, f Facts) []site.Check {
	var checks []site.Check
	add := func(code, node, detail string) {
		checks = append(checks, site.Check{Code: code, Route: route, Node: node, Viewport: v.Width, Detail: detail})
	}
	for _, message := range f.ConsoleErrors {
		add(site.CheckConsoleError, "", strings.TrimSpace(message))
	}
	for _, failed := range f.FailedRequests {
		add(site.CheckRequestFailed, "", requestDetail(failed))
	}
	if phone(v) && f.ScrollWidth > v.Width+1 {
		detail := fmt.Sprintf("%dpx > %dpx", f.ScrollWidth, v.Width)
		if len(f.Overflow) == 0 {
			add(site.CheckOverflowHorizontal, "", detail)
		}
		for i, node := range f.Overflow {
			if i == maxOverflowNodes {
				break
			}
			add(site.CheckOverflowHorizontal, node, detail)
		}
	}
	axe := append([]AxeViolation(nil), f.Axe...)
	sort.SliceStable(axe, func(i, j int) bool { return impactOf(axe[i].Impact) < impactOf(axe[j].Impact) })
	for _, violation := range axe {
		node := ""
		if len(violation.Nodes) > 0 {
			node = violation.Nodes[0]
		}
		add(site.CheckAxeViolation, node, fmt.Sprintf("%s (%s) x%d", violation.ID, violation.Impact, max(violation.Count, 1)))
	}
	for _, text := range f.Texts {
		if text.OverMedia {
			continue
		}
		ratio := wcag.ContrastRatio(channels(text.FG), channels(text.BG))
		minimum := wcag.Minimum(text.Px, text.Weight)
		if ratio < minimum {
			add(site.CheckContrastLow, text.Node, fmt.Sprintf("%.2f < %.1f", math.Floor(ratio*100)/100, minimum))
		}
	}
	density := 1
	if phone(v) {
		density = phoneDensity
	}
	for _, img := range f.Images {
		if img.NaturalW <= 0 || img.ShownW <= 0 {
			continue
		}
		if img.NaturalW < img.ShownW*density {
			add(site.CheckImageTooSmall, img.Node, fmt.Sprintf("%dx%d shown %dx%d", img.NaturalW, img.NaturalH, img.ShownW, img.ShownH))
		}
	}
	if f.Metrics.LCP > lcpSlowMS {
		add(site.CheckLCPSlow, "", fmt.Sprintf("%.1f s", f.Metrics.LCP/1000))
	}
	if f.Metrics.CLS > clsHigh {
		add(site.CheckCLSHigh, "", fmt.Sprintf("%.2f", f.Metrics.CLS))
	}
	if f.Metrics.TBT > tbtHighMS {
		add(site.CheckTBTHigh, "", fmt.Sprintf("%d ms", int(math.Round(f.Metrics.TBT))))
	}
	if f.TransferBytes > pageWeightHighBytes {
		add(site.CheckPageWeightHigh, "", fmt.Sprintf("%.1f MB", float64(f.TransferBytes)/megabyte))
	}
	return checks
}

func impactOf(impact string) int {
	if rank, ok := impactRank[impact]; ok {
		return rank
	}
	return len(impactRank)
}

func requestDetail(failed FailedRequest) string {
	target := failed.URL
	if u, err := url.Parse(failed.URL); err == nil && u.Host != "" {
		target = u.Host + u.Path
		if failed.Blocked {
			return "blocked " + u.Host
		}
	}
	switch {
	case failed.Blocked:
		return "blocked " + target
	case failed.Status > 0:
		return fmt.Sprintf("%d %s", failed.Status, target)
	case failed.Reason != "":
		return failed.Reason + " " + target
	}
	return target
}

func truncateRunes(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	return string(runes[:limit-1]) + "…"
}

func Condense(checks []site.Check) ([]site.Check, int) {
	type key struct{ code, route, node, detail string }
	index := map[key]int{}
	var unique []site.Check
	for _, c := range checks {
		c.Detail = truncateRunes(strings.TrimSpace(c.Detail), maxDetailRunes)
		k := key{c.Code, c.Route, c.Node, c.Detail}
		if at, ok := index[k]; ok {
			if unique[at].Viewport != c.Viewport {
				unique[at].Viewport = 0
			}
			continue
		}
		index[k] = len(unique)
		unique = append(unique, c)
	}
	sort.SliceStable(unique, func(i, j int) bool { return rankOf(unique[i].Code) < rankOf(unique[j].Code) })
	perCode := map[string]int{}
	out := make([]site.Check, 0, min(len(unique), MaxCheckLines))
	for _, c := range unique {
		if len(out) == MaxCheckLines || perCode[c.Code] == maxPerCode {
			continue
		}
		perCode[c.Code]++
		out = append(out, c)
	}
	return out, len(unique) - len(out)
}
