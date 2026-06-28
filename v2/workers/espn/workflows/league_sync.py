import datetime
from dataclasses import dataclass

from temporalio import workflow

with workflow.unsafe.imports_passed_through():
    from activities.credentials import get_espn_credentials
    from activities.teams import ESPNLeagueSyncParams
    from workflows.draft import ESPNDraftSyncWorkflow
    from workflows.schedule import ESPNScheduleSyncWorkflow
    from workflows.teams import ESPNTeamSyncWorkflow
    from workflows.transactions import ESPNTransactionSyncWorkflow

TASK_QUEUE = "espn-sync"
_SHORT = dict(start_to_close_timeout=datetime.timedelta(seconds=30))


@dataclass
class LeagueDispatchParams:
    espn_league_id: str
    year: int


@workflow.defn
class LeagueESPNSyncWorkflow:
    @workflow.run
    async def run(self, params: LeagueDispatchParams) -> None:
        creds = await workflow.execute_activity(
            get_espn_credentials, params.espn_league_id, **_SHORT
        )
        sync_params = ESPNLeagueSyncParams(
            espn_league_id=params.espn_league_id,
            year=params.year,
            espn_s2=creds.espn_s2,
            swid=creds.swid,
        )

        child_configs = [
            (ESPNTeamSyncWorkflow, "teams"),
            (ESPNScheduleSyncWorkflow, "schedule"),
            (ESPNDraftSyncWorkflow, "draft"),
            (ESPNTransactionSyncWorkflow, "transactions"),
        ]
        for child_cls, suffix in child_configs:
            try:
                await workflow.start_child_workflow(
                    child_cls.run,
                    sync_params,
                    id=f"espn-{suffix}-{params.espn_league_id}-{params.year}",
                    task_queue=TASK_QUEUE,
                    parent_close_policy=workflow.ParentClosePolicy.ABANDON,
                )
            except Exception as exc:
                workflow.logger.warning(
                    "Failed to start %s child for league %s: %s",
                    suffix, params.espn_league_id, exc,
                )
