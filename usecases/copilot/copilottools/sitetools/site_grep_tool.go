package sitetools

import (
	"context"
	"fmt"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"

	"vozko/domain/copilot"
	"vozko/domain/tools"
	"vozko/usecases/copilot/copilottools"
)

const (
	maxGrepMatches = 40
	maxGrepLine    = 160
	grepNoText     = "text é obrigatório"
	grepBadGlob    = "files não é um padrão válido, como src/**/*.jsx ou pages/*.html"
	grepMore       = "e mais %d linhas; refine text ou files"
)

type siteGrepArgs struct {
	Text      string `json:"text" req:"true" desc:"o texto procurado, literal"`
	Files     string `json:"files,omitempty" desc:"opcional: padrão de caminho, como src/*.jsx ou pages/*.html"`
	MatchCase bool   `json:"match_case,omitempty" desc:"diferencia maiúsculas"`
}

type siteGrepTool struct {
	siteScope
	deps Deps
}

func NewSiteGrepTool(deps Deps) copilot.Tool {
	return &siteGrepTool{deps: deps}
}

func (t *siteGrepTool) Meta() copilot.Meta { return readMeta() }

func (t *siteGrepTool) Definition() tools.Definition {
	return copilottools.Definition("site_grep",
		"Procura um texto literal em todos os arquivos do site aberto e devolve cada linha encontrada como caminho:linha: texto, até 40 linhas.",
		siteGrepArgs{})
}

func globMatch(pattern, name string) (bool, error) {
	if pattern == "" {
		return true, nil
	}
	if strings.Contains(pattern, "**") {
		prefix, suffix, _ := strings.Cut(pattern, "**")
		if !strings.HasPrefix(name, prefix) {
			return false, nil
		}
		rest := strings.TrimPrefix(suffix, "/")
		if rest == "" {
			return true, nil
		}
		return path.Match(rest, path.Base(name))
	}
	return path.Match(pattern, name)
}

func (t *siteGrepTool) Execute(ctx context.Context, cc copilot.Context, args map[string]interface{}) copilot.Result {
	var a siteGrepArgs
	if err := copilottools.DecodeArgs(args, &a); err != nil {
		return failure(err.Error())
	}
	if a.Text == "" {
		return failure(grepNoText)
	}
	if _, err := globMatch(a.Files, "x"); err != nil {
		return failure(grepBadGlob)
	}
	view, err := t.deps.open(ctx, cc, forReading)
	if err != nil {
		return failure(err.Error())
	}
	needle := a.Text
	if !a.MatchCase {
		needle = strings.ToLower(needle)
	}
	var matches []string
	extra := 0
	for _, name := range slices.Sorted(maps.Keys(view.Files)) {
		if ok, _ := globMatch(a.Files, name); !ok {
			continue
		}
		for i, line := range strings.Split(view.Files[name].Content, "\n") {
			haystack := line
			if !a.MatchCase {
				haystack = strings.ToLower(line)
			}
			if !strings.Contains(haystack, needle) {
				continue
			}
			if len(matches) == maxGrepMatches {
				extra++
				continue
			}
			matches = append(matches, name+":"+strconv.Itoa(i+1)+": "+clip(strings.TrimSpace(line), maxGrepLine))
		}
	}
	body := map[string]interface{}{"matches": matches}
	if matches == nil {
		body["matches"] = []string{}
	}
	if extra > 0 {
		body["note"] = fmt.Sprintf(grepMore, extra)
	}
	return copilot.Result{Status: copilot.StatusOK, Data: asPageData(body)}
}
