package main

import (
	"context"
	"errors"
	"testing"
)

func TestResolveJob_KnownJobReturnsItsFunc(t *testing.T) {
	called := false
	registry := map[string]func(context.Context) error{
		"discovery": func(context.Context) error { called = true; return nil },
	}
	fn, err := resolveJob(registry, "discovery")
	if err != nil {
		t.Fatalf("resolveJob error: %v", err)
	}
	if err := fn(context.Background()); err != nil {
		t.Fatalf("job func error: %v", err)
	}
	if !called {
		t.Error("expected the registered job function to run")
	}
}

func TestResolveJob_UnknownJobErrorsCleanly(t *testing.T) {
	registry := map[string]func(context.Context) error{
		"discovery": func(context.Context) error { return nil },
	}
	_, err := resolveJob(registry, "does-not-exist")
	if err == nil {
		t.Fatal("expected an error for an unregistered job name")
	}
	if !errors.Is(err, errUnknownJob) {
		t.Errorf("expected errUnknownJob, got %v", err)
	}
}
