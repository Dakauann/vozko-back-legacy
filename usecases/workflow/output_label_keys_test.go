package workflow_usecase

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"vozko/domain/workflow"
)

var updateOutputLabels = flag.Bool("update-output-labels", false, "rewrite testdata/node_output_labels.golden.json")

func TestEveryNodeOutputLabelKeyIsListedSoTheFrontTranslatesIt(t *testing.T) {
	labels := map[string]string{}
	owners := map[string]string{}
	for _, def := range productionCatalog(t) {
		for _, out := range def.OutputKeys {
			key := workflow.OutputLabelKey(out)
			if key == "" {
				continue
			}
			if seen, ok := labels[key]; ok && seen != out.Label {
				t.Errorf("label key %s means %q in %s and %q in %s; give one of them its own LabelKey", key, seen, owners[key], out.Label, def.Type)
				continue
			}
			labels[key], owners[key] = out.Label, string(def.Type)
		}
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ordered := make([][2]string, 0, len(keys))
	for _, key := range keys {
		ordered = append(ordered, [2]string{key, labels[key]})
	}
	got, err := json.MarshalIndent(ordered, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("testdata", "node_output_labels.golden.json")
	if *updateOutputLabels {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, append(got, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ReplaceAll(string(want), "\r\n", "\n") != string(got)+"\n" {
		t.Fatalf("node output labels changed; run go test -run NodeOutputLabelKey -update-output-labels and translate workflowsPage.outputLabels.<key> in the pt, en, es and de catalogs")
	}
}
