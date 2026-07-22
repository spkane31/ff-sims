"""
ExpectedWinsBackfillWorkflow — manually-triggered workflow for backfilling or
re-running expected wins calculations for a league.

Typical use cases:
  • First-time backfill after adding a historical league
  • Re-running after fixing a data issue
  • Ad-hoc debugging for a specific year

Trigger via Temporal CLI:
  temporal workflow start \
    --task-queue espn-sync \
    --type ExpectedWinsBackfillWorkflow \
    --input '{"espn_league_id": "12345", "year": null}'

Or for a specific year:
  temporal workflow start \
    --task-queue espn-sync \
    --type ExpectedWinsBackfillWorkflow \
    --input '{"espn_league_id": "12345", "year": 2024}'
"""
import datetime
from dataclasses import dataclass

from temporalio import workflow

with workflow.unsafe.imports_passed_through():
    from activities.expected_wins import (
        ExpectedWinsParams,
        calculate_and_store_expected_wins,
        get_matchup_years,
    )
    from workflows.retry import DB_WRITE_RETRY as _RETRY

TASK_QUEUE = "espn-sync"
_LONG = dict(start_to_close_timeout=datetime.timedelta(minutes=30), retry_policy=_RETRY)
_SHORT = dict(start_to_close_timeout=datetime.timedelta(seconds=30), retry_policy=_RETRY)


@dataclass
class ExpectedWinsBackfillParams:
    espn_league_id: str
    year: int | None = None  # None = backfill all years


@workflow.defn
class ExpectedWinsBackfillWorkflow:
    @workflow.run
    async def run(self, params: ExpectedWinsBackfillParams) -> None:
        if params.year is not None:
            years = [params.year]
        else:
            years = await workflow.execute_activity(
                get_matchup_years, params.espn_league_id, **_SHORT
            )
            if not years:
                workflow.logger.info(
                    "No matchup years found for league %s", params.espn_league_id
                )
                return

        workflow.logger.info(
            "Backfilling expected wins for league %s, years=%s",
            params.espn_league_id, years,
        )

        for year in years:
            await workflow.execute_activity(
                calculate_and_store_expected_wins,
                ExpectedWinsParams(espn_league_id=params.espn_league_id, year=year),
                **_LONG,
            )
            workflow.logger.info(
                "Completed expected wins for league %s year %d",
                params.espn_league_id, year,
            )
