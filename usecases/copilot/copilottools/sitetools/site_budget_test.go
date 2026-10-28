package sitetools

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"vozko/domain/copilot"
	"vozko/domain/tools"
	"vozko/usecases/copilot/copilottools/attendancetools"
	"vozko/usecases/copilot/copilottools/platformtools"
	"vozko/usecases/copilot/copilottools/skilltools"
	"vozko/usecases/copilot/copilottools/tooltest"
	"vozko/usecases/copilot/skills"
)

const bytesPerToken = 2.78

const (
	siteSchemaTokenBudget = 10000
	siteIndexTokenBudget  = 1500
	siteSkillTokenBudget  = 3000
	siteMapTokenBudget    = 2000
	siteEditTokenBudget   = 1500
	siteLookTokenBudget   = 1000
	siteFieldBudget       = 200
)

func tokensOf(chars int) int {
	return int(float64(chars)/bytesPerToken + 0.5)
}

func siteViewTools(t *testing.T) []tools.Definition {
	t.Helper()
	library, err := skills.Load()
	if err != nil {
		t.Fatal(err)
	}
	candidates := append(SiteTools(Deps{}), skilltools.NewLoadSkillTool(library), platformtools.NewAskUserTool(), attendancetools.NewCalculateTool())
	var defs []tools.Definition
	for _, tool := range candidates {
		if siteView.Offers(tool) {
			defs = append(defs, copilot.DefinitionOn(tool, siteView))
		}
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].Name < defs[j].Name })
	return defs
}

func TestTheSiteToolSchemasFitTheirBudget(t *testing.T) {
	total := 0
	var sizes []string
	for _, def := range siteViewTools(t) {
		size := tooltest.SchemaChars(def)
		total += size
		sizes = append(sizes, def.Name+" "+itoa(tokensOf(size)))
	}
	t.Logf("site schema tokens: %d (%s)", tokensOf(total), strings.Join(sizes, ", "))
	if tokensOf(total) > siteSchemaTokenBudget {
		t.Fatalf("the site surface sends %d schema tokens on every call, over %d; move craft into skills", tokensOf(total), siteSchemaTokenBudget)
	}
}

func TestTheSkillIndexOnTheSiteSurfaceStaysSmall(t *testing.T) {
	for _, def := range siteViewTools(t) {
		if def.Name != "load_skill" {
			continue
		}
		tokens := tokensOf(tooltest.SchemaChars(def))
		t.Logf("load_skill on the site surface: %d tokens", tokens)
		if tokens > siteIndexTokenBudget {
			t.Fatalf("load_skill grew to %d tokens on the site surface, over %d", tokens, siteIndexTokenBudget)
		}
		return
	}
	t.Fatal("load_skill must be offered on the site surface")
}

func TestNoSiteFieldDescriptionTurnsIntoAGuide(t *testing.T) {
	var walk func(owner, name string, p tools.Parameter)
	walk = func(owner, name string, p tools.Parameter) {
		if len(p.Description) > siteFieldBudget && owner != "load_skill" {
			t.Errorf("%s.%s has a %d character description; craft belongs in a skill", owner, name, len(p.Description))
		}
		if p.Items != nil {
			for child, cp := range p.Items.Properties {
				walk(owner, child, cp)
			}
		}
	}
	for _, def := range siteViewTools(t) {
		if strings.HasPrefix(def.Name, "site_") && def.Name != "site_generate_image" {
			for name, p := range def.Parameters {
				walk(def.Name, name, p)
			}
		}
	}
}

func resultTokens(t *testing.T, data interface{}) int {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	return tokensOf(len(raw))
}

func TestTheReactStarterMapFitsItsBudget(t *testing.T) {
	r := newRig(t)
	r.editor.pages = allRoutes(gappedHome)
	res := NewSiteReadTool(r.deps).Execute(context.Background(), r.cc, map[string]interface{}{})
	tokens := resultTokens(t, res.Data)
	t.Logf("site_read map for the react captura-de-leads starter: %d tokens", tokens)
	if tokens > siteMapTokenBudget {
		t.Fatalf("the map costs %d tokens, over %d", tokens, siteMapTokenBudget)
	}
}

func TestAnEditResultFitsItsBudgetEvenWithManyProblems(t *testing.T) {
	r := newRig(t)
	r.editor.pages = allRoutes(strings.Replace(gappedHome, "<main>", "<main>"+strings.Repeat(`<img src="https://cdn.example.com/x.jpg"><a href="#">x</a>`, 40), 1))
	r.editor.replies = map[copilot.ScreenCommandName]copilot.ScreenReply{
		copilot.ScreenEdit: {OK: true, Data: []byte(`{"files":{"src/routes/index.jsx":3},"conflicts":["src/routes/index.jsx:1-2","src/routes/index.jsx:5-9","src/routes/index.jsx:12-13","src/routes/index.jsx:20-28","src/routes/index.jsx:31-32","src/routes/index.jsx:40-41","src/routes/index.jsx:50-52","src/routes/index.jsx:60-61","src/routes/index.jsx:70-75","src/routes/index.jsx:80-81","src/routes/index.jsx:90-95","src/routes/index.jsx:100-101","src/routes/index.jsx:110-111","src/routes/index.jsx:120-121"],"skipped":[]}`)},
	}
	res := NewSiteEditTool(r.deps).Execute(context.Background(), r.cc, args(t, headlineEdit))
	tokens := resultTokens(t, res.Data)
	t.Logf("a crowded site_edit result: %d tokens", tokens)
	if res.Status != copilot.StatusOK || tokens > siteEditTokenBudget {
		t.Fatalf("got %d tokens: %+v", tokens, res)
	}
	refused := NewSiteEditTool(r.deps).Execute(context.Background(), r.cc, args(t, `{"operations":[`+strings.TrimSuffix(strings.Repeat(`{"op":"edit_file","path":"src/Nada.jsx","old":"a","new":"b","extra":1},`, 30), ",")+`]}`))
	if tokensOf(len(refused.Message)) > siteEditTokenBudget {
		t.Fatalf("a refusal costs %d tokens", tokensOf(len(refused.Message)))
	}
}

func TestALookResultFitsItsBudget(t *testing.T) {
	r := newRig(t)
	answering(t, r, lookAnswer(t, richCapture(viewportsFor(""), true), sheetURL(t)))
	r.editor.pages = allRoutes(strings.Replace(gappedHome, "<main>", "<main>"+distinctGaps(10), 1))
	res := NewSiteLookTool(r.deps).Execute(context.Background(), r.cc, args(t, `{"marks":true}`))
	tokens := resultTokens(t, res.Data)
	t.Logf("a site_look result with every mark and check line the editor capture allows: %d tokens", tokens)
	if res.Status != copilot.StatusOK || tokens > siteLookTokenBudget {
		t.Fatalf("got %d tokens: %+v", tokens, res)
	}
}

func itoa(n int) string {
	raw, _ := json.Marshal(n)
	return string(raw)
}

func distinctGaps(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(`<img src="https://cdn.example.com/x.jpg" data-vz-node="src/components/Beneficios.jsx#1` + itoa(1000+i) + `">`)
		b.WriteString(`<a href="#" data-vz-node="src/components/Beneficios.jsx#2` + itoa(1000+i) + `">x</a>`)
	}
	return b.String()
}
