package variabledecl

import (
	"context"
	"strings"
)

const (
	KindSheet  Kind  = "sheet"
	ShapeSheet Shape = "sheet"

	SettingModel      = "model"
	SettingModelTitle = "model_title"
	SettingDefault    = "default"
)

type Preparer interface {
	Prepare(ctx context.Context, workspaceID string, t Type) (Type, error)
}

func (r *Registry) Prepare(ctx context.Context, workspaceID string, d Declaration) (Declaration, error) {
	preparer, ok := r.rules[d.Type.Kind].(Preparer)
	if !ok {
		return d, nil
	}
	prepared, err := preparer.Prepare(ctx, workspaceID, d.Type)
	if err != nil {
		return Declaration{}, err
	}
	d.Type = prepared
	return d, nil
}

func DefaultOf(d Declaration) string {
	return strings.TrimSpace(d.Type.Settings[SettingDefault])
}
