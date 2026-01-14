# ETL Player Data

This directory contains player data used by the ETL pipeline.

## players.json

This file contains all NFL player data from the Sleeper API. It's checked into source control to:

1. **Enable offline development**: Don't need API access for every ETL run
2. **Allow manual fixes**: Missing or incorrect player data can be manually corrected
3. **Provide data consistency**: Same dataset across all environments
4. **Track changes**: Git history shows when player data was updated

### File Structure

The file is a JSON map where keys are Sleeper player IDs and values are player objects:

```json
{
  "player_id": {
    "player_id": "1234",
    "espn_id": 4567,
    "yahoo_id": "8910",
    "first_name": "John",
    "last_name": "Doe",
    "position": "RB",
    "team": "SF",
    ...
  }
}
```

### How It Works

1. **First run**: File doesn't exist → Fetches from Sleeper API → Saves to `players.json`
2. **Subsequent runs**: Reads from `players.json` (no API call)
3. **Force refresh**: Use `--refresh-players` flag to update from API

### Usage

**Normal ETL run (uses local file):**
```bash
make etl
# or
./etl upload --data-dir=./data
```

**Force refresh from Sleeper API:**
```bash
./etl upload --refresh-players
```

This will:
- Fetch fresh data from Sleeper API
- Update `players.json`
- Import to database

### Manual Edits

You can manually edit `players.json` to:

- **Fix missing ESPN IDs**: Add `espn_id` field for players
- **Correct positions**: Update `position` field
- **Add missing players**: Add new entries with proper structure
- **Fix player names**: Correct `first_name`, `last_name`, or `name` fields

**Example fix for missing ESPN ID:**
```json
{
  "4430807": {
    "player_id": "4430807",
    "espn_id": 4430807,  // ← Add this
    "first_name": "Bijan",
    "last_name": "Robinson",
    "position": "RB",
    "team": "ATL"
  }
}
```

### When to Refresh

Refresh player data when:
- New NFL season starts
- Rookies are drafted
- Players are traded or change teams
- Missing player data is causing ETL failures

### Maintenance

- **Size**: The file is ~2-5MB (acceptable for source control)
- **Format**: Use 2-space indentation (configured in save function)
- **Backup**: Git history serves as backup
- **Updates**: Refresh monthly during season, or as needed

### Troubleshooting

**Problem**: Player not found during ETL
**Solution**:
1. Check if player exists in `players.json`
2. If missing, run with `--refresh-players`
3. If still missing, manually add entry

**Problem**: ESPN ID mismatch
**Solution**:
1. Look up correct ESPN ID
2. Manually edit `players.json`
3. Re-run ETL

**Problem**: File corrupted or invalid JSON
**Solution**:
1. Delete `players.json`
2. Run with `--refresh-players` to regenerate
