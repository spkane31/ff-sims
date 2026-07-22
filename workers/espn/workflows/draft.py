import datetime

from temporalio import workflow

with workflow.unsafe.imports_passed_through():
    from activities.draft import fetch_and_upsert_draft, mark_draft_fetched
    from activities.teams import ESPNLeagueSyncParams
    from workflows.retry import DB_WRITE_RETRY as _RETRY

_LONG = dict(start_to_close_timeout=datetime.timedelta(minutes=30), retry_policy=_RETRY)
_SHORT = dict(start_to_close_timeout=datetime.timedelta(seconds=30), retry_policy=_RETRY)


@workflow.defn
class ESPNDraftSyncWorkflow:
    @workflow.run
    async def run(self, params: ESPNLeagueSyncParams) -> None:
        await workflow.execute_activity(fetch_and_upsert_draft, params, **_LONG)
        await workflow.execute_activity(mark_draft_fetched, params.espn_league_id, **_SHORT)
