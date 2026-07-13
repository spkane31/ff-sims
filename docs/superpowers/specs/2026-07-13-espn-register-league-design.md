# Spec: ESPN `register-league` CLI

**Date:** 2026-07-13
**Status:** Draft

## Context

Onboarding or re-authenticating an ESPN league today means hand-writing two separate SQL
statements: one `INSERT ... ON CONFLICT` into `espn_league_credentials` (espn_s2/swid) and a
second, easy-to-forget `INSERT` into `leagues` (platform='ESPN', external_id). The Temporal
activities that actually sync ESPN data (`resolve_league_id` in `workers/espn/db.py`) only ever
look at `leagues` — they never touch `espn_league_credentials` directly — so a league with
credentials but no `leagues` row fails every sync activity with `ValueError: No ESPN league found
with external_id=...`, retried forever (until the retry-policy fix landed) with no indication of
what's actually wrong.

This happened in production: `espn_league_credentials` was updated (cookie rotation) while the
corresponding `leagues` row didn't exist, and the two ESPN leagues (345674, 1094568961) silently
failed to sync. This spec adds a single CLI command that writes both rows together, atomically,
and validates the credentials against the live ESPN API before writing anything — so a bad
league ID or expired cookie is caught immediately instead of surfacing later as a Temporal retry
loop.

While setting up a worktree to build this, a second, related gap surfaced: `workers/espn/db.py`'s
`get_connection()` reads `os.environ["DATABASE_URL"]` unconditionally, ignoring
`TEST_DATABASE_URL` — unlike `tests/conftest.py`'s `db_conn` fixture, which prefers
`TEST_DATABASE_URL`. That mismatch is *how* the original incident's blast radius extended to
`pytest` itself: running the test suite (with only `DATABASE_URL` set, no `TEST_DATABASE_URL`)
wrote real "Test"/"Test League" rows into the production `leagues` table, because every activity
under test calls `get_connection()` internally. Fixing `db.py` to match the fixture's precedence
is a prerequisite for testing `register_league.py` (or anything else in this package) safely, so
it's included here rather than as a separate follow-up.

## Non-goals

- No support for other platforms (Sleeper already has automated discovery via `cmd/seed` +
  `DiscoveryBatchDispatcher` — it doesn't need manual registration).
- No interactive/prompted input — flags only, so it's scriptable.
- No bulk/batch registration (one league per invocation) — YAGNI until there's a real multi-league
  onboarding need.
- No new CLI framework dependency — stdlib `argparse` is enough for one command with four flags.
- No change to the weekly `ESPNSyncDispatcher` schedule or its league-discovery query
  (`get_espn_leagues`, `activities/credentials.py`) — this tool only writes rows and, once written,
  kicks a single explicit sync; it doesn't change how the recurring dispatcher enumerates leagues.

## Design

### Prerequisite fix: `workers/espn/db.py`

```python
def get_connection() -> psycopg.Connection:
    # TEST_DATABASE_URL takes precedence so tests can never fall through to
    # production even if only DATABASE_URL is set in the environment —
    # matches tests/conftest.py's db_conn fixture precedence.
    url = os.environ.get("TEST_DATABASE_URL", os.environ["DATABASE_URL"])
    return psycopg.connect(url)
```

No other changes to `db.py`. This alone makes `TEST_DATABASE_URL` an effective, whole-package
safety net — both the test fixture's own connection and every activity/CLI function's connection
now agree on which database "test mode" means.

### New module: `workers/espn/temporal_client.py`

Extracted from `worker.py` unchanged (`create_client()` and `_fetch_server_tls_config()`), so both
`worker.py` and `register_league.py` share one Temporal-connection implementation instead of
duplicating the openssl-based TLS cert-chain fetch. `worker.py` imports `create_client` from this
new module instead of defining it inline; its behavior (env vars, TLS handling, local-dev
fallback) is unchanged.

### New module: `workers/espn/register_league.py`

```
uv run register-league --league-id 345674 --espn-s2 '...' --swid '{...}' [--year 2026] [--no-sync]
```

Exposed as a `uv run register-league` entry point via a new `[project.scripts]` table in
`pyproject.toml` (`register-league = "register_league:main"`).

**Flags:**
- `--league-id` (required, str) — ESPN league ID.
- `--espn-s2` (required, str) — ESPN_S2 cookie value.
- `--swid` (required, str) — SWID cookie value.
- `--year` (optional, int, default: `datetime.date.today().year`) — season to validate against and
  to sync.
- `--no-sync` (optional flag) — write the DB rows but skip starting the Temporal workflow.

**Flow (`main()`):**

1. Parse args.
2. Validate against the live ESPN API first, before any DB access:
   ```python
   league = League(league_id=int(args.league_id), year=args.year,
                    espn_s2=args.espn_s2, swid=args.swid)
   ```
   Any exception here (bad league ID, expired/invalid cookies, network error) is caught, printed
   as a clear one-line error (`f"Could not reach league {args.league_id}: {exc}"`), and the
   program exits non-zero without touching the database at all.
3. Pull the real league name: `name = league.settings.name`.
4. Open one `get_connection()` and, in a single transaction, upsert both rows:
   ```sql
   INSERT INTO leagues (name, platform, external_id, created_at, updated_at)
   VALUES (%s, 'ESPN', %s, NOW(), NOW())
   ON CONFLICT (platform, external_id) WHERE platform != '' AND external_id != ''
   DO UPDATE SET name = EXCLUDED.name, updated_at = NOW()
   RETURNING id, (xmax = 0) AS inserted;

   INSERT INTO espn_league_credentials (espn_league_id, espn_s2, swid)
   VALUES (%s, %s, %s)
   ON CONFLICT (espn_league_id) DO UPDATE
       SET espn_s2 = EXCLUDED.espn_s2, swid = EXCLUDED.swid, updated_at = NOW();
   ```
   Both statements run before a single `conn.commit()` — they either both land or neither does,
   which is the direct fix for today's split-state failure. (`xmax = 0` is the standard Postgres
   idiom for "this row was inserted, not updated by the ON CONFLICT clause" — used only to decide
   the wording of the confirmation message in step 6, not for control flow.)
