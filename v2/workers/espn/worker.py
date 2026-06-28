"""
ESPN Temporal worker entry point.

Connects to Temporal Cloud (TEMPORAL_NAMESPACE_ENDPOINT set) or a local dev server,
registers all ESPN workflows and activities on the 'espn-sync' task queue, creates the
weekly Tuesday 8 AM EST schedule (idempotent), then polls indefinitely.

Temporal Cloud env vars:
  TEMPORAL_NAMESPACE_ENDPOINT     e.g. ff-sims.b3i2g.tmprl-test.cloud:7233
  TEMPORAL_NAMESPACE              e.g. ff-sims.b3i2g
  TEMPORAL_API_KEY                API key
  TEMPORAL_TLS_DISABLE_HOST_VERIFY=true  for tmprl-test.cloud (self-signed cert)

Local dev server fallback:
  TEMPORAL_HOST                default localhost:7233
  TEMPORAL_NAMESPACE           default "default"

Database:
  DATABASE_URL                 PostgreSQL connection string
"""
import asyncio
import logging
import os
import socket
import ssl
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
from temporalio.service import TLSConfig
from temporalio.worker import Worker

from activities.credentials import get_any_espn_credentials, get_espn_credentials, get_espn_leagues
from activities.draft import fetch_and_upsert_draft, mark_draft_fetched
from activities.player_status import mark_players_updated, update_active_players
from activities.schedule import fetch_and_upsert_schedule, mark_schedule_fetched
from activities.teams import fetch_and_upsert_teams, mark_teams_fetched
from activities.transactions import fetch_and_upsert_transactions, mark_transactions_fetched
from workflows.dispatcher import ESPNSyncDispatcher
from workflows.draft import ESPNDraftSyncWorkflow
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


def _fetch_server_tls_config(endpoint: str) -> TLSConfig:
    """Fetch the server's full cert chain and trust it — for tmprl-test.cloud (custom CA).

    Uses get_unverified_chain() (Python 3.13+) to capture the full chain including
    intermediates and root CA, so rustls can validate the certificate.
    """
    host, port_str = endpoint.rsplit(":", 1)
    ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
    ctx.check_hostname = False
    ctx.verify_mode = ssl.CERT_NONE
    with socket.create_connection((host, int(port_str))) as raw:
        with ctx.wrap_socket(raw, server_hostname=host) as ssock:
            try:
                chain: list[bytes] = ssock.get_unverified_chain()  # Python 3.13+
            except AttributeError:
                chain = [ssock.getpeercert(binary_form=True)]
    return TLSConfig(
        server_root_ca_cert=b"".join(ssl.DER_cert_to_PEM_cert(der).encode() for der in chain)
    )


async def create_client() -> Client:
    if endpoint := os.getenv("TEMPORAL_NAMESPACE_ENDPOINT"):
        if os.getenv("TEMPORAL_TLS_DISABLE_HOST_VERIFY") == "true":
            tls: bool | TLSConfig = _fetch_server_tls_config(endpoint)
        else:
            tls = True
        return await Client.connect(
            endpoint,
            namespace=os.environ["TEMPORAL_NAMESPACE"],
            tls=tls,
            api_key=os.getenv("TEMPORAL_API_KEY"),
        )
    return await Client.connect(
        os.getenv("TEMPORAL_HOST", "localhost:7233"),
        namespace=os.getenv("TEMPORAL_NAMESPACE", "default"),
    )


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
    ]

    all_workflows = [
        ESPNSyncDispatcher,
        LeagueESPNSyncWorkflow,
        ESPNTeamSyncWorkflow,
        ESPNScheduleSyncWorkflow,
        ESPNDraftSyncWorkflow,
        ESPNTransactionSyncWorkflow,
        ESPNPlayerStatusSyncWorkflow,
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
