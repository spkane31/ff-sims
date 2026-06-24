# Temporal Sleeper Workers — Design Spec

**Date:** 2026-06-24
**Status:** Approved
**Author:** Sean Kane

## Context

The ff-sims project needs a player valuation model for Sleeper fantasy football. The model requires a large corpus of historical trade and draft data to compute relative player values, augmented by recency weighting and aging curves. Today there is no automated mechanism to collect Sleeper data — the existing ETL pipeline only handles ESPN leagues via a manual Python script.

This spec describes a new `workers/` Go service that uses Temporal to continuously scrape the Sleeper API: discovering leagues by recursive user-graph expansion, then fetching per-league trade and draft data. The design is explicitly built to later absorb the ESPN ETL migration and the valuation computation step.

---

## Scope

**In scope (Phase 1):**
- New `workers/` Go module with Temporal workers
- Sleeper API client (no auth required)
- Batch-based user/league discovery with `last_fetched_at` LRF queue
- Per-league trade and draft data fetching
- Daily full player database sync (Sleeper → Postgres, with ESPN ID cross-reference)
- New Postgres tables for all Sleeper data
- One-shot seed CLI to bootstrap from a known username
- Temporal schedules for automated recurring runs

**Out of scope (future phases):**
- ESPN ETL migration to Temporal
- Valuation computation workflows
- Frontend-facing valuation endpoints

---

## Directory Structure

New `workers/` directory at the repository root, alongside `v2/` and `scripts/`. This will become a top-level module as the monorepo flattens to `workers/`, `backend/`, `frontend/`, `scripts/`.

```
workers/
├── go.mod                              # module workers
├── cmd/
│   ├── worker/main.go                  # long-running binary: registers workflows/activities, starts polling
│   └── seed/main.go                    # one-shot CLI: bootstrap from --username flag
├── internal/
│   ├── db/
│   │   ├── postgres.go                 # connects via DATABASE_URL env var (same DB as backend)
│   │   └── migrations/                 # goose SQL migrations for Sleeper tables
│   ├── models/
│   │   ├── sleeper_user.go
│   │   ├── sleeper_league.go
│   │   ├── sleeper_player.go
│   │   ├── sleeper_draft.go
│   │   └── sleeper_transaction.go
│   ├── sleeper/
│   │   ├── client.go                   # HTTP client with 429 retry + rate awareness
│   │   └── types.go                    # API response structs
│   ├── activities/
│   │   ├── discovery.go                # GetStaleUsers, FetchUserLeagues, FetchLeagueMembers, MarkUserFetched
│   │   ├── data_fetch.go               # GetStaleLeagues, FetchLeagueDrafts, FetchDraftPicks, FetchLeagueTransactions, MarkLeagueFetched
│   │   └── player_sync.go              # FetchAndUpsertAllPlayers (heartbeating)
│   └── workflows/
│       ├── dispatcher.go               # DiscoveryBatchDispatcher (parent, scheduled)
│       ├── discovery.go                # UserDiscoveryWorkflow (child)
│       ├── league_sync.go              # LeagueSyncWorkflow (child)
│       └── player_sync.go              # PlayerDatabaseSyncWorkflow (scheduled)
└── schedules/
    └── register.go                     # creates/updates Temporal schedules on worker startup
```

---

## Database Schema

All new tables in the shared Postgres instance (same `DATABASE_URL` as `v2/backend`). Managed by goose migrations in `workers/internal/db/migrations/`.

### `sleeper_users`
```sql
CREATE TABLE sleeper_users (
    sleeper_user_id  TEXT        PRIMARY KEY,
    username         TEXT,
    display_name     TEXT,
    avatar           TEXT,
    last_fetched_at  TIMESTAMPTZ,          -- NULL = never fetched; drives LRF queue
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_sleeper_users_last_fetched ON sleeper_users (last_fetched_at ASC NULLS FIRST);
```

