import datetime

from temporalio import workflow

with workflow.unsafe.imports_passed_through():
    from activities.credentials import get_espn_leagues
    from workflows.league_sync import LeagueDispatchParams, LeagueESPNSyncWorkflow
    from workflows.player_status import ESPNPlayerStatusSyncWorkflow

TASK_QUEUE = "espn-sync"
_SHORT = dict(start_to_close_timeout=datetime.timedelta(seconds=30))


@workflow.defn
class ESPNSyncDispatcher:
    @workflow.run
    async def run(self, year: int | None = None) -> None:
        effective_year = year if year is not None else workflow.now().year

        league_ids: list[str] = await workflow.execute_activity(
            get_espn_leagues, effective_year, **_SHORT
        )

        try:
            await workflow.start_child_workflow(
                ESPNPlayerStatusSyncWorkflow.run,
                effective_year,
                id=f"espn-player-status-{effective_year}",
                task_queue=TASK_QUEUE,
                parent_close_policy=workflow.ParentClosePolicy.ABANDON,
            )
        except Exception as exc:
            workflow.logger.warning("Failed to start ESPNPlayerStatusSyncWorkflow: %s", exc)

        for league_id in league_ids:
            try:
                await workflow.start_child_workflow(
                    LeagueESPNSyncWorkflow.run,
                    LeagueDispatchParams(espn_league_id=league_id, year=effective_year),
                    id=f"espn-league-{league_id}-{effective_year}",
                    task_queue=TASK_QUEUE,
                    parent_close_policy=workflow.ParentClosePolicy.ABANDON,
                )
            except Exception as exc:
                workflow.logger.warning(
                    "Failed to start LeagueESPNSyncWorkflow for %s: %s", league_id, exc
                )
