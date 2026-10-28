package conversation

import (
	"strings"
	"testing"
)

func TestANewPageReferenceUsesTheUnambiguousAlphabet(t *testing.T) {
	seen := map[string]bool{}
	for range 200 {
		code, err := NewPageReference()
		if err != nil {
			t.Fatal(err)
		}
		if len(code) != PageReferenceLength || strings.ContainsAny(code, "01IO") {
			t.Fatalf("code %q", code)
		}
		if found, ok := FindPageReference(PrefilledText("Oi", code)); !ok || found != code {
			t.Fatalf("FindPageReference(%q) = %q, %v", PrefilledText("Oi", code), found, ok)
		}
		seen[code] = true
	}
	if len(seen) < 190 {
		t.Fatalf("only %d distinct codes in 200", len(seen))
	}
}

func TestPrefilledTextEndsWithTheVisibleReference(t *testing.T) {
	cases := map[string]struct {
		text, code, want string
	}{
		"with a message": {"Quero saber mais", "LP7K2", "Quero saber mais (ref LP7K2)"},
		"spaces trimmed": {"  Oi  ", "LP7K2", "Oi (ref LP7K2)"},
		"no message":     {"", "LP7K2", "(ref LP7K2)"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := PrefilledText(tc.text, tc.code); got != tc.want {
				t.Fatalf("PrefilledText = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFindPageReferenceReadsWhatPeopleSend(t *testing.T) {
	cases := map[string]struct {
		text string
		want string
		ok   bool
	}{
		"untouched":            {"Quero saber mais (ref LP7K2)", "LP7K2", true},
		"lower case typed":     {"oi (REF lp7k2)", "LP7K2", true},
		"text after it":        {"(ref LP7K2) e mais uma coisa", "LP7K2", true},
		"no reference":         {"Quero saber mais", "", false},
		"ambiguous letters":    {"(ref LP0K1)", "", false},
		"too short":            {"(ref LP7K)", "", false},
		"too long":             {"(ref LP7K22)", "", false},
		"missing parenthesis":  {"ref LP7K2", "", false},
		"first one wins":       {"(ref AAAAA) (ref BBBBB)", "AAAAA", true},
		"empty message":        {"", "", false},
		"only spaces inside":   {"( ref LP7K2 )", "LP7K2", true},
		"no space after ref":   {"(refLP7K2)", "", false},
		"line break before it": {"Oi\n(ref LP7K2)", "LP7K2", true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, ok := FindPageReference(tc.text)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("FindPageReference(%q) = %q, %v; want %q, %v", tc.text, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestAPageOriginNeedsItsConversationAndPage(t *testing.T) {
	full := PageOrigin{EntryID: "e1", EntryType: "whatsapp", WorkspaceID: "w1", ProjectID: "p1", Reference: "LP7K2", Route: "/"}
	if !full.Complete() {
		t.Fatal("a full origin must be complete")
	}
	for name, broken := range map[string]PageOrigin{
		"no entry":     {EntryType: "whatsapp", WorkspaceID: "w1", ProjectID: "p1", Reference: "LP7K2"},
		"no type":      {EntryID: "e1", WorkspaceID: "w1", ProjectID: "p1", Reference: "LP7K2"},
		"no workspace": {EntryID: "e1", EntryType: "whatsapp", ProjectID: "p1", Reference: "LP7K2"},
		"no project":   {EntryID: "e1", EntryType: "whatsapp", WorkspaceID: "w1", Reference: "LP7K2"},
		"no reference": {EntryID: "e1", EntryType: "whatsapp", WorkspaceID: "w1", ProjectID: "p1"},
	} {
		if broken.Complete() {
			t.Fatalf("%s: must not be complete", name)
		}
	}
}