### `sleeper_leagues`
```sql
CREATE TABLE sleeper_leagues (
    sleeper_league_id  TEXT        PRIMARY KEY,
    name               TEXT,
    season             TEXT,              -- "2024", "2025"
    sport              TEXT,              -- "nfl"
    status             TEXT,              -- "in_season", "complete", etc.
    total_rosters      INT,
    -- Scoring format (derived from API response for easy querying)
    ppr                FLOAT,             -- 0 = standard, 0.5 = half PPR, 1.0 = full PPR (from scoring_settings.rec)
    te_premium         FLOAT,             -- bonus reception points for TEs (from scoring_settings.bonus_rec_te)
    is_superflex       BOOLEAN,           -- true if roster_positions contains "SUPER_FLEX"
    -- Full raw settings stored for future use
    scoring_settings   JSONB,             -- complete scoring_settings object from Sleeper API
    roster_positions   JSONB,             -- array of roster slot strings, e.g. ["QB","WR","SUPER_FLEX",...]
    last_fetched_at    TIMESTAMPTZ,       -- NULL = found but data not yet fetched
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_sleeper_leagues_last_fetched ON sleeper_leagues (last_fetched_at ASC NULLS FIRST);
```

### `sleeper_league_users` (junction)
```sql
CREATE TABLE sleeper_league_users (
    sleeper_league_id  TEXT REFERENCES sleeper_leagues(sleeper_league_id),
    sleeper_user_id    TEXT REFERENCES sleeper_users(sleeper_user_id),
    PRIMARY KEY (sleeper_league_id, sleeper_user_id)
);
```

