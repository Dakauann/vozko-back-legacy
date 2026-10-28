package workflow

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"vozko/domain/variabledecl"
	"vozko/pkg/codegolden"
)

var updateVariableGoldens = flag.Bool("update-variables", false, "rewrite the variable goldens the front reads")

var labelKeyLiteral = regexp.MustCompile(`(?:keyed\([^\n]*?match\w+, "|LabelKey: "|messageEventProvisions\(np, ")([a-z_]+)"`)

func declaredLabelKeys(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range labelKeyLiteral.FindAllStringSubmatch(string(raw), -1) {
			keys = append(keys, m[1])
		}
	}
	for _, name := range variabledecl.SystemKeys() {
		keys = append(keys, variabledecl.SystemLabelKey(name))
	}
	return keys
}

func TestEveryVariableLabelKeyIsListedSoTheFrontTranslatesIt(t *testing.T) {
	golden := filepath.Join("testdata", "variable_label_keys.golden")
	keys := declaredLabelKeys(t)
	if *updateVariableGoldens {
		if err := os.WriteFile(golden, []byte(strings.Join(codegolden.Unique(keys), "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	codegolden.Compare(t, golden, keys, "run go test -run VariableLabelKey -update-variables and translate workflowsPage.variableLabels.<key> in the pt, en, es and de catalogs")
}

type templateRefCase struct {
	Text     string             `json:"text"`
	Segments []templateRefPiece `json:"segments"`
}

type templateRefPiece struct {
	Text  string `json:"text"`
	Ref   string `json:"ref,omitempty"`
	Label string `json:"label,omitempty"`
}

type templateRefGolden struct {
	Variables []CatalogVariable `json:"variables"`
	Cases     []templateRefCase `json:"cases"`
}

func TestTheTemplateParserAndLabelsAreSharedWithTheFront(t *testing.T) {
	g := conversationGraph()
	catalog := VariableCatalogAt(g, catalogDefs(), VariableSources{Specs: TriggerInputSpecsOf(g, TriggerInputSources{}), Declarations: conversationDeclarations}, "s")
	texts := []string{
		"Oi {{campvars.matricula}}, tudo bem?",
		"{{ campvars.matricula }} e {{var.campvars.apelido}}",
		"Status {{cadastro.status}} de {{campvars[lead_name]}}",
		"{{node.h.status_code}}{{sys.date}}",
		"sem nada",
		"{{ desconhecida.coisa }} e {{x y}} e {{}}",
		"{{var.saudacao}}!",
	}
	stable := make([]CatalogVariable, 0, len(catalog.Variables))
	for _, v := range catalog.Variables {
		if strings.HasPrefix(v.Ref, "sys.") {
			v.Example = nil
		}
		stable = append(stable, v)
	}
	out := templateRefGolden{Variables: stable}
	for _, text := range texts {
		c := templateRefCase{Text: text}
		for _, seg := range TemplateSegments(text) {
			piece := templateRefPiece{Text: seg.Text, Ref: seg.Ref}
			if seg.Ref != "" {
				piece.Label, _ = catalog.LabelOf(seg.Ref)
			}
			c.Segments = append(c.Segments, piece)
		}
		out.Cases = append(out.Cases, c)
	}
	got, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("testdata", "template_refs.golden.json")
	if *updateVariableGoldens {
		if err := os.WriteFile(golden, append(got, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ReplaceAll(string(want), "\r\n", "\n") != string(got)+"\n" {
		t.Fatalf("the template golden changed; run go test -run TemplateParser -update-variables and check the front test lib/workflows/variable-catalog.test.ts")
	}
}
