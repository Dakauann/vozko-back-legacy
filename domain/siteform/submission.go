package siteform

import (
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"vozko/domain/emailaddress"
	"vozko/domain/shared"
)

const (
	CodeFieldRequired    = "form_field_required"
	CodeValueTooLong     = "form_value_too_long"
	CodeValueInvalid     = "form_value_invalid"
	CodeOptionInvalid    = "form_option_invalid"
	CodePhoneInvalid     = "form_phone_invalid"
	CodeEmailInvalid     = "form_email_invalid"
	CodeConsentRequired  = "consent_required"
	CodeIdentityMissing  = "form_identity_missing"
	checkboxCheckedValue = "true"
)

var ErrSubmissionInvalid = errors.New("siteform: the submission does not match the published form")

type SubmissionError struct {
	Problems []Problem
}

func (e *SubmissionError) Error() string {
	codes := make([]string, 0, len(e.Problems))
	for _, p := range e.Problems {
		codes = append(codes, p.Field+": "+p.Code)
	}
	return ErrSubmissionInvalid.Error() + " (" + strings.Join(codes, ", ") + ")"
}

func (e *SubmissionError) Is(target error) bool {
	return target == ErrSubmissionInvalid
}

type Values struct {
	Name       string
	Phone      string
	Email      string
	Fields     map[string]string
	Consent    bool
	EmailOptIn bool
}

func (m Manifest) Validate(submission map[string]string) (Values, error) {
	var problems []Problem
	refuse := func(field, code string) {
		problems = append(problems, Problem{Code: code, Form: m.ID, Field: field})
	}
	unknown := []string{}
	for name := range submission {
		if _, ok := m.Field(name); !ok {
			unknown = append(unknown, name)
		}
	}
	slices.Sort(unknown)
	for _, name := range unknown {
		refuse(name, CodeFormFieldUnknown)
	}
	values := Values{Fields: map[string]string{}}
	for _, f := range m.Fields {
		raw := strings.TrimSpace(submission[f.Name])
		if raw == "" {
			switch {
			case f.Name == TargetConsent:
				refuse(f.Name, CodeConsentRequired)
			case f.Required:
				refuse(f.Name, CodeFieldRequired)
			}
			continue
		}
		value, code := f.normalize(raw)
		if code != "" {
			if f.Name == TargetConsent && code == CodeValueInvalid {
				code = CodeConsentRequired
			}
			refuse(f.Name, code)
			continue
		}
		values.assign(f.Name, value)
	}
	if m.HasIdentity() && values.Phone == "" && values.Email == "" && !hasProblemOn(problems, TargetPhone, TargetEmail) {
		refuse("", CodeIdentityMissing)
	}
	if len(problems) > 0 {
		return Values{}, &SubmissionError{Problems: problems}
	}
	return values, nil
}

func hasProblemOn(problems []Problem, fields ...string) bool {
	for _, p := range problems {
		if slices.Contains(fields, p.Field) {
			return true
		}
	}
	return false
}

func (v *Values) assign(name, value string) {
	switch name {
	case TargetName:
		v.Name = value
	case TargetPhone:
		v.Phone = value
	case TargetEmail:
		v.Email = value
	case TargetConsent:
		v.Consent = true
	case TargetEmailOptIn:
		v.EmailOptIn = true
	default:
		v.Fields[strings.TrimPrefix(name, FieldPrefix)] = value
	}
}

func (f Field) limit() int {
	if f.MaxLength > 0 {
		return f.MaxLength
	}
	if f.Kind == KindTextarea {
		return maxLongTextLength
	}
	return maxTextLength
}

func (f Field) normalize(raw string) (string, string) {
	if utf8.RuneCountInString(raw) > f.limit() {
		return "", CodeValueTooLong
	}
	switch f.Name {
	case TargetPhone:
		phone, err := shared.ParsePhone(raw)
		if err != nil {
			return "", CodePhoneInvalid
		}
		return phone, ""
	case TargetEmail:
		address, err := emailaddress.Normalize(raw)
		if err != nil {
			return "", CodeEmailInvalid
		}
		return string(address), ""
	}
	switch f.Kind {
	case KindCheckbox:
		if raw == "on" || raw == checkboxCheckedValue || (f.Value != "" && raw == f.Value) {
			return checkboxCheckedValue, ""
		}
		return "", CodeValueInvalid
	case KindRadio, KindSelect:
		if !slices.Contains(f.Options, raw) {
			return "", CodeOptionInvalid
		}
	case KindHidden:
		if raw != f.Value {
			return "", CodeValueInvalid
		}
	case KindNumber:
		if _, err := strconv.ParseFloat(raw, 64); err != nil {
			return "", CodeValueInvalid
		}
	case KindDate:
		if _, err := time.Parse(time.DateOnly, raw); err != nil {
			return "", CodeValueInvalid
		}
	case KindURL:
		parsed, err := url.Parse(raw)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return "", CodeValueInvalid
		}
	case KindEmail:
		address, err := emailaddress.Normalize(raw)
		if err != nil {
			return "", CodeEmailInvalid
		}
		return string(address), ""
	case KindTel:
		phone, err := shared.ParsePhone(raw)
		if err != nil {
			return "", CodePhoneInvalid
		}
		return phone, ""
	}
	return raw, ""
}
