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

### 1. Draft Pick Enhancement
**Location:** `_league.py:143-158`

The current `DraftPick` class is missing:
- `player_name` - Name of drafted player (currently only has player_id)
- `player_position` - Position of drafted player

**Reference:** `main.py:347-349`

---

### 2. Transaction Enhancement
**Location:** `_league.py:194-253`

The current `Action` and `Transaction` classes are missing:
- **Action class:** `player_name` and `player_position` fields
- **Transaction class:** `year` field (for multi-year consistency)
- Date formatting: Currently stored as timestamp (int), should be formatted string
- Year validation: Transactions only available for 2024+

**Reference:** `main.py:417-419, 438-447`

---

### 3. Separate Data Collections

`main.py` generates three separate outputs from schedule data:
1. **Pure Matchups** - Just matchup pairings without scores (`pure_matchups_{year}.json`)
2. **Matchups with Scores** - Full matchup + lineup data (`matchups_{year}.json`)
3. **Box Score Players** - Individual player performances (`box_score_players_{year}.json`)

The current `Schedule` class combines matchups and boxscores but doesn't separate "pure matchups" (matchups without any score data).

**Reference:** `main.py:309, 323-325` (pure matchups), `main.py:279-306, 317-320` (box score players)

---

### 4. Year Filtering Logic

`main.py` has special handling for different years:
- **Years < 2019:** Uses different data retrieval approach (lines 176-195)
- **Years >= 2019:** Uses `box_scores()` function with enhanced data (lines 197-273)
- **Current year filtering:** Only collects box score player data for current year, previous weeks (lines 278-306)

This logic is **not reflected** in `Schedule.from_espn_league()` at `_league.py:83-139`

---

### 5. Current Week Filter Bug
**Location:** `_league.py:115`

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
| Basic Teams | ✅ | ✅ | Complete |
| Basic Matchups | ✅ | ✅ | Complete |
| game_type/is_playoff | ✅ | ✅ | Complete |
| Enhanced player boxscore | ✅ | ✅ | Complete |
| Basic Draft | ✅ | ✅ | Complete |
| Draft player details | ✅ | ❌ | **Missing** |
| Basic Transactions | ✅ | ✅ | Complete |
| Transaction player details | ✅ | ❌ | **Missing** |
| Transaction year field | ✅ | ❌ | **Missing** |
| Transaction year validation | ✅ | ❌ | **Missing** |
| Pure matchups separation | ✅ | ❌ | **Missing** |
| Box score player dataset | ✅ | ❌ | **Missing** |
| Year-specific logic (<2019) | ✅ | ❌ | **Missing** |
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

This will generate a single YAML file containing all league data.

---

## Next Steps

To complete the YAML migration:

1. Add draft player details (player_name, player_position)
2. Add transaction enhancements (player details, year field, year validation)
3. Add year-specific logic to `Schedule.from_espn_league()` (handle pre-2019 data)
4. Fix current week filtering logic bug at line 115
5. Decide on structure for "pure matchups" vs full boxscores
6. Test YAML output matches JSON data completeness
7. Update main.py to use YAML approach

---

## Environment Variables

Required environment variables (stored in `.env`):
- `SWID` - ESPN SWID cookie value
- `ESPN_S2` - ESPN S2 cookie value
- `DATABASE_URL` - PostgreSQL connection string (for active player updates)
