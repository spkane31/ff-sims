import datetime

from temporalio import workflow
from temporalio.common import RetryPolicy

with workflow.unsafe.imports_passed_through():
    from activities.teams import ESPNLeagueSyncParams, fetch_and_upsert_teams, mark_teams_fetched

# ValueError signals a permanent precondition failure (e.g. league/credentials
# not registered) from resolve_league_id / get_espn_credentials — retrying it
# forever just wastes attempts and hides the failure. maximum_attempts bounds
# every other (transient) failure too, so a broken activity fails loud instead
# of silently retrying indefinitely.
_RETRY = RetryPolicy(maximum_attempts=5, non_retryable_error_types=["ValueError"])


@workflow.defn
class ESPNTeamSyncWorkflow:
    @workflow.run
    async def run(self, params: ESPNLeagueSyncParams) -> None:
        await workflow.execute_activity(
            fetch_and_upsert_teams,
            params,
            start_to_close_timeout=datetime.timedelta(minutes=5),
            retry_policy=_RETRY,
        )
        await workflow.execute_activity(
            mark_teams_fetched,
            params.espn_league_id,
            start_to_close_timeout=datetime.timedelta(minutes=1),
            retry_policy=_RETRY,
        )
