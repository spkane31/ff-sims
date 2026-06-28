import datetime

from temporalio import workflow

with workflow.unsafe.imports_passed_through():
    from activities.schedule import fetch_and_upsert_schedule, mark_schedule_fetched
    from activities.teams import ESPNLeagueSyncParams

_LONG = dict(start_to_close_timeout=datetime.timedelta(minutes=30))
_SHORT = dict(start_to_close_timeout=datetime.timedelta(seconds=30))


@workflow.defn
class ESPNScheduleSyncWorkflow:
    @workflow.run
    async def run(self, params: ESPNLeagueSyncParams) -> None:
        await workflow.execute_activity(fetch_and_upsert_schedule, params, **_LONG)
        await workflow.execute_activity(mark_schedule_fetched, params.espn_league_id, **_SHORT)
