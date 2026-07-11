package workload

import (
	"context"
	"sort"

	"github.com/oguzhankaracabay/hostshift/internal/core"
	"github.com/oguzhankaracabay/hostshift/internal/profile"
)

type Context struct {
	Profile profile.Profile
}

type PlanResult struct {
	Actions      []core.Action
	Streams      []core.StreamAction
	Blockers     []string
	Capabilities []string
}

type Adapter interface {
	Type() string
	Discover(context.Context, Context) ([]profile.Workload, error)
	Plan(context.Context, Context, profile.Workload) (PlanResult, error)
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
	sort.Strings(types)
	return types
}
