# Player Data Management Implementation

## Overview

Implemented a local JSON cache for player data with support for manual edits and version control.

## Implementation Details

### Files Modified

1. **`internal/etl/new_upload.go`**
   - Added `RefreshPlayerData` option to `NewUploadOptions`
   - Modified `checkPlayerEntries()` to accept `forceRefresh` parameter
   - Added `loadPlayersFromFile()` function
   - Added `savePlayersToFile()` function
   - Smart loading: File → API → Save

2. **`cmd/etl/main.go`**
   - Added `--refresh-players` flag to upload command
   - Passes flag through to ETL options

3. **`internal/etl/data/README.md`**
   - Comprehensive documentation
   - Usage examples
   - Troubleshooting guide

### How It Works

```
┌─────────────────────────────────────────────┐
│  ETL Upload Command                         │
└─────────────────┬───────────────────────────┘
                  │
                  ▼
         ┌────────────────────┐
         │ checkPlayerEntries │
         └────────┬───────────┘
                  │
                  ├─── Check: --refresh-players flag?
                  │
         ┌────────┴─────────┐
         │                  │
    YES  │                  │  NO
         ▼                  ▼
  ┌──────────────┐   ┌─────────────────┐
  │ Fetch from   │   │ Check: File     │
  │ Sleeper API  │   │ exists?         │
  └──────┬───────┘   └────────┬────────┘
         │                    │
         │              ┌─────┴─────┐
         │         YES  │           │  NO
         │              ▼           ▼
         │      ┌────────────┐  ┌─────────┐
         │      │ Load from  │  │ Fetch   │
         │      │ JSON file  │  │ from    │
         │      └─────┬──────┘  │ API     │
         │            │         └────┬────┘
         │            │              │
         ▼            ▼              ▼
      ┌──────────────────────────────┐
      │ Save to players.json         │
      └──────────────┬───────────────┘
                     │
                     ▼
      ┌──────────────────────────────┐
      │ Upsert to Database           │
      └──────────────────────────────┘
```

## Usage

### Normal Operation (Uses Cached File)

```bash
# Make command
make etl

# Direct command
./etl upload --data-dir=../scripts/data
```

**Behavior:**
- Checks if `internal/etl/data/players.json` exists
- If yes → Loads from file (fast, no API call)
- If no → Fetches from API and saves to file

### Force Refresh from API

```bash
./etl upload --refresh-players
```

**Behavior:**
- Fetches fresh data from Sleeper API
- Overwrites `players.json` with latest data
- Updates database with new data

### First-Time Setup

On first run, the file doesn't exist:
1. ETL fetches from Sleeper API
2. Saves to `internal/etl/data/players.json`
3. Imports to database
4. Subsequent runs use the cached file

## Manual Data Fixes

### Common Scenarios

#### Fix Missing ESPN ID

Edit `internal/etl/data/players.json`:

```json
{
  "4430807": {
    "player_id": "4430807",
    "espn_id": 4430807,  // ← Add this field
    "first_name": "Bijan",
    "last_name": "Robinson",
    "position": "RB",
    "team": "ATL",
    ...
  }
}
```

#### Fix Position

```json
{
  "1234": {
    "player_id": "1234",
    "position": "WR",  // ← Change from "RB" to "WR"
    ...
  }
}
```

#### Add Missing Player

If a player is completely missing, add a new entry:

```json
{
  "9999999": {
    "player_id": "9999999",
    "espn_id": 9999999,
    "fantasy_data_id": null,
    "first_name": "New",
    "last_name": "Player",
    "full_name": "New Player",
    "position": "QB",
    "team": "NYG",
    "status": "Active",
    "sport": "nfl"
  }
}
```

### After Manual Edits

1. Save `players.json`
2. Re-run ETL: `make etl`
3. Player data updates in database
4. Commit changes to git

## Benefits

### 1. Offline Development
- No internet required for ETL runs
- Faster execution (no API latency)
- Works in restricted networks

### 2. Data Quality Control
- Fix missing ESPN IDs manually
- Correct position mismatches
- Add missing players
- Standardize player names

### 3. Version Control
- Track changes in git history
- See when data was updated
- Rollback if needed
- Share fixes across team

### 4. Consistency
- Same dataset across all environments
- Reproducible builds
- No API rate limits
- Controlled updates

### 5. Debugging
- Inspect player data easily
- Identify mapping issues
- Test with specific datasets
- Validate transformations

## File Size

- **Typical size**: 2-5 MB
- **Number of players**: ~3,000-4,000
- **Format**: Pretty-printed JSON (2-space indent)
- **Git-friendly**: Text format, line-based diffing

## Maintenance Schedule

### Regular Updates
- **Weekly during season**: For active roster changes
- **Monthly off-season**: For team changes, retirements
- **After draft**: For rookies

### Trigger Events
- ETL failures due to missing players
- New season starts
- Player trades affecting multiple teams
- Data quality issues discovered

## Git Workflow

### Initial Commit
```bash
# First run generates the file
./etl upload --refresh-players

# Add to git
git add internal/etl/data/players.json
git commit -m "Add initial player data from Sleeper API"
```

### Update Player Data
```bash
# Refresh from API
./etl upload --refresh-players

# Review changes
git diff internal/etl/data/players.json

# Commit if updates are significant
git add internal/etl/data/players.json
git commit -m "Update player data - Week 12 roster changes"
```

### Manual Fixes
```bash
# Edit file manually
vim internal/etl/data/players.json

# Test the fix
make etl

# Commit
git add internal/etl/data/players.json
git commit -m "Fix: Add missing ESPN ID for Bijan Robinson"
```

## Troubleshooting

### File Doesn't Exist
**Symptom**: First run, no API call happens
**Solution**: File will be created automatically on first run

### JSON Parse Error
**Symptom**: `Failed to load players from file`
**Solution**:
1. Validate JSON: `jq . internal/etl/data/players.json`
2. If corrupted, delete and run with `--refresh-players`

### Stale Data
**Symptom**: New players not appearing in ETL
**Solution**: Run `./etl upload --refresh-players`

### Large File Size
**Symptom**: File too big for git (unlikely)
**Solution**:
- Git handles 5MB easily
- If needed, use Git LFS
- Or, exclude from git and document manual refresh process

## Future Enhancements

1. **Incremental Updates**: Only fetch changed players
2. **Multiple Sources**: Merge ESPN + Sleeper data
3. **Validation**: Schema validation on load
4. **Stats History**: Include career stats in cache
5. **Auto-refresh**: Scheduled updates via cron

## Testing

### Verify File Creation
```bash
# Delete existing file
rm internal/etl/data/players.json

# Run ETL
make etl

# Verify file exists
ls -lh internal/etl/data/players.json
```

### Verify Manual Edits
```bash
# Edit a player's ESPN ID
jq '.["4430807"].espn_id = 12345' internal/etl/data/players.json > tmp.json
mv tmp.json internal/etl/data/players.json

# Run ETL
make etl

# Check database
psql -d fantasy_football -c "SELECT * FROM players WHERE sleeper_id = '4430807';"
```

### Verify Force Refresh
```bash
# Note current file timestamp
stat internal/etl/data/players.json

# Force refresh
./etl upload --refresh-players

# Verify timestamp changed
stat internal/etl/data/players.json
```

## Integration with Existing ETL

The player data system integrates seamlessly:

1. **checkPlayerEntries()** called at start of ETL
2. Loads/refreshes player data
3. Upserts to database
4. Draft selection processing uses cached players
5. Box score processing uses cached players
6. Transaction processing uses cached players

No changes needed to downstream processing!
