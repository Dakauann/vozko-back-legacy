package sitebuilder

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	"vozko/domain/site"
)

type packageFile struct {
	Dependencies map[string]string `json:"dependencies"`
}

type vendorEntries struct {
	Modules map[string][]string `json:"modules"`
	Styles  map[string][]string `json:"styles"`
	Inline  []string            `json:"inline"`
	HTML    []string            `json:"html"`
}

func readJSON(t *testing.T, path string, into any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatal(err)
	}
}

func TestTheBuilderVendorsEveryAllowedLibraryAtItsPinnedVersion(t *testing.T) {
	var pkg packageFile
	readJSON(t, "../../services/sitebuilder/package.json", &pkg)
	var entries vendorEntries
	readJSON(t, "../../services/sitebuilder/src/vendor-entries.json", &entries)
	for _, lib := range site.Libraries() {
		if pkg.Dependencies[lib.Package] != lib.Version {
			t.Errorf("%s: package.json pins %q, the allow list %q", lib.Package, pkg.Dependencies[lib.Package], lib.Version)
		}
		if len(entries.Modules[lib.Package]) == 0 && !slices.Contains(entries.Inline, lib.Package) {
			t.Errorf("%s has no vendored entry", lib.Package)
		}
		if slices.Contains(lib.Stacks, site.StackHTML) != slices.Contains(entries.HTML, lib.Package) {
			t.Errorf("%s: the html stack list in the builder disagrees with the allow list", lib.Package)
		}
	}
	allowed := map[string]bool{}
	for _, lib := range site.Libraries() {
		allowed[lib.Package] = true
	}
	for name := range entries.Modules {
		if !allowed[name] {
			t.Errorf("the builder vendors %s, which is not in the allow list", name)
		}
	}
	for name := range entries.Styles {
		if !allowed[name] {
			t.Errorf("the builder vendors styles of %s, which is not in the allow list", name)
		}
	}
}
