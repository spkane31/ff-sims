import datetime

from temporalio.common import RetryPolicy

# Shared by every workflow that executes a DB-writing activity.
#
# ValueError signals a permanent precondition failure (e.g. league/credentials
# not registered) — retrying it forever just wastes attempts and hides the
# failure, so it's excluded here; every other failure still gets bounded
# retries.
#
# Explicit backoff (not the SDK's unlabeled 1s/2x/100s-cap defaults) so a
# burst of concurrently-failing activities — e.g. several backfill workflows
# whose writes collide on the players table shared across every league —
# backs off predictably instead of hammering Postgres in a tight loop.
# Temporal's server applies jitter to the computed interval itself.
DB_WRITE_RETRY = RetryPolicy(
    initial_interval=datetime.timedelta(seconds=4),
    backoff_coefficient=2.0,
    maximum_interval=datetime.timedelta(seconds=60),
    maximum_attempts=5,
    non_retryable_error_types=["ValueError"],
)
