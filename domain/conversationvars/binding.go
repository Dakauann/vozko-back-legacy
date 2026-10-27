package conversationvars

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"vozko/domain/automationneeds"
	"vozko/domain/campaign"
	"vozko/domain/customfield"
)

const SourceCurrent campaign.BindingSource = "conversation"

var (
	ErrSourceInvalid = errors.New("conversation variables: the source is not one a variable can take")
	ErrValueEmpty    = errors.New("conversation variables: the chosen source has no value for this conversation")
)

type Binding struct {
	Name   string
	Source campaign.BindingSource
	Value  string
}

type VariablesError struct {
	Cause error
	Names []string
}

func (e *VariablesError) Error() string {
	return fmt.Sprintf("%v: %s", e.Cause, strings.Join(e.Names, ", "))
}

func (e *VariablesError) Unwrap() error { return e.Cause }

func LiteralBindings(values map[string]string) []Binding {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	slices.Sort(names)
	out := make([]Binding, 0, len(names))
	for _, name := range names {
		out = append(out, Binding{Name: name, Source: campaign.BindLiteral, Value: values[name]})
	}
	return out
}

type Assignment struct {
	names []string
	bound []string
	kept  []string
	plan  campaign.BindingPlan
}

func Assign(bindings []Binding, defs []*customfield.Definition) (Assignment, error) {
	if len(bindings) == 0 {
		return Assignment{}, ErrNothingToSave
	}
	var a Assignment
	var planned []campaign.VariableBinding
	refused := map[error][]string{}
	seen := map[string]bool{}
	for _, b := range bindings {
		name := strings.TrimSpace(b.Name)
		if err := checkName(name); err != nil {
			return Assignment{}, err
		}
		if seen[name] {
			return Assignment{}, fmt.Errorf("%w: %q comes twice", ErrNameInvalid, name)
		}
		seen[name] = true
		a.names = append(a.names, name)
		binding := campaign.VariableBinding{Source: campaign.BindingSource(strings.TrimSpace(string(b.Source))), Value: b.Value}
		if cause := checkBinding(binding, defs); cause != nil {
			refused[cause] = append(refused[cause], name)
			continue
		}
		if binding.Source == SourceCurrent {
			a.kept = append(a.kept, name)
			continue
		}
		a.bound = append(a.bound, name)
		planned = append(planned, binding)
	}
	for _, cause := range []error{ErrSourceInvalid, ErrValueEmpty, ErrValueInvalid} {
		if names := refused[cause]; len(names) > 0 {
			return Assignment{}, &VariablesError{Cause: cause, Names: names}
		}
	}
	plan, err := campaign.PlanBindings(planned, len(planned), defs)
	if err != nil {
		return Assignment{}, fmt.Errorf("%w: %w", ErrSourceInvalid, err)
	}
	a.plan = plan
	return a, nil
}

func checkName(name string) error {
	if !namePattern.MatchString(name) || utf8.RuneCountInString(name) > MaxNameRunes {
		return fmt.Errorf("%w: %q", ErrNameInvalid, name)
	}
	if automationneeds.IsSystemKey(name) {
		return fmt.Errorf("%w: %q", ErrNameReserved, name)
	}
	return nil
}

func checkBinding(b campaign.VariableBinding, defs []*customfield.Definition) error {
	if b.Source == SourceCurrent {
		if strings.TrimSpace(b.Value) != "" {
			return ErrSourceInvalid
		}
		return nil
	}
	_, err := campaign.PlanBindings([]campaign.VariableBinding{b}, 1, defs)
	switch {
	case errors.Is(err, campaign.ErrBindingLiteralEmpty):
		return ErrValueEmpty
	case err != nil:
		return ErrSourceInvalid
	case b.Source == campaign.BindLiteral && tooLong(b.Value):
		return ErrValueInvalid
	}
	return nil
}

func tooLong(value string) bool {
	return utf8.RuneCountInString(strings.TrimSpace(value)) > MaxValueRunes
}

func (a Assignment) Names() []string { return slices.Clone(a.names) }

func (a Assignment) Plan() campaign.BindingPlan { return a.plan }

func (a Assignment) Resolve(subject campaign.BindingSubject, current map[string]string) (map[string]string, error) {
	values := a.plan.Values(subject)
	refused := map[error][]string{}
	out := make(map[string]string, len(a.bound))
	for i, name := range a.bound {
		switch value := values[i]; {
		case value == "":
			refused[ErrValueEmpty] = append(refused[ErrValueEmpty], name)
		case tooLong(value):
			refused[ErrValueInvalid] = append(refused[ErrValueInvalid], name)
		default:
			out[name] = value
		}
	}
	for _, name := range a.kept {
		if strings.TrimSpace(current[name]) == "" {
			refused[ErrValueEmpty] = append(refused[ErrValueEmpty], name)
		}
	}
	for _, cause := range []error{ErrValueEmpty, ErrValueInvalid} {
		if names := refused[cause]; len(names) > 0 {
			return nil, &VariablesError{Cause: cause, Names: a.ordered(names)}
		}
	}
	return out, nil
}

func (a Assignment) ordered(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range a.names {
		if slices.Contains(names, name) {
			out = append(out, name)
		}
	}
	return out
}
