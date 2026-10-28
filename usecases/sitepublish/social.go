package sitepublish_usecase

import (
	"context"
	"fmt"
	"log"
	"strings"

	"vozko/domain/site"
	"vozko/domain/sitepublish"
)

const (
	socialImageType      = "image/jpeg"
	socialImageExtension = ".jpg"
)

func (s *Service) candidate(ctx context.Context, req CandidateRequest, stack site.Stack) (Candidate, error) {
	candidate, err := s.Builder.Candidate(ctx, req)
	if err != nil {
		return Candidate{}, fmt.Errorf("%w: %w", ErrBuildFailed, err)
	}
	if len(candidate.Errors) > 0 {
		return Candidate{}, &BuildFailure{Errors: candidate.Errors}
	}
	if issues := site.CheckImports(stack, candidate.Imports); len(issues) > 0 {
		return Candidate{}, &IssuesFailure{Kind: ErrPublishBlocked, Issues: issues}
	}
	return candidate, nil
}

func routeAddress(candidateURL, route string) string {
	return strings.TrimSuffix(candidateURL, "/") + "/" + strings.TrimPrefix(route, "/")
}

func (s *Service) withSocialImages(ctx context.Context, req CandidateRequest, first Candidate) (Candidate, map[string][]byte) {
	images, bodies := map[string]string{}, map[string][]byte{}
	for _, r := range first.Routes {
		if req.Settings.Pages[r.Route].OGMediaID != "" {
			continue
		}
		shot, err := s.Renderer.OpenGraph(ctx, routeAddress(first.URL, r.Route))
		if err != nil || len(shot) == 0 {
			log.Printf("[sitepublish] project %s route %s goes out without a social image: %v", req.ProjectID, r.Route, err)
			continue
		}
		served := sitepublish.MediaPathPrefix + sitepublish.SHA256Hex(shot) + socialImageExtension
		images[r.Route] = req.PrimaryOrigin + served
		bodies[served] = shot
	}
	if len(images) == 0 {
		return first, nil
	}
	req.OGImages = images
	rebuilt, err := s.candidate(ctx, req, req.Project.Stack)
	if err != nil {
		log.Printf("[sitepublish] project %s goes out without social images, the rebuild failed: %v", req.ProjectID, err)
		return first, nil
	}
	return rebuilt, bodies
}
