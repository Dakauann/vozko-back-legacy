package site_usecase

import (
	"context"
	"maps"
	"slices"

	"vozko/domain/site"
)

func (s *Service) foreignNumbers(ctx context.Context, workspaceID string, batch site.Batch) []site.Failure {
	checked := map[string]bool{}
	var failures []site.Failure
	for _, use := range batch.WhatsAppNumbers() {
		usable, seen := checked[use.Ref]
		if !seen {
			_, err := s.numbers.NumberOf(ctx, workspaceID, use.Ref)
			usable = err == nil
			checked[use.Ref] = usable
		}
		if !usable {
			failures = append(failures, site.Failure{Index: use.Index, Code: site.CodeWhatsAppNumberUnknown, Field: use.Field})
		}
	}
	return failures
}

func (s *Service) bases(ctx context.Context, v *View, wants map[string][]uint64) (map[string][]site.File, error) {
	if len(wants) == 0 {
		return nil, nil
	}
	found, err := s.store.FileRevs(ctx, v.Project.WorkspaceID, v.Project.ID, wants)
	if err != nil {
		return nil, err
	}
	var shas []string
	for _, revs := range found {
		for _, sha := range revs {
			shas = append(shas, sha)
		}
	}
	slices.Sort(shas)
	blobs, err := s.store.Blobs(ctx, v.Project.WorkspaceID, slices.Compact(shas))
	if err != nil {
		return nil, err
	}
	out := map[string][]site.File{}
	for _, path := range slices.Sorted(maps.Keys(found)) {
		for _, rev := range slices.Sorted(maps.Keys(found[path])) {
			body, ok := blobs[found[path][rev]]
			if !ok || contentSHA(string(body)) != found[path][rev] {
				continue
			}
			out[path] = append(out[path], site.File{Content: string(body), Rev: rev})
		}
	}
	return out, nil
}

func sourced(batch site.Batch, result ExecuteResult) ExecuteResult {
	out := ExecuteResult{Steps: make([]site.StepPatch, 0, len(result.Steps))}
	for _, f := range result.Failures {
		f.Index = batch.Origin(f.Index).Index
		out.Failures = append(out.Failures, f)
	}
	for _, skip := range result.Skipped {
		skip.Index, skip.After = batch.Origin(skip.Index).Index, batch.Origin(skip.After).Index
		if skip.Index != skip.After {
			out.Skipped = append(out.Skipped, skip)
		}
	}
	for _, step := range result.Steps {
		step.Index = batch.Origin(step.Index).Index
		out.Steps = append(out.Steps, step)
	}
	if result.Steps == nil {
		out.Steps = nil
	}
	return out
}
