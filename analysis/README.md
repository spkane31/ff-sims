# analysis

Bayesian player valuation on real Sleeper data. See
`docs/superpowers/specs/2026-07-02-player-valuation-sleeper-data-design.md`.

```bash
uv sync
uv run pytest                                  # unit tests (no DB needed)
uv run python main.py --demo                   # synthetic data
uv run python main.py --season 2025 --backtest # full replay, rewrites snapshots
uv run python main.py --season 2025            # incremental run (default mode)
```

Reads `DATABASE_URL` from `analysis/.env` (gitignored). Segment v1 is
`ppr-sf-12` (full PPR, superflex, 12-team, redraft) — see `src/config.py`.
Outputs land in `player_valuations` (dated snapshots), `valuation_state`
(beliefs), `valuation_runs` (watermarks).
