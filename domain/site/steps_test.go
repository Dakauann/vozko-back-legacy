package site

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestApplyStepsReplaysWhatTheBuilderReturned(t *testing.T) {
	base := htmlProject()
	steps, err := DecodeSteps([]byte(`[
	  {"op":"set_text","index":0,"focus":"pages/index.html#24","files":{"pages/index.html":"<section class=\"hero\">\n  <h1>Olá</h1>\n</section>\n"}},
	  {"op":"rename_file","index":1,"focus":"partials/topo.html","renamed":[{"from":"partials/header.html","to":"partials/topo.html"}]},
	  {"op":"edit_file","index":2,"focus":"partials/topo.html","renamed":[],"files":{"partials/topo.html":"<header>Marca</header>\n"}},
	  {"op":"delete_file","index":3,"focus":"scripts/motion.js","deleted":["scripts/motion.js"]},
	  {"op":"write_file","index":4,"focus":"pages/sobre.html","files":{"pages/sobre.html":"<h1>Sobre</h1>\n"}},
	  {"op":"set_classes","index":5,"focus":"pages/index.html#0"}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	final, err := ApplySteps(base, steps)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]File{
		"pages/index.html":    {Content: "<section class=\"hero\">\n  <h1>Olá</h1>\n</section>\n", Rev: 2},
		"partials/topo.html":  {Content: "<header>Marca</header>\n", Rev: 3},
		"pages/sobre.html":    {Content: "<h1>Sobre</h1>\n", Rev: 1},
		"pages/obrigado.html": base.Files["pages/obrigado.html"],
	}
	for path, file := range want {
		if final.Files[path] != file {
			t.Fatalf("%s got %+v want %+v", path, final.Files[path], file)
		}
	}
	if final.Has("partials/header.html") || final.Has("scripts/motion.js") || base.Files["pages/index.html"].Rev != 1 {
		t.Fatal("the base must stay untouched and renamed or deleted files gone")
	}
}

func TestApplyStepsBumpsARevisionOncePerStepAndNotForUnchangedContent(t *testing.T) {
	base := htmlProject()
	same := base.Files["pages/obrigado.html"].Content
	steps := []StepPatch{
		{Op: OpRenameFile, Renamed: []Rename{{From: "pages/obrigado.html", To: "pages/fim.html"}}, Files: map[string]string{"pages/fim.html": "<h1>Fim</h1>\n"}},
		{Op: OpWriteFile, Files: map[string]string{"pages/index.html": base.Files["pages/index.html"].Content}},
		{Op: OpWriteFile, Files: map[string]string{"pages/fim.html": same}},
	}
	final, err := ApplySteps(base, steps)
	if err != nil {
		t.Fatal(err)
	}
	if final.Files["pages/fim.html"].Rev != 3 || final.Files["pages/index.html"].Rev != 1 {
		t.Fatalf("got %+v %+v", final.Files["pages/fim.html"], final.Files["pages/index.html"])
	}
}

func TestApplyStepsRefusesStepsThatDoNotFitTheProject(t *testing.T) {
	cases := map[string][]StepPatch{
		"unknown op":       {{Op: "paint"}},
		"rename missing":   {{Op: OpRenameFile, Renamed: []Rename{{From: "pages/x.html", To: "pages/y.html"}}}},
		"rename onto file": {{Op: OpRenameFile, Renamed: []Rename{{From: "pages/obrigado.html", To: "pages/index.html"}}}},
		"rename bad path":  {{Op: OpRenameFile, Renamed: []Rename{{From: "pages/obrigado.html", To: "../x.html"}}}},
		"delete missing":   {{Op: OpDeleteFile, Deleted: []string{"pages/x.html"}}},
		"write bad path":   {{Op: OpWriteFile, Files: map[string]string{"index.html": ""}}},
		"delete and write": {{Op: OpDeleteFile, Deleted: []string{"pages/obrigado.html"}, Files: map[string]string{"pages/obrigado.html": "x"}}},
	}
	for name, steps := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ApplySteps(htmlProject(), steps); !errors.Is(err, ErrStepsInvalid) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestDecodeStepsIsStrict(t *testing.T) {
	for _, raw := range []string{``, `{}`, `[{"op":"set_text","extra":1}]`, `[{"op":"set_text","files":{"a":1}}]`, `[] []`} {
		if _, err := DecodeSteps([]byte(raw)); !errors.Is(err, ErrStepsInvalid) {
			t.Fatalf("%q: got %v", raw, err)
		}
	}
	steps, err := DecodeSteps([]byte(`[]`))
	if err != nil || len(steps) != 0 {
		t.Fatalf("an empty batch result is no steps: %v %v", steps, err)
	}
}

func TestStepPatchesEncodeCompactly(t *testing.T) {
	raw, err := json.Marshal(StepPatch{Op: OpSetClasses, Index: 2, Focus: "pages/index.html#0"})
	if err != nil || string(raw) != `{"op":"set_classes","index":2,"focus":"pages/index.html#0"}` {
		t.Fatalf("got %s %v", raw, err)
	}
}
