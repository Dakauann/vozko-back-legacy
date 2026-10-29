package copilot

import (
	"errors"
	"regexp"
	"slices"
	"strings"

	"vozko/domain/shared"
)

var (
	ErrSkillName        = errors.New("copilot skill: name must be lowercase words joined by hyphens")
	ErrSkillDescription = errors.New("copilot skill: description is required")
	ErrSkillBody        = errors.New("copilot skill: body is required")
	ErrSkillDash        = errors.New("copilot skill: em and en dashes are not allowed")
	ErrSkillSurface     = errors.New("copilot skill: surface must be empty, studio, panel, email, site or ads, and extra surfaces must be known and named once")
)

var skillName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

type SkillSurface string

const (
	SkillEverywhere SkillSurface = ""
	SkillStudioOnly SkillSurface = "studio"
	SkillPanelOnly  SkillSurface = "panel"
	SkillEmailWork  SkillSurface = "email"
	SkillSiteWork   SkillSurface = "site"
	SkillAdsWork    SkillSurface = "ads"
)

type Skill struct {
	Name        string
	Description string
	Body        string
	Surface     SkillSurface
	Also        []SkillSurface
}

func (s Skill) OfferedOn(view View) bool {
	if view.OnSite() {
		return s.Surface == SkillSiteWork || slices.Contains(s.Also, SkillSiteWork)
	}
	if slices.ContainsFunc(s.Also, func(extra SkillSurface) bool { return extra.offeredOn(view) }) {
		return true
	}
	return s.Surface.offeredOn(view)
}

func (surface SkillSurface) offeredOn(view View) bool {
	switch surface {
	case SkillStudioOnly:
		return view.OnStudio()
	case SkillPanelOnly:
		return !view.Focused()
	case SkillEmailWork:
		return view.OnEmailDocument() || !view.Focused()
	case SkillSiteWork:
		return view.OnSite()
	case SkillAdsWork:
		return view.OnStudio() || !view.Focused()
	}
	return !view.OnSite()
}

func (surface SkillSurface) known() bool {
	switch surface {
	case SkillEverywhere, SkillStudioOnly, SkillPanelOnly, SkillEmailWork, SkillSiteWork, SkillAdsWork:
		return true
	}
	return false
}

func (s Skill) surfacesValid() bool {
	if !s.Surface.known() {
		return false
	}
	seen := map[SkillSurface]bool{s.Surface: true}
	for _, extra := range s.Also {
		if extra == SkillEverywhere || !extra.known() || seen[extra] {
			return false
		}
		seen[extra] = true
	}
	return true
}

func (s Skill) Title() string {
	for _, line := range strings.Split(s.Body, "\n") {
		if heading, ok := strings.CutPrefix(strings.TrimSpace(line), "# "); ok && strings.TrimSpace(heading) != "" {
			return strings.TrimSpace(heading)
		}
	}
	return s.Name
}

func (s Skill) Validate() error {
	switch {
	case !skillName.MatchString(s.Name):
		return ErrSkillName
	case strings.TrimSpace(s.Description) == "":
		return ErrSkillDescription
	case strings.TrimSpace(s.Body) == "":
		return ErrSkillBody
	case shared.HasLongDash(s.Description + s.Body):
		return ErrSkillDash
	case !s.surfacesValid():
		return ErrSkillSurface
	}
	return nil
}
