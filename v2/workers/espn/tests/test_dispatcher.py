import concurrent.futures

import pytest
from temporalio import activity
from temporalio.testing import WorkflowEnvironment
from temporalio.worker import Worker

from workflows.dispatcher import ESPNSyncDispatcher
from workflows.league_sync import LeagueDispatchParams, LeagueESPNSyncWorkflow
from workflows.player_status import ESPNPlayerStatusSyncWorkflow


@activity.defn(name="get_espn_leagues")
def mock_get_espn_leagues(year: int) -> list[str]:
    return []


@pytest.mark.asyncio
async def test_dispatcher_completes_with_no_leagues():
    async with await WorkflowEnvironment.start_time_skipping() as env:

        async def noop_league_run(params: LeagueDispatchParams) -> None:
            pass

        async def noop_player_run(year: int) -> None:
            pass

        original_league = LeagueESPNSyncWorkflow.run
        original_player = ESPNPlayerStatusSyncWorkflow.run
        LeagueESPNSyncWorkflow.run = noop_league_run
        ESPNPlayerStatusSyncWorkflow.run = noop_player_run

        try:
            with concurrent.futures.ThreadPoolExecutor() as executor:
                async with Worker(
                    env.client,
                    task_queue="test-espn-sync",
                    workflows=[ESPNSyncDispatcher, LeagueESPNSyncWorkflow, ESPNPlayerStatusSyncWorkflow],
                    activities=[mock_get_espn_leagues],
                    activity_executor=executor,
                ):
                    await env.client.execute_workflow(
                        ESPNSyncDispatcher.run,
                        id="test-dispatcher-empty",
                        task_queue="test-espn-sync",
                    )
        finally:
            LeagueESPNSyncWorkflow.run = original_league
            ESPNPlayerStatusSyncWorkflow.run = original_player
