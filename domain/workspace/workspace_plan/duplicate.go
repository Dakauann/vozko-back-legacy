package workspace_plan

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	PlanNameMaxRunes      = 120
	DuplicateKeyMaxRunes  = 128
	DuplicateNameAttempts = 1000
	duplicateNameWord     = "cópia"
)

var (
	ErrDuplicateNameUnavailable   = errors.New("every copy name for this plan is taken")
	ErrDuplicateKeyRequired       = errors.New("an Idempotency-Key header of at most 128 characters is required")
	ErrDuplicateKeyReused         = errors.New("this Idempotency-Key already duplicated a different plan")
	ErrDuplicateRequesterRequired = errors.New("the person duplicating the plan is unknown")
	ErrPlanDuplicationNotFound    = errors.New("plan duplication not found")
	ErrPlanDuplicationExists      = errors.New("a plan duplication with this Idempotency-Key already exists")
)

var duplicateNameSuffix = regexp.MustCompile(`\s*\(` + duplicateNameWord + `(?: \d+)?\)$`)

type DuplicatePlanInput struct {
	SourcePlanID   string
	RequestedBy    string
	IdempotencyKey string
}

func (in DuplicatePlanInput) Validate() error {
	if strings.TrimSpace(in.SourcePlanID) == "" {
		return ErrPlanNotFound
	}
	if strings.TrimSpace(in.RequestedBy) == "" {
		return ErrDuplicateRequesterRequired
	}
	key := strings.TrimSpace(in.IdempotencyKey)
	if key == "" || utf8.RuneCountInString(key) > DuplicateKeyMaxRunes {
		return ErrDuplicateKeyRequired
	}
	return nil
}

type DuplicatePlanResult struct {
	Plan     *PlanDefinition
	Replayed bool
}

type PlanDuplicatePreview struct {
	Name string `json:"name"`
}

type PlanDuplication struct {
	SourcePlanID   string
	PlanID         string
	RequestedBy    string
	IdempotencyKey string
	CreatedAt      time.Time
}

type PlanNamer func(taken []string) (string, error)

type PlanDuplicationRepository interface {
	PlanNames() ([]string, error)
	FindDuplication(requestedBy, idempotencyKey string) (*PlanDuplication, error)
	CreateDuplicate(plan *PlanDefinition, record PlanDuplication, name PlanNamer) error
}

type DuplicatePlanDefinitionUseCase interface {
	Execute(input DuplicatePlanInput) (*DuplicatePlanResult, error)
}

type PreviewPlanDuplicateUseCase interface {
	Execute(sourcePlanID string) (*PlanDuplicatePreview, error)
}

func (p *PlanDefinition) Duplicate(newID string) *PlanDefinition {
	items := make([]PlanPricingItem, len(p.PricingItems))
	for i, item := range p.PricingItems {
		items[i] = PlanPricingItem{
			PlanDefinitionID: newID,
			Category:         item.Category,
			Service:          item.Service,
			Metric:           item.Metric,
			CostMicros:       item.CostMicros,
			PriceMicros:      item.PriceMicros,
			MarkupPct:        item.MarkupPct,
			Currency:         item.Currency,
		}
	}
	return &PlanDefinition{
		ID:                             newID,
		Name:                           p.Name,
		Description:                    p.Description,
		BasePriceBRLCents:              p.BasePriceBRLCents,
		MaxCallChannels:                p.MaxCallChannels,
		IncludedWhatsAppBusinessPhones: p.IncludedWhatsAppBusinessPhones,
		MaxBranches:                    p.MaxBranches,
		IsGloballyVisible:              false,
		PricingItems:                   items,
	}
}

func DuplicateName(source string, taken []string) (string, error) {
	whole := strings.TrimSpace(source)
	if whole == "" {
		return "", ErrInvalidPlanName
	}
	base := strings.TrimSpace(duplicateNameSuffix.ReplaceAllString(whole, ""))
	if base == "" {
		base = whole
	}
	used := make(map[string]struct{}, len(taken))
	for _, name := range taken {
		used[comparableName(name)] = struct{}{}
	}
	for n := 1; n <= DuplicateNameAttempts; n++ {
		candidate := copyName(base, n)
		if _, clash := used[comparableName(candidate)]; !clash {
			return candidate, nil
		}
	}
	return "", ErrDuplicateNameUnavailable
}

func copyName(base string, n int) string {
	suffix := " (" + duplicateNameWord + ")"
	if n > 1 {
		suffix = " (" + duplicateNameWord + " " + strconv.Itoa(n) + ")"
	}
	room := PlanNameMaxRunes - utf8.RuneCountInString(suffix)
	if runes := []rune(base); len(runes) > room {
		base = strings.TrimSpace(string(runes[:room]))
	}
	return base + suffix
}

func comparableName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
