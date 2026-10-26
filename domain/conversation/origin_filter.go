package conversation

import (
	"fmt"
	"slices"
	"strings"

	"vozko/domain/crmfilter"
)

func OriginScope(origin Origin) []Origin {
	if origin == "" {
		return nil
	}
	return []Origin{origin}
}

func OriginsAllowedBy(p crmfilter.Predicate) ([]Origin, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	var named []Origin
	for _, raw := range p.Values {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		origin, err := ParseOrigin(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("%w: %w", crmfilter.ErrInvalidValue, err)
		}
		named = append(named, origin)
	}
	switch p.Operator {
	case crmfilter.OpIn, crmfilter.OpEquals:
		return named, nil
	case crmfilter.OpNotIn, crmfilter.OpNotEquals:
		return slices.DeleteFunc(Origins(), func(o Origin) bool { return slices.Contains(named, o) }), nil
	}
	return nil, fmt.Errorf("%w: %q on %q", crmfilter.ErrUnsupportedOp, p.Operator, p.Field)
}

func OriginScopeOf(f crmfilter.Filter) ([]Origin, crmfilter.Filter, error) {
	hoisted, rest := f.Hoist(crmfilter.FieldOrigin)
	if len(hoisted) == 0 {
		return nil, rest, nil
	}
	allowed := Origins()
	for _, p := range hoisted {
		named, err := OriginsAllowedBy(p)
		if err != nil {
			return nil, crmfilter.Filter{}, err
		}
		allowed = slices.DeleteFunc(allowed, func(o Origin) bool { return !slices.Contains(named, o) })
	}
	return allowed, rest, nil
}
