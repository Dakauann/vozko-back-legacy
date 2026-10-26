package crmfilter

func (f Filter) Hoist(field Field) ([]Predicate, Filter) {
	var hoisted []Predicate
	groups := make([]Group, 0, len(f.Groups))
	for _, g := range f.Groups {
		if g.Conj() != And && len(g.Predicates) > 1 {
			groups = append(groups, g)
			continue
		}
		kept := make([]Predicate, 0, len(g.Predicates))
		for _, p := range g.Predicates {
			if p.Field == field {
				hoisted = append(hoisted, p)
			} else {
				kept = append(kept, p)
			}
		}
		if len(kept) > 0 {
			g.Predicates = kept
			groups = append(groups, g)
		}
	}
	if hoisted == nil {
		return nil, f
	}
	return hoisted, Filter{Groups: groups}
}
