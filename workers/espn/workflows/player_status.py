import datetime

from temporalio import workflow

with workflow.unsafe.imports_passed_through():
    from activities.credentials import get_any_espn_credentials
    from activities.player_status import PlayerStatusParams, mark_players_updated, update_active_players
    from workflows.retry import DB_WRITE_RETRY as _RETRY

_LONG = dict(start_to_close_timeout=datetime.timedelta(minutes=30), retry_policy=_RETRY)
_SHORT = dict(start_to_close_timeout=datetime.timedelta(seconds=30), retry_policy=_RETRY)


@workflow.defn
class ESPNPlayerStatusSyncWorkflow:
    @workflow.run
    async def run(self, year: int) -> None:
        creds = await workflow.execute_activity(get_any_espn_credentials, **_SHORT)
        params = PlayerStatusParams(
            espn_league_id=creds.espn_league_id,
            espn_s2=creds.espn_s2,
            swid=creds.swid,
            year=year,
        )
        await workflow.execute_activity(update_active_players, params, **_LONG)
        await workflow.execute_activity(mark_players_updated, **_SHORT)
