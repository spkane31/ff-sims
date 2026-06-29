import datetime

from temporalio import workflow

with workflow.unsafe.imports_passed_through():
    from activities.teams import ESPNLeagueSyncParams
    from activities.transactions import fetch_and_upsert_transactions, mark_transactions_fetched

_LONG = dict(start_to_close_timeout=datetime.timedelta(minutes=30))
_SHORT = dict(start_to_close_timeout=datetime.timedelta(seconds=30))


@workflow.defn
class ESPNTransactionSyncWorkflow:
    @workflow.run
    async def run(self, params: ESPNLeagueSyncParams) -> None:
        await workflow.execute_activity(fetch_and_upsert_transactions, params, **_LONG)
        await workflow.execute_activity(mark_transactions_fetched, params.espn_league_id, **_SHORT)
