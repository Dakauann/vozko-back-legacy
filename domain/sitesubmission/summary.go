package sitesubmission

import (
	"strings"

	"vozko/domain/siteform"
)

func (s Submission) field(target string) string {
	return strings.TrimSpace(s.Fields[target])
}

func (s Submission) Name() string {
	return s.field(siteform.TargetName)
}

func (s Submission) Contact() string {
	if phone := s.field(siteform.TargetPhone); phone != "" {
		return phone
	}
	return s.field(siteform.TargetEmail)
}

func (s Submission) Source() string {
	if source := strings.TrimSpace(s.UTM.Source); source != "" {
		return source
	}
	return s.ReferrerOrigin
}
