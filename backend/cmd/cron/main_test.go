package main

import (
	"context"
	"errors"
	"testing"

	"backend/internal/discoverycron"
	"backend/internal/transactioncron"
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

func TestJobFailed_ZeroProgressWithClaimErrorsIsFailure(t *testing.T) {
	report := discoverycron.Report{UserClaimErrors: 1}
	if err := jobFailed(report); err == nil {
		t.Error("expected an error when the run made zero progress and saw a claim error")
	}
}

func TestJobFailed_ZeroProgressWithLeagueClaimErrorsIsFailure(t *testing.T) {
	report := discoverycron.Report{LeagueClaimErrors: 1}
	if err := jobFailed(report); err == nil {
		t.Error("expected an error when the run made zero progress and saw a league claim error")
	}
}

func TestJobFailed_ZeroProgressWithNoClaimErrorsIsNotFailure(t *testing.T) {
	report := discoverycron.Report{}
	if err := jobFailed(report); err != nil {
		t.Errorf("expected a genuinely-empty queue (no claim errors) to not be a failure, got %v", err)
	}
}

func TestJobFailed_RealProgressIsNotFailureEvenWithClaimErrors(t *testing.T) {
	report := discoverycron.Report{UsersProcessed: 1, UserClaimErrors: 5}
	if err := jobFailed(report); err != nil {
		t.Errorf("expected real progress alongside claim errors to not be a failure, got %v", err)
	}
}

func TestJobFailed_OnlyFailedCountsStillCountAsProgress(t *testing.T) {
	report := discoverycron.Report{LeaguesFailed: 1, LeagueClaimErrors: 1}
	if err := jobFailed(report); err != nil {
		t.Errorf("expected a run with real (even if failed) processing activity to not be treated as total failure, got %v", err)
	}
}

func TestTxnJobFailed_ZeroProgressWithClaimErrorsIsFailure(t *testing.T) {
	report := transactioncron.Report{ClaimErrors: 1}
	if err := txnJobFailed(report); err == nil {
		t.Error("expected an error when the run made zero progress and saw a claim error")
	}
}

func TestTxnJobFailed_ZeroProgressWithNoClaimErrorsIsNotFailure(t *testing.T) {
	report := transactioncron.Report{}
	if err := txnJobFailed(report); err != nil {
		t.Errorf("expected a genuinely-empty queue (no claim errors) to not be a failure, got %v", err)
	}
}

func TestTxnJobFailed_RealProgressIsNotFailureEvenWithClaimErrors(t *testing.T) {
	report := transactioncron.Report{LeaguesProcessed: 1, ClaimErrors: 5}
	if err := txnJobFailed(report); err != nil {
		t.Errorf("expected real progress alongside claim errors to not be a failure, got %v", err)
	}
}

func TestTxnJobFailed_OnlyFailedCountsStillCountAsProgress(t *testing.T) {
	report := transactioncron.Report{LeaguesFailed: 1, ClaimErrors: 1}
	if err := txnJobFailed(report); err != nil {
		t.Errorf("expected a run with real (even if failed) processing activity to not be treated as total failure, got %v", err)
	}
}
