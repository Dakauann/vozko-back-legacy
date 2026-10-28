package siteforms

import (
	"context"
	"errors"
	"strings"
)

var ErrWorkspaceRequired = errors.New("siteforms: the page forms list needs a workspace")

type PageForm struct {
	ProjectID   string
	ProjectName string
	FormID      string
	FormKey     string
	Live        bool
}

type FormDirectory interface {
	WorkspaceForms(ctx context.Context, workspaceID string) ([]PageForm, error)
}

func (s *Service) PageForms(ctx context.Context, workspaceID string) ([]PageForm, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, ErrWorkspaceRequired
	}
	return s.d.Forms.WorkspaceForms(ctx, workspaceID)
}