5. On any DB error, print it and exit non-zero. No Temporal call is attempted.
6. Print a confirmation: league name, internal `leagues.id`, and whether it was newly created or
   updated.
7. Unless `--no-sync`, connect via `temporal_client.create_client()` and start:
   ```python
   await client.start_workflow(
       LeagueESPNSyncWorkflow.run,
       LeagueDispatchParams(espn_league_id=args.league_id, year=args.year),
       id=f"espn-league-{args.league_id}-{args.year}",
       task_queue="espn-sync",
   )
   ```
   using the same workflow ID convention `league_sync.py`'s own child-workflow starts use, so this
   is idempotent with (and dedupes against) anything the weekly dispatcher might concurrently
   start. If starting the workflow fails (e.g. one's already running for this league/year), catch
   the exception, print it as a warning (not an error), and exit 0 — the DB rows are already
   correctly written at that point, matching `league_sync.py`'s own ABANDON-and-warn precedent for
   child workflow starts.
8. Print the workflow ID so the user can look it up in the Temporal UI.

`main()` is synchronous except for the Temporal portion, which runs via `asyncio.run(...)` only
when reached (steps 1–6 are plain sync code, matching every other activity in this package).

### Error handling summary

| Failure point | Behavior |
|---|---|
| ESPN API call (bad ID/cookie/network) | Print error, exit non-zero, no DB writes |
| DB upsert | Print error, exit non-zero, no Temporal call |
| Temporal workflow start | Print warning, exit 0 (DB rows already correct) |

### Testing

New `workers/espn/tests/test_register_league.py`, using the existing `db_conn` fixture (now safe
by construction once the `db.py` fix lands, since both the fixture and the code under test agree
on `TEST_DATABASE_URL`):

- Registering a brand-new league: mock `League`, call the CLI's main function, assert both a
  `leagues` row and an `espn_league_credentials` row exist with the expected values.
- Re-registering an existing league (cookie rotation): seed both rows first, run again with a
  different `espn_s2`, assert both rows are updated in place — no duplicate `leagues` row (unique
  index `idx_leagues_platform_external_id` enforces this; the test asserts the row count stays 1).
- Bad credentials: mock `League` to raise, assert neither table gains a row.
- Temporal start is mocked/patched in all of the above (via `--no-sync` or a patched
  `temporal_client.create_client`) — these are DB-behavior tests, not Temporal integration tests.

No changes to CI (`ci.yml` doesn't run the Python worker's test suite at all today — out of scope
for this spec).
