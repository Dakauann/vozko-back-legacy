package sitesubmission

import "testing"

func TestASubmissionSummarisesWhoSentItAndWhereTheyCameFrom(t *testing.T) {
	cases := map[string]struct {
		sub                   Submission
		name, contact, source string
	}{
		"phone wins over email": {
			Submission{Fields: map[string]string{"lead.name": "  Ana Souza ", "lead.phone": "+55 11 99999-0000", "lead.email": "ana@example.com", "field.cidade": "Recife"}, UTM: UTM{Source: "instagram"}, ReferrerOrigin: "https://www.google.com"},
			"Ana Souza", "+55 11 99999-0000", "instagram",
		},
		"email when there is no phone": {
			Submission{Fields: map[string]string{"lead.email": "bia@example.com", "lead.phone": "  "}, ReferrerOrigin: "https://www.google.com"},
			"", "bia@example.com", "https://www.google.com",
		},
		"nothing known": {
			Submission{Fields: map[string]string{"field.mensagem": "oi"}},
			"", "", "",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := c.sub.Name(); got != c.name {
				t.Fatalf("name %q", got)
			}
			if got := c.sub.Contact(); got != c.contact {
				t.Fatalf("contact %q", got)
			}
			if got := c.sub.Source(); got != c.source {
				t.Fatalf("source %q", got)
			}
		})
	}
}
