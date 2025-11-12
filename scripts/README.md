# ESPN Fantasy Football Data Collection Comparison

## Overview
This document compares the `main.py` script used to collect data from the ESPN API with the `_league.py` dataclass-based implementation.

## What Still Needs to Be Done

### 1. Draft Player Metadata
**Location in main.py**: Lines 344-345

Missing in DraftPick:
- `player_name`
- `player_position`

Currently only stores player_id.

---

### 2. Game Type & Playoff Metadata
**Location in main.py**: Lines 145-146, 181-182, 206-207

Missing in Matchup dataclass:
- `game_type` (matchup_type) - e.g., "NONE", "PLAYOFF"
- `is_playoff` flag

---

### 3. Extended Player Stats in PlayerBoxscore
**Location in main.py**: Lines 215-267

PlayerBoxscore only has 7 fields. Missing 11 fields:
- `pro_opponent` - NFL opponent
- `pro_pos_rank` - Position rank
- `game_played` - Whether player's game has been played
- `game_date` - Date/time of NFL game
- `active_status` - Active/inactive status
- `eligible_slots` - Eligible roster positions
- `on_team_id` - Team ID player is rostered on
- `injured` - Injury flag
- `injury_status` - Injury status text
- `percent_owned` - Ownership percentage
- `percent_started` - Started percentage
- `stats` - Detailed player statistics

---

### 4. Separate Box Score Player Data Structure
**Location in main.py**: Lines 271-316

`main.py` creates a dedicated box score player dataset for analytics:
- Player performance by week
- Owner association
- Status (starter/bench)

This doesn't exist in `_league.py`.

---

### 5. Transaction Metadata
**Location in main.py**: Lines 434-443

Missing in Action/Transaction:
- `player_name`
- `player_position`
- `year` field
- Year validation (transactions only available for 2024+)

---

### 6. Historical Year Handling
**Location in main.py**: Lines 172-191

Missing logic for years < 2019:
- Should use `scoreboard()` instead of `box_scores()` for old years
- ESPN API structure changed in 2019

---

### 7. Current Week Filtering Logic
**Location in main.py**: Lines 170-171, 214, 274
**Location in _league.py**: Line 115

Line 115 check appears incorrect:
```python
# Current:
if espn_league.current_week <= week:

# Should probably be:
if espn_league.current_week > week:
```

This should skip future weeks and only process completed weeks.
