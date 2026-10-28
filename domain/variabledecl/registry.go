package variabledecl

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

type Registry struct {
	rules map[Kind]Rule
	order []Kind
}

func NewRegistry(rules ...Rule) (*Registry, error) {
	r := &Registry{rules: make(map[Kind]Rule, len(rules))}
	for _, rule := range rules {
		if _, taken := r.rules[rule.Kind()]; taken {
			return nil, fmt.Errorf("%w: %s", ErrKindTaken, rule.Kind())
		}
		r.rules[rule.Kind()] = rule
		r.order = append(r.order, rule.Kind())
	}
	return r, nil
}

func (r *Registry) Kinds() []Kind {
	return append([]Kind(nil), r.order...)
}

func (r *Registry) Rule(kind Kind) (Rule, bool) {
	rule, ok := r.rules[kind]
	return rule, ok
}

func (r *Registry) ShapeOf(t Type) Shape {
	if rule, ok := r.rules[t.Kind]; ok {
		return rule.Shape()
	}
	return ""
}

func (r *Registry) CheckDeclaration(d Declaration) (Declaration, error) {
	d = normalized(d)
	if err := checkPresentation(d); err != nil {
		return Declaration{}, err
	}
	rule, ok := r.rules[d.Type.Kind]
	if !ok {
		return Declaration{}, fmt.Errorf("%w: %q", ErrKindUnknown, d.Type.Kind)
	}
	if err := rule.CheckType(d.Type); err != nil {
		return Declaration{}, err
	}
	if d.Example != "" {
		if err := rule.Accepts(context.Background(), Check{Name: d.Name, Type: d.Type, Value: d.Example}); err != nil {
			return Declaration{}, fmt.Errorf("%w: %w", ErrExampleInvalid, err)
		}
	}
	return d, nil
}

func (r *Registry) ExampleOf(d Declaration) string {
	if d.Example != "" {
		return d.Example
	}
	if rule, ok := r.rules[d.Type.Kind]; ok {
		return rule.Sample(d.Type)
	}
	return ""
}

func (r *Registry) Described(declarations []Declaration) []Declaration {
	out := make([]Declaration, len(declarations))
	for i, d := range declarations {
		d.Example = r.ExampleOf(d)
		out[i] = d
	}
	return out
}

func (r *Registry) Accepts(ctx context.Context, c Check) error {
	rule, ok := r.rules[c.Type.Kind]
	if !ok {
		return fmt.Errorf("%w: %q", ErrKindUnknown, c.Type.Kind)
	}
	return rule.Accepts(ctx, c)
}

func (r *Registry) Rejected(ctx context.Context, workspaceID string, catalog Catalog, values map[string]string) ([]string, error) {
	var rejected []string
	for name, value := range values {
		typ := Type{Kind: KindText}
		if d, ok := catalog.Find(name); ok {
			typ = d.Type
		}
		err := r.Accepts(ctx, Check{WorkspaceID: workspaceID, Name: name, Type: typ, Value: value})
		switch {
		case err == nil:
		case errors.Is(err, ErrValueInvalid):
			rejected = append(rejected, name)
		default:
			return nil, err
		}
	}
	sort.Strings(rejected)
	return rejected, nil
}
