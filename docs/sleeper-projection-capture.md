# Sleeper projection capture

The worker captures Sleeper's weekly player projections (`/projections/nfl/{season}/{week}`) with the UTC time each pull was taken, so replay can use only forecasts that existed before kickoff. Captures run on the `sleeper-projections` task queue and write to the main database.

## What runs

| Schedule | Workflow | When |
| --- | --- | --- |
| `sleeper-projections-daily-sweep-schedule` | `ProjectionDailySweep` | 10:00 UTC daily |
| `sleeper-projections-pregame-dispatch-schedule` | `ProjectionPregameDispatcher` | :05 every hour |

`ProjectionDailySweep` captures the current week and every later week through Week 18. It skips a week once its final verified kickoff has passed (Sleeper keeps pointing at the old week until Wednesday), reports verified kickoff windows in the current and previous week that got no capture within two hours before kickoff, logs the age of the last capture and prunes old captures. A failed week does not stop the sweep, but the workflow fails at the end and its error details carry the full report.

`ProjectionPregameDispatcher` starts one `ProjectionPregameCapture` per distinct verified kickoff time in the next 36 hours, with workflow ID `sleeper-projections-pregame-{season}-w{week}-{kickoff unix}`. Each run sleeps until 60 minutes before kickoff, checks the window still exists in `nfl_kickoffs`, and captures that week. When a kickoff moves, the next dispatch starts a run for the new time and the old run finds its window gone and exits.

## Loading confirmed kickoffs

Until validated schedule revisions (#230) land, an operator loads kickoff times checked against the [official NFL schedule](https://www.nfl.com/schedules/). Write one game per line, with the kickoff in RFC 3339 including the offset:

```csv
season,week,away,home,kickoff,status
2026,13,GB,DET,2026-12-03T20:15:00-05:00,confirmed
2026,13,DAL,SEA,2026-12-06T16:25:00-05:00,confirmed
```

```bash
cd backend
DATABASE_URL=... go run ./cmd/kickoffs -file week13.csv
```

Each season/week in the file replaces everything stored for it, so fix a moved game by editing the file and loading it again. Team codes must match Sleeper's (`WAS`, `LAR`, `JAX`). Use `provisional` for times not yet confirmed: they are recorded as cutoffs but never make a capture replay-eligible or schedule a pregame run.

## Replay eligibility

Each capture records a `week_phase` and the kickoff cutoff for every game ID in the response (`sleeper_projection_capture_games`):

- `pregame`: every team with a game has a verified kickoff after capture time.
- `in_progress`: some verified kickoffs had passed. Only games whose kickoff follows capture time are replay-eligible.
- `future`: a later week without full verified kickoffs. `replay_eligible` stays null.
- `unverified`: the current or a past week without full verified kickoffs. Not replay-eligible; `replay_note` names the teams missing a verified kickoff.

`models.ReplayProjections(db, season, week, company, asOf)` returns one provider's rows from the latest eligible capture before `asOf`, limited to eligible games. The company is required.

A player missing from a capture has no row; a present row with no projection (only `adp_*` fields) is stored with `row_kind = 'metadata_only'`; an explicit projected 0 stays 0 rather than null.

Current-week captures also write `sleeper_player_status_observations` (NFL team, `injury_status`, `injury_start_date`, `news_updated`) under the same capture ID and observation time, for #231.

## Manual capture

Manual runs go through Temporal, not the site's HTTP routes, and use the same cutoffs. They record a present-time observation:

```bash
temporal workflow start \
  --task-queue sleeper-projections \
  --type ProjectionManualCapture \
  --input '{"Season":"2026","Week":13}'
```

Omit the input fields to capture Sleeper's current week.

## Configuration

| Env var | Default | Meaning |
| --- | --- | --- |
| `SLEEPER_PROJECTION_COMPANIES` | `rotowire` | Comma-separated providers expected in every response. A missing one is logged and returned in the capture result. |
| `PROJECTION_RETENTION_DAYS` | `90` | Age after which captures that are not replay-eligible are pruned. Replay-eligible captures are kept. `0` disables pruning. |
