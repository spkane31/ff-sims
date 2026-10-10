package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"os/exec"
	"strings"
	"testing"

	"backend/internal/discoverycron"
	"backend/internal/sleeper"
	"backend/internal/transactioncron"
	"gorm.io/gorm"
)

// Run the real entrypoint in a subprocess: a retired job must succeed even
// without valid database configuration, including when launched manually.
func TestRetiredJobsDoNotConnectToDatabase(t *testing.T) {
	if job := os.Getenv("FF_SIMS_TEST_RETIRED_JOB"); job != "" {
		flag.CommandLine = flag.NewFlagSet("cron", flag.ExitOnError)
		os.Args = []string{"cron", "-job=" + job, "-max-duration=1s"}
		main()
		return
	}
	for _, job := range []string{"discovery", "transactions", "lifetime-counts"} {
		t.Run(job, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestRetiredJobsDoNotConnectToDatabase$")
			cmd.Dir = t.TempDir()
			cmd.Env = append(os.Environ(), "FF_SIMS_TEST_RETIRED_JOB="+job,
				"DATABASE_URL=invalid://retired-job-must-not-connect",
				"ARCHIVE_DATABASE_URL=invalid://retired-job-must-not-connect")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("retired job failed: %v\n%s", err, out)
			}
			if !strings.Contains(string(out), "job "+job+" is retired") {
				t.Fatalf("missing retirement message: %s", out)
			}
		})
	}
}

func TestRunDiscoveryJob_IsPausedWithoutCallingSleeper(t *testing.T) {
	originalRunDiscovery := runDiscovery
	t.Cleanup(func() { runDiscovery = originalRunDiscovery })

	called := false
	runDiscovery = func(context.Context, *gorm.DB, *sleeper.Client, discoverycron.Config) (discoverycron.Report, error) {
		called = true
		return discoverycron.Report{}, nil
	}

	if err := runDiscoveryJob(context.Background(), nil, nil); err != nil {
		t.Fatalf("runDiscoveryJob error: %v", err)
	}
	if called {
		t.Error("paused discovery job called Sleeper discovery")
	}
}

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
