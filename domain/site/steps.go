package site

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"vozko/domain/strictjson"
)

var ErrStepsInvalid = errors.New("site: the builder steps do not fit the project")

type Rename struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type StepPatch struct {
	Op      OperationKind     `json:"op"`
	Index   int               `json:"index"`
	Focus   string            `json:"focus,omitempty"`
	Files   map[string]string `json:"files,omitempty"`
	Deleted []string          `json:"deleted,omitempty"`
	Renamed []Rename          `json:"renamed,omitempty"`
}

func DecodeSteps(raw []byte) ([]StepPatch, error) {
	var steps []StepPatch
	if err := strictjson.Decode(raw, &steps); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrStepsInvalid, err)
	}
	if steps == nil {
		return nil, fmt.Errorf("%w: steps must be a list", ErrStepsInvalid)
	}
	return steps, nil
}

func stepError(index int, reason string) error {
	return fmt.Errorf("%w: step %d %s", ErrStepsInvalid, index+1, reason)
}

func ApplySteps(base Project, steps []StepPatch) (Project, error) {
	p := base.Clone()
	for n, step := range steps {
		if !step.Op.Known() {
			return Project{}, stepError(n, "has an unknown operation")
		}
		bumped := map[string]bool{}
		bump := func(path string, rev uint64) {
			if !bumped[path] {
				rev++
				bumped[path] = true
			}
			p.Files[path] = File{Content: p.Files[path].Content, Rev: rev}
		}
		for _, r := range step.Renamed {
			from, ok := p.Files[r.From]
			if !ok || p.Has(r.To) || ValidatePath(p.Stack, r.To) != nil {
				return Project{}, stepError(n, "renames "+r.From+" to "+r.To)
			}
			delete(p.Files, r.From)
			p.Files[r.To] = from
			bump(r.To, from.Rev)
		}
		for _, path := range step.Deleted {
			if !p.Has(path) {
				return Project{}, stepError(n, "deletes the missing file "+path)
			}
			if _, rewritten := step.Files[path]; rewritten {
				return Project{}, stepError(n, "deletes and writes "+path)
			}
			delete(p.Files, path)
		}
		for _, path := range slices.Sorted(maps.Keys(step.Files)) {
			content := step.Files[path]
			if ValidatePath(p.Stack, path) != nil {
				return Project{}, stepError(n, "writes the invalid path "+path)
			}
			current, exists := p.Files[path]
			if exists && current.Content == content {
				continue
			}
			p.Files[path] = File{Content: content, Rev: current.Rev}
			if bumped[path] {
				continue
			}
			bump(path, current.Rev)
		}
	}
	return p, nil
}
