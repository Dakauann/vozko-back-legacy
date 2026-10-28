package copilot_usecase

import (
	"context"
	"strings"
	"testing"

	"vozko/domain/ai"
	"vozko/domain/aichat"
	"vozko/domain/copilot"
	"vozko/usecases/copilot/copilottools/attendancetools"
	"vozko/usecases/copilot/copilottools/platformtools"
	"vozko/usecases/copilot/copilottools/sitetools"
	"vozko/usecases/copilot/copilottools/skilltools"
	"vozko/usecases/copilot/copilottools/tooltest"
	"vozko/usecases/copilot/skills"
)

const (
	siteBytesPerToken      = 2.78
	siteFirstCallTokenCeil = 16000
)

func siteFirstCall(t *testing.T) ai.GenerateInput {
	t.Helper()
	library, err := skills.Load()
	if err != nil {
		t.Fatal(err)
	}
	prov := &scriptAI{turns: [][]ai.ToolCall{{}}, texts: []string{"Vejo a página."}, models: []ai.ModelInfo{toolModel("m")}}
	th := &fakeThreads{thread: &aichat.Thread{ID: "t1", WorkspaceID: "ws1", UserID: "u1", Model: "m", ProjectID: siteView.ProjectID}}
	plain := &fakeTool{name: "list_agents", meta: readMeta}
	toolset := append(sitetools.SiteTools(sitetools.Deps{}), skilltools.NewLoadSkillTool(library), platformtools.NewAskUserTool(), attendancetools.NewCalculateTool(), plain)
	svc := newService(prov, th, &fakeMessages{}, toolset...)
	cc := ownerCtx
	cc.View = siteView
	if err := svc.Stream(context.Background(), th.thread, copilot.UserMessage{Content: "crie uma landing page para a minha clínica"}, cc, func(string, interface{}) {}); err != nil {
		t.Fatal(err)
	}
	if len(prov.inputs) == 0 {
		t.Fatal("the model was never called")
	}
	return prov.inputs[0]
}

func TestTheFirstCallOnTheSiteSurfaceFitsItsCeiling(t *testing.T) {
	in := siteFirstCall(t)
	chars := len(in.SystemPrompt)
	var names []string
	for _, def := range in.Tools {
		chars += tooltest.SchemaChars(def)
		names = append(names, def.Name)
	}
	for _, m := range in.Messages {
		chars += len(m.Content)
	}
	tokens := int(float64(chars)/siteBytesPerToken + 0.5)
	t.Logf("first call on the site surface: %d tokens (system %d chars, tools %s)", tokens, len(in.SystemPrompt), strings.Join(names, ", "))
	if tokens > siteFirstCallTokenCeil {
		t.Fatalf("the first site call costs %d tokens, over %d", tokens, siteFirstCallTokenCeil)
	}
	for _, name := range names {
		if strings.Contains(name, "_ad") || strings.HasPrefix(name, "ad_") || name == "list_agents" || name == "new_site" {
			t.Fatalf("%s does not belong on the site surface", name)
		}
	}
	if !offers(in, "site_edit") || !offers(in, "load_skill") {
		t.Fatal("the site tools and the skill index are offered")
	}
}
