package automationneedshttp

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domain "vozko/domain/automationneeds"
)

var updateCodes = flag.Bool("update", false, "rewrite testdata/error_codes.golden")

func TestEveryCodeTheFrontMustTranslateIsListed(t *testing.T) {
	codes := append(domain.ErrorCodes(), ValueCodes()...)
	seen := map[string]bool{}
	var lines []string
	for _, code := range codes {
		if !seen[code] {
			seen[code] = true
			lines = append(lines, code)
		}
	}
	got := strings.Join(lines, "\n") + "\n"
	golden := filepath.Join("testdata", "error_codes.golden")
	if *updateCodes {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Fatalf("codes changed; translate them in the four catalogs and run with -update:\n%s", got)
	}
}
