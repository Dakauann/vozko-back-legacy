package siteform

import (
	"errors"
	"strings"
	"testing"
)

func contactManifest(t *testing.T) Manifest {
	t.Helper()
	manifests, problems := Extract(parse(t, contactForm))
	if len(problems) != 0 || len(manifests) != 1 {
		t.Fatalf("got %+v %+v", manifests, problems)
	}
	return manifests[0]
}

func TestValidateCanonicalisesAnAcceptedSubmission(t *testing.T) {
	values, err := contactManifest(t).Validate(map[string]string{
		"lead.name":       "  Ana Souza ",
		"lead.phone":      "(11) 98765-4321",
		"lead.email":      " Ana@Example.COM ",
		"field.interesse": "implante",
		"field.periodo":   "tarde",
		"field.obs":       "",
		"field.origem":    "landing",
		"consent":         "on",
		"email_opt_in":    "true",
	})
	if err != nil {
		t.Fatal(err)
	}
	if values.Name != "Ana Souza" || values.Phone != "5511987654321" || values.Email != "ana@example.com" || !values.Consent || !values.EmailOptIn {
		t.Fatalf("got %+v", values)
	}
	if len(values.Fields) != 3 || values.Fields["interesse"] != "implante" || values.Fields["periodo"] != "tarde" || values.Fields["origem"] != "landing" {
		t.Fatalf("got %+v", values.Fields)
	}
}

func TestValidateRefusesEveryProblemAtOnce(t *testing.T) {
	_, err := contactManifest(t).Validate(map[string]string{
		"lead.name":       strings.Repeat("a", 81),
		"lead.phone":      "123",
		"lead.email":      "ana@",
		"field.interesse": "ortodontia",
		"field.origem":    "outro",
		"cpf":             "000",
		"email_opt_in":    "talvez",
	})
	var invalid *SubmissionError
	if !errors.Is(err, ErrSubmissionInvalid) || !errors.As(err, &invalid) {
		t.Fatalf("got %v", err)
	}
	got := []string{}
	for _, p := range invalid.Problems {
		got = append(got, p.Field+":"+p.Code)
	}
	want := "cpf:form_field_unknown,lead.name:form_value_too_long,lead.phone:form_phone_invalid,lead.email:form_email_invalid,field.interesse:form_option_invalid,field.origem:form_value_invalid,consent:consent_required,email_opt_in:form_value_invalid"
	if strings.Join(got, ",") != want {
		t.Fatalf("got  %s\nwant %s", strings.Join(got, ","), want)
	}
}

func TestValidateEnforcesRequiredFieldsAndIdentity(t *testing.T) {
	m, problems := Extract(parse(t, `<form data-vz-form="a"><input name="lead.name"><input type="tel" name="lead.phone"><input type="email" name="lead.email"><input name="field.cidade" required></form>`))
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	_, err := m[0].Validate(map[string]string{"lead.name": "Ana", "lead.phone": "  ", "field.cidade": " "})
	var invalid *SubmissionError
	if !errors.As(err, &invalid) {
		t.Fatalf("got %v", err)
	}
	got := []string{}
	for _, p := range invalid.Problems {
		got = append(got, p.Field+":"+p.Code)
	}
	if strings.Join(got, ",") != "field.cidade:form_field_required,:form_identity_missing" {
		t.Fatalf("got %v", got)
	}
}

func TestValidateChecksTypedValues(t *testing.T) {
	m, _ := Extract(parse(t, `<form data-vz-form="a"><input type="email" name="lead.email" required><input type="number" name="field.idade"><input type="date" name="field.nascimento"><input type="url" name="field.site"><input type="checkbox" name="field.aceita_ligacao" value="sim"></form>`))
	values, err := m[0].Validate(map[string]string{"lead.email": "a@b.co", "field.idade": "42", "field.nascimento": "1990-02-28", "field.site": "https://loja.example.com", "field.aceita_ligacao": "sim"})
	if err != nil {
		t.Fatal(err)
	}
	if values.Fields["idade"] != "42" || values.Fields["nascimento"] != "1990-02-28" || values.Fields["aceita_ligacao"] != "true" {
		t.Fatalf("got %+v", values.Fields)
	}
	for field, bad := range map[string]string{"field.idade": "quarenta", "field.nascimento": "1990-02-30", "field.site": "javascript:alert(1)", "field.aceita_ligacao": "nao"} {
		sub := map[string]string{"lead.email": "a@b.co", field: bad}
		if _, err := m[0].Validate(sub); !errors.Is(err, ErrSubmissionInvalid) {
			t.Fatalf("%s=%q: got %v", field, bad, err)
		}
	}
}
