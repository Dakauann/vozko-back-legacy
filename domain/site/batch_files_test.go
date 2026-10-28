package site

import (
	"testing"
)

func revisedProject() Project {
	p := htmlProject()
	for _, path := range []string{"pages/index.html", "scripts/motion.js", "partials/header.html"} {
		p.Files[path] = File{Content: p.Files[path].Content, Rev: 4}
	}
	return p
}

func TestWholeFileOperationsNeedTheRevTheyWereReadAt(t *testing.T) {
	cases := map[string]struct {
		op   string
		want string
	}{
		"write without rev":      {`{"op":"write_file","path":"scripts/motion.js","content":"export {}\n"}`, "0:field_required:rev"},
		"write at an old rev":    {`{"op":"write_file","path":"scripts/motion.js","rev":3,"content":"export {}\n"}`, "0:file_stale:rev"},
		"write at a future rev":  {`{"op":"write_file","path":"scripts/motion.js","rev":5,"content":"export {}\n"}`, "0:rev_invalid:rev"},
		"write at the rev":       {`{"op":"write_file","path":"scripts/motion.js","rev":4,"content":"export {}\n"}`, ""},
		"new file":               {`{"op":"write_file","path":"pages/sobre.html","content":"<h1>Sobre</h1>\n"}`, ""},
		"new file with a rev":    {`{"op":"write_file","path":"pages/sobre.html","rev":2,"content":"<h1>Sobre</h1>\n"}`, "0:file_stale:rev"},
		"delete without rev":     {`{"op":"delete_file","path":"scripts/motion.js"}`, "0:field_required:rev"},
		"delete at an old rev":   {`{"op":"delete_file","path":"scripts/motion.js","rev":1}`, "0:file_stale:rev"},
		"delete at the rev":      {`{"op":"delete_file","path":"scripts/motion.js","rev":4}`, ""},
		"rename without rev":     {`{"op":"rename_file","path":"partials/header.html","to":"partials/topo.html"}`, "0:field_required:rev"},
		"rename at an old rev":   {`{"op":"rename_file","path":"partials/header.html","rev":2,"to":"partials/topo.html"}`, "0:file_stale:rev"},
		"rename at the rev":      {`{"op":"rename_file","path":"partials/header.html","rev":4,"to":"partials/topo.html"}`, ""},
		"required before rev":    {`{"op":"delete_file","path":"site.json"}`, "0:file_required:path"},
		"edit_file needs no rev": {`{"op":"edit_file","path":"scripts/motion.js","old":"animate","new":"timeline"}`, ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, refused := prepare(t, revisedProject(), "["+c.op+"]")
			if got := failures(refused); got != c.want {
				t.Fatalf("got  %s\nwant %s", got, c.want)
			}
		})
	}
}

func TestFilesBornInTheBatchNeedNoRev(t *testing.T) {
	_, refused := prepare(t, revisedProject(), `[
	  {"op":"write_file","path":"pages/sobre.html","content":"<h1>Sobre</h1>\n"},
	  {"op":"write_file","path":"pages/sobre.html","content":"<h1>Quem somos</h1>\n"},
	  {"op":"rename_file","path":"pages/sobre.html","to":"pages/quem.html"},
	  {"op":"write_file","path":"scripts/motion.js","rev":4,"content":"export {}\n"},
	  {"op":"write_file","path":"scripts/motion.js","rev":4,"content":"export const a = 1\n"},
	  {"op":"delete_file","path":"pages/quem.html"}
	]`)
	if refused != nil {
		t.Fatalf("got %s", failures(refused))
	}
}
