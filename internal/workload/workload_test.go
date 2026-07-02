package workload

import (
	"context"
	"testing"

	"github.com/oguzhankaracabay/hostshift/internal/core"
	"github.com/oguzhankaracabay/hostshift/internal/profile"
)

type fakeAdapter struct{ kind string }

func (f fakeAdapter) Type() string { return f.kind }

func (f fakeAdapter) Discover(context.Context, Context) ([]profile.Workload, error) {
	return nil, nil
}

func (f fakeAdapter) Plan(context.Context, Context, profile.Workload) ([]core.Action, []string, error) {
	return nil, nil, nil
}

func (f fakeAdapter) Verify(context.Context, Context, profile.Workload) ([]core.Action, error) {
	return nil, nil
}

func TestRegistryFindsAdapters(t *testing.T) {
	registry := NewRegistry(fakeAdapter{kind: "docker-compose"})
	if _, ok := registry.Get("docker-compose"); !ok {
		t.Fatal("expected adapter to be registered")
	}
	if _, ok := registry.Get("unknown"); ok {
		t.Fatal("unexpected unknown adapter")
	}
}
