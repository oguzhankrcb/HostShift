package workload

import (
	"context"

	"github.com/oguzhankaracabay/hostshift/internal/core"
	"github.com/oguzhankaracabay/hostshift/internal/profile"
)

type Context struct {
	Profile profile.Profile
}

type Adapter interface {
	Type() string
	Discover(context.Context, Context) ([]profile.Workload, error)
	Plan(context.Context, Context, profile.Workload) ([]core.Action, []string, error)
	Verify(context.Context, Context, profile.Workload) ([]core.Action, error)
}

type Registry struct {
	adapters map[string]Adapter
}

func NewRegistry(adapters ...Adapter) Registry {
	registry := Registry{adapters: map[string]Adapter{}}
	for _, adapter := range adapters {
		registry.adapters[adapter.Type()] = adapter
	}
	return registry
}

func (r Registry) Get(kind string) (Adapter, bool) {
	adapter, ok := r.adapters[kind]
	return adapter, ok
}

func (r Registry) Types() []string {
	types := make([]string, 0, len(r.adapters))
	for kind := range r.adapters {
		types = append(types, kind)
	}
	return types
}