### `sleeper_players`
```sql
CREATE TABLE sleeper_players (
    sleeper_player_id  TEXT        PRIMARY KEY,
    espn_id            TEXT,          -- cross-reference key into ESPN data
    yahoo_id           TEXT,
    full_name          TEXT,
    position           TEXT,
    nfl_team           TEXT,
    age                INT,
    years_exp          INT,
    last_fetched_at    TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### `sleeper_drafts`
```sql
CREATE TABLE sleeper_drafts (
    sleeper_draft_id   TEXT        PRIMARY KEY,
    sleeper_league_id  TEXT        REFERENCES sleeper_leagues(sleeper_league_id),
    type               TEXT,          -- "snake", "auction"
    status             TEXT,          -- "complete", "in_progress"
    season             TEXT,
    last_fetched_at    TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### `sleeper_draft_picks`
```sql
CREATE TABLE sleeper_draft_picks (
    sleeper_draft_id    TEXT  REFERENCES sleeper_drafts(sleeper_draft_id),
    round               INT,
    pick_no             INT,
    roster_id           INT,
    picked_by_user_id   TEXT  REFERENCES sleeper_users(sleeper_user_id),
    sleeper_player_id   TEXT  REFERENCES sleeper_players(sleeper_player_id),
    metadata            JSONB,    -- keeper, nomination, etc.
    PRIMARY KEY (sleeper_draft_id, round, pick_no)
);
```

### `sleeper_transactions`
```sql
CREATE TABLE sleeper_transactions (
    sleeper_transaction_id  TEXT        PRIMARY KEY,
    sleeper_league_id       TEXT        REFERENCES sleeper_leagues(sleeper_league_id),
    type                    TEXT,          -- "trade", "waiver", "free_agent"
    status                  TEXT,          -- "complete", "failed"
    created_at_sleeper      BIGINT,        -- Sleeper epoch-ms timestamp
    leg                     INT,           -- transaction round/week
    adds                    JSONB,         -- { player_id: roster_id }
    drops                   JSONB,         -- { player_id: roster_id }
    draft_picks             JSONB,         -- traded future picks
    waiver_budget           JSONB,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### `player_valuations` (future phase, created now as empty table)
```sql
CREATE TABLE player_valuations (
    sleeper_player_id  TEXT   REFERENCES sleeper_players(sleeper_player_id),
    valuation_date     DATE,
    raw_trade_value    FLOAT,
    recency_factor     FLOAT,
    age_curve_factor   FLOAT,
    adjusted_value     FLOAT,
    PRIMARY KEY (sleeper_player_id, valuation_date)
);
```

---

## Temporal Architecture

### Task Queues

| Queue | Purpose |
|---|---|
| `sleeper-discovery` | User/league graph expansion |
| `sleeper-data` | Per-league trade, draft, picks fetching |
| `sleeper-player-sync` | Daily full player DB refresh |

The `cmd/worker/main.go` binary polls all three queues in the same process. They can be split into separate processes later if load warrants it.

---

### Workflow: `DiscoveryBatchDispatcher`

**Queue:** `sleeper-discovery`
**Trigger:** Temporal Schedule (`sleeper-discovery-schedule`, every 15 minutes)

Parent dispatcher. Picks up stale entities and fans out to child workflows with `ABANDON` close policy — it does not wait for children and completes quickly.

```
DiscoveryBatchDispatcher
  │
  ├─ Activity: GetStaleUsers(batchSize=25)
  │    → SELECT * FROM sleeper_users ORDER BY last_fetched_at ASC NULLS FIRST LIMIT 25
  │
  ├─ For each user → spawn UserDiscoveryWorkflow (child, ABANDON, non-blocking)
  │
  ├─ Activity: GetStaleLeagues(batchSize=25)
  │    → SELECT * FROM sleeper_leagues WHERE last_fetched_at IS NULL OR ... LIMIT 25
  │
  └─ For each league → spawn LeagueSyncWorkflow (child, ABANDON, non-blocking)
```

Children run on `sleeper-discovery` and `sleeper-data` queues respectively and manage their own lifecycle. A failed child appears in the Temporal UI for independent inspection and retry.

---

### Workflow: `UserDiscoveryWorkflow`

**Queue:** `sleeper-discovery`
**Trigger:** Spawned by `DiscoveryBatchDispatcher` (also directly by seed CLI)

Handles one user. Idempotent — safe to re-run for the same user.

```
UserDiscoveryWorkflow(userID string)
  │
  ├─ Activity: FetchUserLeagues(userID, seasons=["2022","2023","2024","2025"])
  │    → GET /v1/user/{id}/leagues/nfl/{season} for each season
  │    → Upsert into sleeper_leagues + sleeper_league_users
  │
  ├─ For each newly discovered league:
  │    Activity: FetchLeagueMembers(leagueID)
  │      → GET /v1/league/{id}/users
  │      → Upsert into sleeper_users (last_fetched_at=NULL for new users)
  │
  └─ Activity: MarkUserFetched(userID)
       → UPDATE sleeper_users SET last_fetched_at=now()
```

New users inserted with `last_fetched_at = NULL` are automatically picked up by future dispatcher runs — this is the recursive expansion mechanism without a hard depth limit.

---

### Workflow: `LeagueSyncWorkflow`

**Queue:** `sleeper-data`
**Trigger:** Spawned by `DiscoveryBatchDispatcher`

Handles one league. Idempotent.

```
LeagueSyncWorkflow(leagueID string)
  │
  ├─ Activity: FetchLeagueDetails(leagueID)
  │    → GET /v1/league/{id}
  │    → Upsert scoring_settings, roster_positions, ppr, te_premium, is_superflex into sleeper_leagues
  │
  ├─ Activity: FetchLeagueDrafts(leagueID)
  │    → GET /v1/league/{id}/drafts
  │    → Upsert into sleeper_drafts
  │
  ├─ For each draft (if status="complete" and not already fully synced):
  │    Activity: FetchDraftPicks(draftID)
  │      → GET /v1/draft/{id}/picks
  │      → Upsert into sleeper_draft_picks (ON CONFLICT DO NOTHING)
  │
  ├─ Activity: FetchLeagueTransactions(leagueID, legs=1..18)
  │    → GET /v1/league/{id}/transactions/{round} for rounds 1–18
  │    → Upsert into sleeper_transactions (ON CONFLICT DO NOTHING — immutable once complete)
  │
  └─ Activity: MarkLeagueFetched(leagueID)
       → UPDATE sleeper_leagues SET last_fetched_at=now()
```

---

### Workflow: `PlayerDatabaseSyncWorkflow`

**Queue:** `sleeper-player-sync`
**Trigger:** Temporal Schedule (`sleeper-player-sync-schedule`, daily at 03:00 UTC)

Single activity with heartbeating for the ~5 MB response.

```
PlayerDatabaseSyncWorkflow
  │
  └─ Activity: FetchAndUpsertAllPlayers()
       → GET /v1/players/nfl
       → activity.RecordHeartbeat() during bulk processing
       → Bulk upsert into sleeper_players (ON CONFLICT DO UPDATE)
       → Links espn_id, yahoo_id for cross-referencing
```

---

### Temporal Schedules

| Schedule ID | Workflow | Interval | Batch |
|---|---|---|---|
| `sleeper-discovery-schedule` | `DiscoveryBatchDispatcher` | Every 15 min | 25 users + 25 leagues |
| `sleeper-player-sync-schedule` | `PlayerDatabaseSyncWorkflow` | Daily 03:00 UTC | — |

Schedules are registered by `schedules/register.go` on worker startup using `client.ScheduleClient().Create()` with `TriggerImmediatelyIfMissed: false`.

---

## Seed CLI

`cmd/seed/main.go` — one-shot bootstrap, run once to prime the database.

```
seed --username <sleeper_username>
  1. GET /v1/user/{username} → resolve sleeper_user_id
  2. INSERT INTO sleeper_users (last_fetched_at=NULL) ON CONFLICT DO NOTHING
  3. Start UserDiscoveryWorkflow directly (synchronous, waits for completion)
  4. Exit — scheduled dispatcher takes over from here
```

---

## Rate Limiting & API Budget

Sleeper API limit: **1000 calls/minute**.

Per 15-minute window with batch size 25:
- 25 users × ~5 calls (4 seasons + members per league) ≈ 125 calls
- 25 leagues × ~20 calls (drafts + picks + 18 transaction rounds) ≈ 500 calls
- **Total: ~625 calls / 15 min ≈ 42 calls/minute** — well within limit

The Sleeper HTTP client (`internal/sleeper/client.go`) handles `429 Too Many Requests` with exponential backoff (2×, max 60s). This is tracked separately from Temporal activity retries so a transient rate-limit doesn't consume retry budget.

---

## Error Handling

| Scenario | Handling |
|---|---|
| `404 Not Found` | Non-retryable error; activity returns sentinel; workflow records `skipped_at` on entity |
| `429 Too Many Requests` | Client-side exponential backoff (up to 60s), transparent to Temporal |
| Network timeout | Temporal retry policy: 3 attempts, 5s initial interval, 2× backoff |
| Child workflow failure | Parent is unaffected (ABANDON policy); child visible in Temporal UI for manual retry |
| Duplicate inserts | All writes use `ON CONFLICT DO UPDATE` or `ON CONFLICT DO NOTHING` |

Activity `StartToCloseTimeout`: 5 minutes for most activities; 15 minutes for `FetchAndUpsertAllPlayers`.

---

## Future Phases

### Phase 2: ESPN ETL Migration
- Add `ESPNLeagueSyncWorkflow` in `workers/internal/workflows/espn_league_sync.go`
- New `espn-data` task queue
- `DiscoveryBatchDispatcher` gains an ESPN branch reading from existing `leagues` table
- `scripts/main.py` can be retired once migration is validated

### Phase 3: Valuation Computation
- Add `ValuationComputeWorkflow` on `valuation` task queue
- Weekly schedule
- Activities: `AggregateTradeData` → `ApplyRecencyWeighting` → `ApplyAgingCurves` → `PublishValuations`
- Reads `sleeper_transactions` + `sleeper_players`; writes `player_valuations`
- No new worker binary needed — register alongside existing workflows in `cmd/worker/main.go`

---

## Verification

After implementation:

1. **Unit tests**: Each activity tested with `httptest.NewServer` mock and SQLite in-memory DB
2. **Workflow tests**: `testsuite.WorkflowTestSuite` verifies dispatcher spawns N children, `MarkUserFetched` called on success, 404 sentinel does not propagate as failure
3. **Integration test**: `cmd/seed/main.go` against local `temporal server start-dev` + real Postgres; verify rows appear in `sleeper_users`, `sleeper_leagues`, `sleeper_league_users`
4. **Schedule smoke test**: Let `sleeper-discovery-schedule` fire once; confirm child workflows appear in Temporal UI and `last_fetched_at` timestamps are updated
