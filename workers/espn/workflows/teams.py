import datetime

from temporalio import workflow

with workflow.unsafe.imports_passed_through():
    from activities.teams import ESPNLeagueSyncParams, fetch_and_upsert_teams, mark_teams_fetched


@workflow.defn
class ESPNTeamSyncWorkflow:
    @workflow.run
    async def run(self, params: ESPNLeagueSyncParams) -> None:
        await workflow.execute_activity(
            fetch_and_upsert_teams,
            params,
            start_to_close_timeout=datetime.timedelta(minutes=5),
        )
        await workflow.execute_activity(
            mark_teams_fetched,
            params.espn_league_id,
            start_to_close_timeout=datetime.timedelta(minutes=1),
        )
