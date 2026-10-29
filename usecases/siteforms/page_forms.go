package siteforms

import (
	"context"
	"errors"
	"strings"
)

var (
	ErrWorkspaceRequired       = errors.New("siteforms: the page forms list needs a workspace")
	ErrFormUnknownForWorkspace = errors.New("siteforms: the page form is not one of this workspace")
)

type PageForm struct {
	ProjectID   string
	ProjectName string
	FormID      string
	FormName    string
	FormKey     string
	Live        bool
}

type FormDirectory interface {
	WorkspaceForms(ctx context.Context, workspaceID string) ([]PageForm, error)
	WorkspaceForm(ctx context.Context, workspaceID, formKey string) (*PageForm, error)
}

func (s *Service) PageForms(ctx context.Context, workspaceID string) ([]PageForm, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, ErrWorkspaceRequired
	}
	return s.d.Forms.WorkspaceForms(ctx, workspaceID)
}
