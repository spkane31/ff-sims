"""
ESPN Temporal worker entry point.

Connects to Temporal Cloud (TEMPORAL_NAMESPACE_ENDPOINT set) or a local dev server,
registers all ESPN workflows and activities on the 'espn-sync' task queue, creates the
weekly Tuesday 8 AM EST schedule (idempotent), then polls indefinitely.

Temporal Cloud env vars:
  TEMPORAL_NAMESPACE_ENDPOINT     e.g. ff-sims.b3i2g.tmprl-test.cloud:7233
  TEMPORAL_NAMESPACE              e.g. ff-sims.b3i2g
  TEMPORAL_API_KEY                API key

Local dev server fallback:
  TEMPORAL_HOST                default localhost:7233
  TEMPORAL_NAMESPACE           default "default"

Database:
  DATABASE_URL                 PostgreSQL connection string
"""
import asyncio
import logging
from concurrent.futures import ThreadPoolExecutor

from dotenv import load_dotenv
from temporalio.client import (
    Client,
    Schedule,
    ScheduleActionStartWorkflow,
    ScheduleCalendarSpec,
    ScheduleRange,
    ScheduleSpec,
)
from temporalio.worker import Worker

from activities.credentials import get_any_espn_credentials, get_espn_credentials, get_espn_leagues
from activities.draft import fetch_and_upsert_draft, mark_draft_fetched
from activities.expected_wins import calculate_and_store_expected_wins, get_matchup_years
from activities.player_status import mark_players_updated, update_active_players
from activities.schedule import fetch_and_upsert_schedule, mark_schedule_fetched
from activities.teams import fetch_and_upsert_teams, mark_teams_fetched
from activities.transactions import fetch_and_upsert_transactions, mark_transactions_fetched
from temporal_client import create_client
from workflows.dispatcher import ESPNSyncDispatcher
from workflows.draft import ESPNDraftSyncWorkflow
from workflows.expected_wins import ExpectedWinsBackfillWorkflow
from workflows.league_sync import LeagueESPNSyncWorkflow
from workflows.player_status import ESPNPlayerStatusSyncWorkflow
from workflows.schedule import ESPNScheduleSyncWorkflow
from workflows.teams import ESPNTeamSyncWorkflow
from workflows.transactions import ESPNTransactionSyncWorkflow

load_dotenv()
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

TASK_QUEUE = "espn-sync"
SCHEDULE_ID = "espn-sync-schedule"


async def register_schedule(client: Client) -> None:
    try:
        await client.create_schedule(
            SCHEDULE_ID,
            Schedule(
                action=ScheduleActionStartWorkflow(
                    ESPNSyncDispatcher.run,
                    id="espn-sync-dispatcher",
                    task_queue=TASK_QUEUE,
                ),
                spec=ScheduleSpec(
                    calendars=[
                        ScheduleCalendarSpec(
                            # Tuesday = 2 (0=Sunday)
                            day_of_week=[ScheduleRange(2)],
                            hour=[ScheduleRange(13)],    # 13:00 UTC = 8:00 AM EST
                            minute=[ScheduleRange(0)],
                        )
                    ]
                ),
            ),
        )
        logger.info("Registered schedule %s", SCHEDULE_ID)
    except Exception as exc:
        logger.info("Schedule %s already exists, skipping: %s", SCHEDULE_ID, exc)


async def main() -> None:
    client = await create_client()
    await register_schedule(client)

    all_activities = [
        get_espn_leagues,
        get_espn_credentials,
        get_any_espn_credentials,
        fetch_and_upsert_teams,
        mark_teams_fetched,
        fetch_and_upsert_schedule,
        mark_schedule_fetched,
        fetch_and_upsert_draft,
        mark_draft_fetched,
        fetch_and_upsert_transactions,
        mark_transactions_fetched,
        update_active_players,
        mark_players_updated,
        get_matchup_years,
        calculate_and_store_expected_wins,
    ]

    all_workflows = [
        ESPNSyncDispatcher,
        LeagueESPNSyncWorkflow,
        ESPNTeamSyncWorkflow,
        ESPNScheduleSyncWorkflow,
        ESPNDraftSyncWorkflow,
        ESPNTransactionSyncWorkflow,
        ESPNPlayerStatusSyncWorkflow,
        ExpectedWinsBackfillWorkflow,
    ]

    with ThreadPoolExecutor(max_workers=20) as executor:
        worker = Worker(
            client,
            task_queue=TASK_QUEUE,
            workflows=all_workflows,
            activities=all_activities,
            activity_executor=executor,
        )
        logger.info("ESPN Temporal worker started on task queue '%s'", TASK_QUEUE)
        await worker.run()


if __name__ == "__main__":
    asyncio.run(main())
