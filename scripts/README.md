# Fantasy Football Data Scripts

## Overview

This directory contains scripts for extracting fantasy football data from ESPN and converting it to various formats for storage and analysis.

## YAML Migration Status

We are in the process of migrating from writing data to multiple JSON files (in `main.py`) to writing a single consolidated YAML file (using the `League` model in `src/models/_league.py`).

### Current State

- **JSON Approach** (`main.py`): Fully functional, writes 5 separate JSON files per league/year
- **YAML Approach** (`_league.py`): Partially implemented, missing several data fields

---

## Missing Features for YAML Migration

The following features are currently implemented in `main.py` but **NOT YET** in the YAML-based `League` model:

### 1. Current Week Filter Bug
**Location:** `_league.py:223`

Potential logic error in current week filtering:
```python
# Current code:
if espn_league.current_week <= week:

# Should probably be:
if espn_league.current_week > week:
```

This should skip future weeks and only process completed weeks.

**Reference:** `main.py:174, 218, 278`

---

## Feature Comparison Table

| Feature | main.py | _league.py | Status |
|---------|---------|------------|--------|
| Current week filtering | ✅ | ⚠️ | **Bug** |

---

## Usage

### Current JSON Approach

```bash
python main.py --year 2025 --league-id 345674 --output-dir data
```

This generates the following files in `data/{league_id}/`:
- `teams_{year}.json`
- `matchups_{year}.json`
- `pure_matchups_{year}.json`
- `box_score_players_{year}.json`
- `draft_selections_{year}.json`
- `transactions_{year}.json`

### Future YAML Approach (In Progress)

```python
from espn_api.football import League as ESPNLeague
from src.models import League as DataLeague

espn_league = ESPNLeague(league_id=345674, year=2025, swid=SWID, espn_s2=ESPN_S2)
league = DataLeague.from_espn_league(espn_league)
league.to_yaml(f"data/{league.id}_{league.year}.yaml")
```

This will generate a single YAML file containing all league data, including:
- **schedule.matchups**: Basic matchup pairings (equivalent to `pure_matchups_{year}.json`)
- **schedule.boxscores**: Full boxscore data with player lineups (equivalent to `matchups_{year}.json`)
- **schedule.box_score_players**: Individual player performance data (equivalent to `box_score_players_{year}.json`)

---

## Next Steps

To complete the YAML migration:

1. Fix current week filtering logic bug at line 223
2. Test YAML output matches JSON data completeness
3. Update main.py to use YAML approach

---

## Environment Variables

Required environment variables (stored in `.env`):
- `SWID` - ESPN SWID cookie value
- `ESPN_S2` - ESPN S2 cookie value
- `DATABASE_URL` - PostgreSQL connection string (for active player updates)
