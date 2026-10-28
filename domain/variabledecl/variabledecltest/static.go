package variabledecltest

import (
	"context"

	"vozko/domain/variabledecl"
)

type Static struct {
	Declarations []variabledecl.Declaration
	Err          error
}

func (s Static) Catalog(context.Context, string) (variabledecl.Catalog, error) {
	return variabledecl.NewCatalog(s.Declarations), s.Err
}

func (s Static) List(context.Context, string) ([]variabledecl.Declaration, error) {
	return s.Declarations, s.Err
}

func (s Static) Described(context.Context, string) ([]variabledecl.Declaration, error) {
	return s.Registry().Described(s.Declarations), s.Err
}

func (Static) Registry() *variabledecl.Registry {
	registry, err := variabledecl.NewRegistry(variabledecl.Builtin()...)
	if err != nil {
		panic(err)
	}
	return registry
}
