package webchat

import (
	"context"
	"errors"

	wcdomain "vozko/domain/webchat"
)

var ErrUsageIncomplete = errors.New("webchat: widget usage needs the widgets and the page origins")

type WidgetUsage struct {
	SiteOrigins    []string
	AllowedOrigins []string
}

type Usage struct {
	widgets wcdomain.WidgetRepository
	sites   SiteOrigins
}

func NewUsage(widgets wcdomain.WidgetRepository, sites SiteOrigins) (*Usage, error) {
	if widgets == nil || sites == nil {
		return nil, ErrUsageIncomplete
	}
	return &Usage{widgets: widgets, sites: sites}, nil
}

func (u *Usage) Of(ctx context.Context, workspaceID, id string) (WidgetUsage, error) {
	w, err := u.widgets.FindByID(ctx, workspaceID, id)
	if err != nil {
		return WidgetUsage{}, err
	}
	if w == nil {
		return WidgetUsage{}, wcdomain.ErrWidgetNotFound
	}
	origins, err := u.sites.WidgetOrigins(ctx, workspaceID, w.ID)
	if err != nil {
		return WidgetUsage{}, err
	}
	return WidgetUsage{SiteOrigins: append([]string{}, origins...), AllowedOrigins: append([]string{}, w.AllowedOrigins...)}, nil
}
