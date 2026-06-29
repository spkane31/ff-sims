# ESPN Temporal Workers Design

**Date:** 2026-06-27
**Branch:** feature/multi-league
**Status:** Approved

## Overview

Migrate `scripts/main.py` into Python Temporal workers that run inside the `v2/` Dockerfile alongside the existing Go Temporal workers. The Python workers fetch ESPN fantasy football data (teams, schedule/box scores, drafts, transactions, active player statuses) and write directly to the database, replacing the current two-step file-dump → Go ETL pipeline.

ESPN data fetching requires the `espn-api` Python library, which has no Go equivalent and is not worth rewriting. A multi-language Temporal deployment is the right fit: Go handles Sleeper and the HTTP server; Python handles ESPN.

## Why This Approach

The existing Sleeper workers (Go) follow a dispatcher → per-league child workflow → activities pattern. The ESPN workers adopt the same pattern in Python, registering against the same Temporal Cloud namespace. Temporal's polyglot support means both worker sets share one cluster and one schedule registry with no additional infrastructure.

## Database

### New Table: `espn_league_credentials`

```sql
CREATE TABLE espn_league_credentials (
    espn_league_id               TEXT PRIMARY KEY,
    espn_s2                      TEXT NOT NULL,
    swid                         TEXT NOT NULL,
    last_teams_fetched_at        TIMESTAMPTZ,
    last_schedule_fetched_at     TIMESTAMPTZ,
    last_draft_fetched_at        TIMESTAMPTZ,
    last_transactions_fetched_at TIMESTAMPTZ,
    last_players_updated_at      TIMESTAMPTZ,
    created_at                   TIMESTAMPTZ DEFAULT NOW(),
    updated_at                   TIMESTAMPTZ DEFAULT NOW()
);
```

`espn_league_id` corresponds to `leagues.external_id` where `leagues.platform = 'ESPN'`. No schema changes are required to the existing `leagues` table.

Per-operation `last_*_fetched_at` timestamps mirror the pattern on `sleeper_leagues` (e.g., `last_drafts_fetched_at`). Each child workflow checks its own timestamp as an idempotency guard — if already fetched in this cycle, the activity returns early.

`espn_s2` and `swid` are ESPN session cookies required to authenticate private league API requests. They are stored per-league because different leagues may belong to different ESPN accounts.

## Workflow Topology

```
[Schedule: Tuesday 13:00 UTC = 8:00 AM EST]
        │
        ▼
ESPNSyncDispatcher(year=<current>)
  Activity: get_espn_leagues(year) → [espn_league_id, ...]
        │
        ├─▶ ESPNPlayerStatusSyncWorkflow   [spawned ONCE — global, not per-league]
        │     Activity: get_any_espn_credentials() → {espn_s2, swid}
        │     Activity: update_active_players(espn_s2, swid)
        │     Activity: mark_players_updated
        │
        └─ for each espn_league_id (ABANDON, fire-and-forget):
                ▼
        LeagueESPNSyncWorkflow(espn_league_id, year)
          Activity: get_espn_credentials(espn_league_id) → {espn_s2, swid}
                │
                ├─ parallel child workflows (ABANDON):
                ├─▶ ESPNTeamSyncWorkflow(espn_league_id, year, espn_s2, swid)
                │     Activity: fetch_and_upsert_teams
                │     Activity: mark_teams_fetched
                │
                ├─▶ ESPNScheduleSyncWorkflow(espn_league_id, year, espn_s2, swid)
                │     Activity: fetch_and_upsert_schedule  ← matchups, box scores, pure matchups
                │     Activity: mark_schedule_fetched
                │
                ├─▶ ESPNDraftSyncWorkflow(espn_league_id, year, espn_s2, swid)
                │     Activity: fetch_and_upsert_draft
                │     Activity: mark_draft_fetched
                │
                └─▶ ESPNTransactionSyncWorkflow(espn_league_id, year, espn_s2, swid)
                      Activity: fetch_and_upsert_transactions
                      Activity: mark_transactions_fetched
```

### Key Decisions

- **`ESPNPlayerStatusSyncWorkflow` runs once per dispatch cycle**, not per league. Updating active player status in the `players` table is a global operation — it queries all active players and calls `league.player_info()` on any valid ESPN session. Running it N times per league would be redundant. `get_any_espn_credentials()` fetches the first non-null credential row.
- **Credentials are fetched once in `LeagueESPNSyncWorkflow`** and passed into child workflows. Avoids five redundant DB queries per league.
- **All child workflows use `PARENT_CLOSE_POLICY_ABANDON`** so they continue independently if the parent exits.
- **Single task queue: `espn-sync`**. One Python worker process handles all ESPN workflows and activities.
- **All activities use upsert semantics** (INSERT ... ON CONFLICT DO UPDATE / DO NOTHING), making every activity safe to retry.
- **`LeagueESPNSyncWorkflow` does not wait for child workflows to complete** — it fires all four children and exits, letting each run independently.

### Year Parameter

`ESPNSyncDispatcher` accepts an optional `year` parameter that defaults to the current calendar year. This is used by:
- `get_espn_leagues(year)` — resolves the ESPN `League` object for the correct season
- All child workflow activities — passed through to `espn_api.football.League(year=year, ...)`

## Historical Backfill

When a new ESPN league is added to the database with credentials, **the next scheduled Tuesday run automatically syncs the current year** — `get_espn_leagues()` queries all leagues in `espn_league_credentials` with no date filter, so new leagues pass the staleness check immediately (all `last_*_fetched_at` are NULL).

**Historical seasons require a manual trigger.** To backfill prior years, run the dispatcher manually via the Temporal CLI once per year:

