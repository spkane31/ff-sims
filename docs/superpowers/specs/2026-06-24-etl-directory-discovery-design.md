---
title: ETL Directory-Driven Discovery
date: 2026-06-24
issue: "#73"
status: approved
---

## Problem

The ETL `upload` command currently requires `--league-id` and implies a flat `--data-dir` containing all files for a single league and year. As the number of leagues and years grows, files collide and the operator must manually organize them and invoke the ETL once per league/year. This issue gives the ETL ownership of the directory structure and removes the need to specify league and year as flags.

## Data Directory Contract

Data files are pre-organized by the scraper into a two-level hierarchy:

```
{data-dir}/
  {leagueExternalID}/      ← directory name is the platform-assigned external ID
    {year}/                ← directory name is a 4-digit year
      matchups_{year}.json
      teams_{year}.json
      transactions_{year}.json
      ...
```

The ETL reads this layout; it does not write or enforce it. Any non-directory entry at the league or year level is silently skipped.

## CLI Changes (`cmd/etl/main.go`)

### Flag changes

| Flag | Subcommand | Was | Now |
|---|---|---|---|
| `--data-dir` | global | optional (default `./data`) | unchanged |
| `--platform` | global | optional (default `ESPN`) | unchanged |
| `--league-id` | global | required | optional filter |
| `--year` | upload | not present | optional filter (`uint`, 0 = all) |

The `--skip-expected-wins` flag on `upload` is unchanged.

### Walk algorithm

`uploadCmd.RunE` replaces the single-league call with a directory walker:

1. Read `dataDir`; skip non-directory entries.
2. If `--league-id` is set, skip any league directory whose name does not match it exactly.
3. For each league directory, read its subdirectories; skip non-directory entries.
4. If `--year` is set, skip non-matching year directories. If a directory name cannot be parsed as a `uint`, log a warning and skip it.
5. For each `(leagueExternalID, parsedYear)` pair:
   - Call `resolveLeagueID(leagueExternalID, platform)` — auto-creates the league record if it does not exist (existing behavior).
   - Log: `Processing league {externalID} (internal ID {id}), year {year} from {path}`
   - Call `etl.UploadWithOptions(scopedPath, leagueID, !skipExpectedWins)`
6. Return the first error encountered, or `nil` if all pairs succeed.

`--league-id` no longer triggers an early-return error when unset.

## ETL Package Changes (`internal/etl/upload.go`)

One guard is added to `UploadWithOptions` immediately after `os.ReadDir` succeeds: count files matching the filename regex (`^(.+)_\d{4}\.json$`). If the count is zero, log a warning and return `nil`:

```go
logging.Warnf("No processable files found in %s, skipping", directory)
return nil
```

This prevents silent no-ops when a year directory exists but is empty or contains only unrecognized files. No other changes to the etl package.

## Acceptance Criteria

- `etl upload --data-dir ./data` processes every `{leagueID}/{year}/` subtree found under `./data`
- `etl upload --data-dir ./data --league-id 345674` processes only the `345674/` subtree
- `etl upload --data-dir ./data --year 2024` processes only `2024/` subdirectories across all leagues
- Both filters can be combined
- A league directory whose name cannot be found in the database is auto-created
- A year directory whose name is not a valid uint is skipped with a logged warning
- An empty year directory logs a warning and exits cleanly (no silent no-op, no error)
- `etl xwins` behavior is unchanged
