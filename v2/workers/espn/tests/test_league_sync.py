import concurrent.futures

import pytest
from temporalio import activity
from temporalio.testing import WorkflowEnvironment
from temporalio.worker import Worker

from activities.credentials import ESPNCredentials
from activities.teams import ESPNLeagueSyncParams
from workflows.draft import ESPNDraftSyncWorkflow
from workflows.league_sync import LeagueDispatchParams, LeagueESPNSyncWorkflow
from workflows.schedule import ESPNScheduleSyncWorkflow
from workflows.teams import ESPNTeamSyncWorkflow
from workflows.transactions import ESPNTransactionSyncWorkflow


@activity.defn(name="get_espn_credentials")
def mock_get_credentials(espn_league_id: str) -> ESPNCredentials:
    return ESPNCredentials(espn_s2="s2", swid="swid")


@pytest.mark.asyncio
async def test_league_sync_completes_without_error():
    async with await WorkflowEnvironment.start_time_skipping() as env:

        async def noop_run(params: ESPNLeagueSyncParams) -> None:
            pass

        original_teams = ESPNTeamSyncWorkflow.run
        original_schedule = ESPNScheduleSyncWorkflow.run
        original_draft = ESPNDraftSyncWorkflow.run
        original_tx = ESPNTransactionSyncWorkflow.run

        ESPNTeamSyncWorkflow.run = noop_run
        ESPNScheduleSyncWorkflow.run = noop_run
        ESPNDraftSyncWorkflow.run = noop_run
        ESPNTransactionSyncWorkflow.run = noop_run

        try:
            with concurrent.futures.ThreadPoolExecutor() as executor:
              async with Worker(
                env.client,
                task_queue="test-espn-sync",
                workflows=[
                    LeagueESPNSyncWorkflow,
                    ESPNTeamSyncWorkflow,
                    ESPNScheduleSyncWorkflow,
                    ESPNDraftSyncWorkflow,
                    ESPNTransactionSyncWorkflow,
                ],
                activities=[mock_get_credentials],
                activity_executor=executor,
              ):
                  await env.client.execute_workflow(
                    LeagueESPNSyncWorkflow.run,
                    LeagueDispatchParams(espn_league_id="123", year=2025),
                    id="test-league-sync-123",
                    task_queue="test-espn-sync",
                  )
        finally:
            ESPNTeamSyncWorkflow.run = original_teams
            ESPNScheduleSyncWorkflow.run = original_schedule
            ESPNDraftSyncWorkflow.run = original_draft
            ESPNTransactionSyncWorkflow.run = original_tx
