import datetime

from temporalio import workflow
from temporalio.common import RetryPolicy

with workflow.unsafe.imports_passed_through():
    from activities.expected_wins import ExpectedWinsParams, calculate_and_store_expected_wins
    from activities.schedule import fetch_and_upsert_schedule, mark_schedule_fetched
    from activities.teams import ESPNLeagueSyncParams

# See workflows/teams.py for why ValueError is non-retryable here.
_RETRY = RetryPolicy(maximum_attempts=5, non_retryable_error_types=["ValueError"])
_LONG = dict(start_to_close_timeout=datetime.timedelta(minutes=30), retry_policy=_RETRY)
_SHORT = dict(start_to_close_timeout=datetime.timedelta(seconds=30), retry_policy=_RETRY)


@workflow.defn
class ESPNScheduleSyncWorkflow:
    @workflow.run
    async def run(self, params: ESPNLeagueSyncParams) -> None:
        await workflow.execute_activity(fetch_and_upsert_schedule, params, **_LONG)
        await workflow.execute_activity(
            calculate_and_store_expected_wins,
            ExpectedWinsParams(espn_league_id=params.espn_league_id, year=params.year),
            **_LONG,
        )
        await workflow.execute_activity(mark_schedule_fetched, params.espn_league_id, **_SHORT)
