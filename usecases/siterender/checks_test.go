package siterender

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"vozko/domain/site"
)

func codes(checks []site.Check) []string {
	out := make([]string, len(checks))
	for i, c := range checks {
		out[i] = c.Code
	}
	return out
}

func find(checks []site.Check, code string) (site.Check, bool) {
	for _, c := range checks {
		if c.Code == code {
			return c, true
		}
	}
	return site.Check{}, false
}

func TestFactChecksTurnRawFactsIntoDomainCodes(t *testing.T) {
	facts := Facts{
		ScrollWidth: 420,
		Overflow:    []string{"src/App.tsx#90"},
		Texts: []TextSample{
			{Node: "src/App.tsx#10", FG: [3]int{150, 150, 150}, BG: [3]int{255, 255, 255}, Px: 16, Weight: 400},
			{Node: "src/App.tsx#11", FG: [3]int{140, 140, 140}, BG: [3]int{255, 255, 255}, Px: 32, Weight: 700},
			{Node: "src/App.tsx#12", FG: [3]int{250, 250, 250}, BG: [3]int{255, 255, 255}, Px: 16, Weight: 400, OverMedia: true},
		},
		Images: []ImageSample{
			{Node: "src/App.tsx#20", NaturalW: 300, NaturalH: 200, ShownW: 358, ShownH: 239},
			{Node: "src/App.tsx#21", NaturalW: 1200, NaturalH: 800, ShownW: 358, ShownH: 239},
			{Node: "src/App.tsx#22", NaturalW: 0, NaturalH: 0, ShownW: 358, ShownH: 239},
		},
		Axe:            []AxeViolation{{ID: "button-name", Impact: "critical", Count: 2, Nodes: []string{"src/App.tsx#30"}}},
		Metrics:        Metrics{LCP: 3100, CLS: 0.25, TBT: 450},
		ConsoleErrors:  []string{"TypeError: x is undefined"},
		FailedRequests: []FailedRequest{{URL: "https://cdn.example.com/lib.js?v=1", Blocked: true}, {URL: "https://p.paginas.test/img/a.png", Status: 404}},
		TransferBytes:  3_200_000,
	}
	checks := FactChecks("/", Mobile, facts)
	for _, code := range []string{site.CheckConsoleError, site.CheckRequestFailed, site.CheckOverflowHorizontal, site.CheckContrastLow, site.CheckImageTooSmall, site.CheckAxeViolation, site.CheckLCPSlow, site.CheckCLSHigh, site.CheckTBTHigh, site.CheckPageWeightHigh} {
		if _, ok := find(checks, code); !ok {
			t.Fatalf("missing %s in %v", code, codes(checks))
		}
	}
	contrast := 0
	images := 0
	for _, c := range checks {
		if c.Viewport != Mobile.Width || c.Route != "/" {
			t.Fatalf("check %+v lost its viewport or route", c)
		}
		switch c.Code {
		case site.CheckContrastLow:
			contrast++
			if c.Node != "src/App.tsx#10" || c.Detail != "2.95 < 4.5" {
				t.Fatalf("contrast check %+v", c)
			}
		case site.CheckImageTooSmall:
			images++
			if c.Node != "src/App.tsx#20" || c.Detail != "300x200 shown 358x239" {
				t.Fatalf("image check %+v", c)
			}
		case site.CheckRequestFailed:
			if !strings.HasPrefix(c.Detail, "blocked cdn.example.com") && c.Detail != "404 p.paginas.test/img/a.png" {
				t.Fatalf("request check %+v", c)
			}
		case site.CheckOverflowHorizontal:
			if c.Node != "src/App.tsx#90" || c.Detail != "420px > 390px" {
				t.Fatalf("overflow check %+v", c)
			}
		}
	}
	if contrast != 1 || images != 1 {
		t.Fatalf("contrast %d images %d, want 1 and 1", contrast, images)
	}
}

func TestFactChecksSkipOverflowOnDesktopAndLowDensityImages(t *testing.T) {
	facts := Facts{
		ScrollWidth: 1600,
		Images:      []ImageSample{{Node: "n#1", NaturalW: 800, NaturalH: 600, ShownW: 800, ShownH: 600}},
		Metrics:     Metrics{LCP: 900, CLS: 0.01, TBT: 10},
	}
	if checks := FactChecks("/", Desktop, facts); len(checks) != 0 {
		t.Fatalf("desktop checks = %+v, want none", checks)
	}
}

func TestCondenseMergesViewportsCapsPerCodeAndOverall(t *testing.T) {
	var in []site.Check
	for i := range 6 {
		in = append(in, site.Check{Code: site.CheckContrastLow, Node: fmt.Sprintf("n#%d", i), Viewport: 1440, Detail: "2.10 < 4.5"})
	}
	in = append(in,
		site.Check{Code: site.CheckContrastLow, Node: "n#0", Viewport: 390, Detail: "2.10 < 4.5"},
		site.Check{Code: site.CheckLCPSlow, Viewport: 1440, Detail: "3.0 s"},
		site.Check{Code: site.CheckLCPSlow, Viewport: 390, Detail: "4.1 s"},
		site.Check{Code: site.CheckH1Missing, Route: "/"},
	)
	for i := range 8 {
		in = append(in, site.Check{Code: site.CheckConsoleError, Viewport: 1440, Detail: fmt.Sprintf("error %d", i)})
	}
	out, omitted := Condense(in)
	if len(out) > MaxCheckLines {
		t.Fatalf("%d lines over the cap", len(out))
	}
	if out[0].Code != site.CheckConsoleError {
		t.Fatalf("console errors come first, got %v", codes(out))
	}
	perCode := map[string]int{}
	for _, c := range out {
		perCode[c.Code]++
	}
	if perCode[site.CheckConsoleError] != maxPerCode || perCode[site.CheckContrastLow] != maxPerCode {
		t.Fatalf("per code counts %v", perCode)
	}
	merged, _ := find(out, site.CheckContrastLow)
	if merged.Node != "n#0" || merged.Viewport != 0 {
		t.Fatalf("the same contrast issue at both widths is one line without a viewport: %+v", merged)
	}
	if perCode[site.CheckLCPSlow] != 2 || perCode[site.CheckH1Missing] != 1 {
		t.Fatalf("per code counts %v", perCode)
	}
	if omitted != len(in)-1-len(out) {
		t.Fatalf("omitted %d, want %d", omitted, len(in)-1-len(out))
	}
}

func TestCondenseTruncatesLongDetails(t *testing.T) {
	out, _ := Condense([]site.Check{{Code: site.CheckConsoleError, Detail: strings.Repeat("é", 400)}})
	if utf8.RuneCountInString(out[0].Detail) > maxDetailRunes {
		t.Fatalf("detail has %d runes", utf8.RuneCountInString(out[0].Detail))
	}
}

func TestCondenseOrdersAxeBeforeContrastAndPerformanceLast(t *testing.T) {
	out, _ := Condense([]site.Check{
		{Code: site.CheckPageWeightHigh, Detail: "3.0 MB"},
		{Code: site.CheckContrastLow, Node: "a#1"},
		{Code: site.CheckAxeViolation, Node: "a#2"},
		{Code: "unknown_code"},
	})
	got := strings.Join(codes(out), ",")
	want := strings.Join([]string{site.CheckAxeViolation, site.CheckContrastLow, site.CheckPageWeightHigh, "unknown_code"}, ",")
	if got != want {
		t.Fatalf("order %s, want %s", got, want)
	}
}
