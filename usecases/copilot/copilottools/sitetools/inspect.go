package sitetools

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"vozko/domain/copilot"
	"vozko/domain/site"
	site_usecase "vozko/usecases/site"
	"vozko/usecases/siteinspect"
	"vozko/usecases/siterender"
)

const (
	maxLines          = siterender.MaxCheckLines
	moreLinesTemplate = "e mais %d"
	elementPathPrefix = "html["
)

type inspection struct {
	known       bool
	reason      string
	gaps        []site.Issue
	checks      []string
	buildErrors []string
}

func (i inspection) gapLines() []string {
	return i.gapLinesUpTo(maxLines)
}

func (i inspection) gapLinesUpTo(limit int) []string {
	lines := make([]string, 0, len(i.gaps))
	for _, gap := range i.gaps {
		lines = append(lines, issueLine(gap))
	}
	return capLinesAt(lines, limit)
}

func issueLine(issue site.Issue) string {
	var b strings.Builder
	b.WriteString(issue.Code)
	if issue.Route != "" {
		b.WriteString(" em " + issue.Route)
	}
	if issue.Node != "" {
		b.WriteString(" (" + issue.Node + ")")
	} else if issue.Path != "" {
		b.WriteString(" (" + issue.Path + ")")
	}
	if detail := strings.TrimSpace(issue.Detail); detail != "" && !(issue.Node != "" && strings.HasPrefix(detail, elementPathPrefix)) {
		b.WriteString(": " + clip(detail, 80))
	}
	return b.String()
}

func checkLine(c site.Check) string {
	return checkLineWithin(c, 80)
}

func checkLineWithin(c site.Check, detailRunes int) string {
	var b strings.Builder
	b.WriteString(c.Code)
	if c.Route != "" {
		b.WriteString(" em " + c.Route)
	}
	if c.Viewport != 0 {
		b.WriteString(" a " + strconv.Itoa(c.Viewport) + " px")
	}
	if c.Node != "" {
		b.WriteString(" (" + c.Node + ")")
	}
	if detail := strings.TrimSpace(c.Detail); detail != "" {
		b.WriteString(": " + clip(detail, detailRunes))
	}
	return b.String()
}

func clip(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit-1]) + "…"
}

func capLines(lines []string) []string {
	return capLinesAt(lines, maxLines)
}

func capLinesAt(lines []string, limit int) []string {
	seen := map[string]bool{}
	out := make([]string, 0, min(len(lines), limit+1))
	extra := 0
	for _, line := range lines {
		if seen[line] {
			continue
		}
		seen[line] = true
		if len(out) == limit {
			extra++
			continue
		}
		out = append(out, line)
	}
	if extra > 0 {
		out = append(out, fmt.Sprintf(moreLinesTemplate, extra))
	}
	return out
}

func buildLines(issues []site_usecase.BuildIssue) []string {
	lines := make([]string, 0, len(issues))
	for _, issue := range issues {
		where := issue.Path
		if issue.Line > 0 {
			where += ":" + strconv.Itoa(issue.Line)
		}
		lines = append(lines, strings.TrimSpace(where+" "+clip(strings.TrimSpace(issue.Message), 160)))
	}
	return capLines(lines)
}

func ruleLines(issues []site.Issue) []string {
	lines := make([]string, 0, len(issues))
	for _, issue := range issues {
		lines = append(lines, issueLine(issue))
	}
	return capLines(lines)
}

func routesOf(view *site_usecase.View) ([]string, error) {
	routes, ok := siteinspect.Routes(view)
	if !ok {
		return nil, errors.New(siteinspect.ReasonNoManifest)
	}
	return routes, nil
}

func settingsOf(view *site_usecase.View) site.Settings {
	return siteinspect.Settings(view)
}

func (d Deps) inspect(ctx context.Context, cc copilot.Context, view *site_usecase.View) inspection {
	report := siteinspect.Service{Previews: d.Storage, Cache: d.Inspections}.Inspect(ctx, view, editorDocuments{cc: cc})
	switch {
	case len(report.BuildIssues) > 0:
		return inspection{reason: report.Reason, buildErrors: buildLines(report.BuildIssues)}
	case len(report.RuleIssues) > 0:
		return inspection{reason: report.Reason, buildErrors: ruleLines(report.RuleIssues)}
	case !report.Known:
		return inspection{reason: report.Reason}
	}
	checks := make([]string, 0, len(report.Checks)+1)
	for _, c := range report.Checks {
		checks = append(checks, checkLine(c))
	}
	if report.OmittedChecks > 0 {
		checks = append(checks, fmt.Sprintf(moreLinesTemplate, report.OmittedChecks))
	}
	return inspection{known: true, gaps: report.Gaps, checks: checks}
}