```bash
# Backfill 2019 through 2024 for all leagues
for year in 2019 2020 2021 2022 2023 2024; do
  temporal workflow start \
    --type ESPNSyncDispatcher \
    --task-queue espn-sync \
    --workflow-id "espn-backfill-${year}" \
    --input "{\"year\": ${year}}"
done

# Or backfill a single league for a single year
temporal workflow start \
  --type LeagueESPNSyncWorkflow \
  --task-queue espn-sync \
  --workflow-id "espn-league-backfill-345674-2022" \
  --input '{"espn_league_id": "345674", "year": 2022}'
```

All activities are idempotent so re-running a year that already has data is safe.

## File Structure

```
v2/workers/espn/
├── pyproject.toml           # temporalio, espn-api, psycopg[binary], python-dotenv
├── uv.lock
├── worker.py                # entry point: Temporal client, worker setup, schedule registration
├── db.py                    # psycopg connection helper (reads DATABASE_URL env var)
├── workflows/
│   ├── __init__.py
│   ├── dispatcher.py        # ESPNSyncDispatcher
│   ├── league_sync.py       # LeagueESPNSyncWorkflow
│   ├── teams.py             # ESPNTeamSyncWorkflow
│   ├── schedule.py          # ESPNScheduleSyncWorkflow
│   ├── draft.py             # ESPNDraftSyncWorkflow
│   ├── transactions.py      # ESPNTransactionSyncWorkflow
│   └── player_status.py     # ESPNPlayerStatusSyncWorkflow
└── activities/
    ├── __init__.py
    ├── credentials.py       # get_espn_leagues, get_espn_credentials, get_any_espn_credentials
    ├── teams.py             # fetch_and_upsert_teams, mark_teams_fetched
    ├── schedule.py          # fetch_and_upsert_schedule, mark_schedule_fetched
    ├── draft.py             # fetch_and_upsert_draft, mark_draft_fetched
    ├── transactions.py      # fetch_and_upsert_transactions, mark_transactions_fetched
    └── player_status.py     # update_active_players, mark_players_updated
```

### Why `uv`

The existing `scripts/` project already uses `uv` with a `pyproject.toml` and `uv.lock`. The ESPN worker adopts the same toolchain for consistency.

## Dockerfile Changes

The `v2/Dockerfile` gains a Python build stage and an updated entrypoint:

```dockerfile
# Stage: Python ESPN worker
FROM python:3.12-slim AS espn-worker-builder
WORKDIR /app/workers/espn
RUN pip install uv
COPY workers/espn/pyproject.toml workers/espn/uv.lock ./
RUN uv sync --frozen --no-dev
COPY workers/espn/ ./

# In the final runtime stage, copy the Python worker alongside Go binaries:
COPY --from=espn-worker-builder /app/workers/espn /app/workers/espn
COPY --from=espn-worker-builder /usr/local/bin/uv /usr/local/bin/uv
COPY --from=espn-worker-builder /usr/local/lib/python3.12 /usr/local/lib/python3.12
COPY --from=espn-worker-builder /usr/local/bin/python3.12 /usr/local/bin/python3.12
RUN ln -sf /usr/local/bin/python3.12 /usr/local/bin/python3

# Updated entrypoint — three processes:
RUN printf '#!/bin/sh\n\
/app/backend/worker &\n\
cd /app/workers/espn && uv run python worker.py &\n\
exec /app/backend/main\n' > /entrypoint.sh && chmod +x /entrypoint.sh
```

### Environment Variables

The Python worker reads the same env vars already wired into the container:

| Variable | Purpose |
|---|---|
| `DATABASE_URL` | PostgreSQL connection string |
| `TEMPORAL_NAMESPACE_ENDPOINT` | Temporal Cloud host (e.g. `ff-sims.b3i2g.tmprl-test.cloud:7233`) |
| `TEMPORAL_NAMESPACE` | Temporal namespace (e.g. `ff-sims.b3i2g`) |
| `TEMPORAL_API_KEY` | Temporal Cloud API key |
| `TEMPORAL_HOST` | Local dev fallback (default `localhost:7233`) |

No new secrets infrastructure is required.

## Schedule Registration

The Python `worker.py` registers the ESPN schedule on startup using the same upsert-on-create pattern as the Go `schedules/register.go`:

```python
# Tuesday 13:00 UTC = 8:00 AM EST
await client.schedule_client.create_schedule(
    "espn-sync-schedule",
    Schedule(
        spec=ScheduleSpec(
            calendars=[ScheduleCalendarSpec(
                day_of_week=[ScheduleRange(2)],   # Tuesday
                hour=[ScheduleRange(13)],          # 13:00 UTC
                minute=[ScheduleRange(0)],
            )]
        ),
        action=ScheduleActionStartWorkflow(
            ESPNSyncDispatcher.run,
            task_queue="espn-sync",
        ),
    ),
)
# If schedule already exists, swallow the AlreadyExistsError (idempotent)
```

## What Gets Deprecated / Removed

- `scripts/main.py` — replaced by the Python Temporal worker activities
- `v2/backend/cmd/etl/` — the Go ETL (file-read → DB upload pipeline) is no longer needed once all ESPN data writes directly from the Python activities
- `v2/data/` directory — no longer needed as an intermediate JSON staging area for ESPN data

The Sleeper Go workers (`cmd/worker/`) are unaffected.

## Testing

Each activity should be tested independently with a real (test) database connection — mocking the DB was ruled out on this project to avoid mock/prod divergence. Workflow logic can be tested with the Temporal Python SDK's `WorkflowEnvironment` time-skipping environment, with activities mocked at the workflow test layer only.
