package site

import (
	"maps"
	"slices"
)

type FileChanges struct {
	Files   map[string]File `json:"files"`
	Deleted []string        `json:"deleted"`
	Renamed []Rename        `json:"renamed"`
}

func Changes(before, after map[string]File, steps []StepPatch) FileChanges {
	out := FileChanges{Files: map[string]File{}, Deleted: []string{}, Renamed: []Rename{}}
	origins := map[string]string{}
	for _, step := range steps {
		for _, r := range step.Renamed {
			origin, ok := origins[r.From]
			if !ok {
				origin = r.From
			}
			delete(origins, r.From)
			origins[r.To] = origin
		}
	}
	moved := map[string]bool{}
	for _, to := range slices.Sorted(maps.Keys(origins)) {
		origin := origins[to]
		_, landed := after[to]
		_, existed := before[origin]
		_, stayed := after[origin]
		if landed && existed && !stayed && origin != to {
			out.Renamed = append(out.Renamed, Rename{From: origin, To: to})
			moved[origin] = true
		}
	}
	for _, path := range slices.Sorted(maps.Keys(before)) {
		if _, ok := after[path]; !ok && !moved[path] {
			out.Deleted = append(out.Deleted, path)
		}
	}
	for path, file := range after {
		if old, ok := before[path]; !ok || old != file {
			out.Files[path] = file
		}
	}
	return out
}
