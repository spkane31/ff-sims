# analysis

Bayesian player valuation on real Sleeper data. See
`docs/superpowers/specs/2026-07-02-player-valuation-sleeper-data-design.md`.

```bash
uv sync
uv run pytest                                  # unit tests (no DB needed)
uv run python main.py --demo                   # synthetic data

# full replay of a season into the cloud database
uv run python main.py --segment ppr-sf-10 --season 2025 \
    --start 2025-08-25 --step 24h [--end YYYY-MM-DD]

# re-run a staged bundle with no database access at all
uv run python main.py --from-bundle /tmp/ff-sims-player-valuations/<run-id>
```

**This CLI does full replays only.** `--step` is what selects the database
path, so a caller cannot accidentally get incremental behavior out of it;
incremental/checkpoint processing belongs in a future scheduled worker.
`--start` must be the season's draft date (the model clock's origin) — a later
start would seed from ADP and then skip every trade and score before it, while
still rewriting `valuation_state`, so it is rejected rather than silently
publishing a model with a hole in its history.

## Two databases

| Env var                | Used for                                                                 |
| ---------------------- | ------------------------------------------------------------------------ |
| `ARCHIVE_DATABASE_URL` | Model **inputs**: complete Sleeper league/draft/pick/transaction history. |
| `DATABASE_URL`         | Player identities, finalized weekly scoring, run state, and **all output**. |

Both are read from `analysis/.env` (gitignored) or the process environment, and
the job refuses to start if either is missing. Inputs come from the archive
because the cloud database only keeps a hot window; output goes to the cloud so
the API can serve it.

Outputs land in `player_valuations` (dated snapshots), `valuation_state`
(beliefs), `valuation_runs` (watermarks). Segments live in `src/config.py`;
`ppr-sf-10` is the one on a daily timer (see `deploy/worker-host/`).

## Replay semantics

A run consumes events in `[--start, --end)` and writes a snapshot at every
`--step` boundary, including `--end`. A row dated `D` is the model state at the
**start** of UTC day `D`, after all events strictly before that boundary — so
the first snapshot is dated `start + step`, not `start`. Quiet days still get a
snapshot: uncertainty drifts even when nothing happens.

`--step` must be a whole number of days that divides the range (partial-day
batching would put two snapshots on one calendar date). `--end` defaults to the
**current UTC date**, so an unqualified run ends on a snapshot dated today
covering every event through the end of yesterday. That is the newest row it
can publish honestly: defaulting to tomorrow would emit a future-dated row from
an input window that hasn't happened yet, stale until the next run rewrote it.

Each run rewrites `[start, end]` for its segment: the delete and every snapshot
land in one cloud transaction, guarded by a session advisory lock so a manual
replay cannot overlap the timer (a contended lock exits `2` immediately rather
than waiting).

## Staged run artifacts

Every database run materializes its normalized inputs as Parquet before
touching cloud output:

```
/tmp/ff-sims-player-valuations/<run-id>/{adp,trades,weekly_scores,players}.parquet
/tmp/ff-sims-player-valuations/<run-id>/manifest.json
```

`players.parquet` is the resolved identity of every player any input
references, not just the drafted ones — trades carry bare Sleeper IDs, so
without it a bundle replay could not name or position a player who only ever
appears in a trade, and would not reproduce the run it was staged from.

The manifest is written last (so its presence means the bundle is complete) and
records the arguments, UTC boundaries, source counts, schema version, and a
SHA-256 per file. Directories are kept on success **and** failure; each run
prunes ones older than the retention window first, never its own.

| Env var                          | Default                          |
| -------------------------------- | -------------------------------- |
| `FF_SIMS_VALUATION_STAGING_DIR`  | `<system temp>/ff-sims-player-valuations` |
| `FF_SIMS_VALUATION_RETENTION_DAYS` | `14`                           |

Emergency manual cleanup (pruning is automatic; this is for when the disk needs
space *now*):

```bash
rm -rf /tmp/ff-sims-player-valuations/*
```
