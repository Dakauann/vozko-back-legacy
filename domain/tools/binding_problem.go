package tools

import "errors"

type BindingProblem struct {
	Code    string
	Message string
}

func (p *BindingProblem) Error() string { return p.Message }

func Invalid(code, message string) error {
	return &BindingProblem{Code: code, Message: message}
}

func ProblemOf(err error) (*BindingProblem, bool) {
	var problem *BindingProblem
	if errors.As(err, &problem) {
		return problem, true
	}
	return nil, false
}

type ConfigNormalizer interface {
	NormalizeConfig(config map[string]interface{}) (map[string]interface{}, error)
}
