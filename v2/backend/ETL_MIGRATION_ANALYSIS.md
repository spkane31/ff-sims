# ETL Pipeline Migration Analysis: JSON to YAML

## Overview

This document compares the current JSON-based ETL pipeline (`upload.go`) with the new YAML-based approach (`new_upload.go`), identifying missing features and parity concerns.

## Architecture Comparison

### JSON-Based Approach (upload.go)
- **File Format**: Multiple JSON files per league/year (e.g., `teams_2024.json`, `matchups_2024.json`)
- **Processing**: File-type-based routing with regex pattern matching
- **League Scope**: Single hardcoded league (ID: 345674)
- **Database Operations**: Manual Create/Update with GORM ErrRecordNotFound checks

### YAML-Based Approach (new_upload.go)
- **File Format**: Single YAML file per league/year (e.g., `2025.yaml`)
- **Processing**: Structured ETLLeague model with embedded data
- **League Scope**: Multi-league support via `MultipleLeagues` option
- **Database Operations**: Modern upsert with `clause.OnConflict`

## Feature Parity Analysis

### ✅ Implemented in YAML Approach

| Feature | JSON Implementation | YAML Implementation | Status |
|---------|-------------------|-------------------|--------|
| League Creation | `upload.go:722-744` | `new_upload.go:153-160` | ✅ Complete |
| Team Creation | `upload.go:510-560` | `new_upload.go:162-182` | ✅ Complete |
| Basic Matchup Creation | `upload.go:204-341` | `new_upload.go:184-206` | ✅ Complete |
| Matchup Score Updates | `upload.go:304-318` | `new_upload.go:208-239` | ✅ Complete |
| Draft Selection Processing | `upload.go:40-124` | `new_upload.go:241-321` | ✅ Complete |
| Player Data Refresh | N/A | `new_upload.go:64-125` | ✅ New Feature |
| Multi-league Support | ❌ Not supported | `new_upload.go:19` | ✅ New Feature |

### ❌ Missing in YAML Approach

| Feature | JSON Implementation | YAML Status | Priority |
|---------|-------------------|-------------|----------|
| **Box Score Processing** | `upload.go:343-500` | ❌ TODO (Line 323) | 🔴 CRITICAL |
| **Transaction Processing** | `upload.go:575-696` | ❌ TODO (Line 325) | 🔴 CRITICAL |
| **Expected Wins Calculation** | `upload.go:816-892` | ❌ TODO (Line 324) | 🟡 HIGH |
| **Simulation Execution** | `upload.go:829-892` | ❌ TODO (Line 326) | 🟡 HIGH |
| **Pure Matchups** | `matchups.go:25-102` | ❌ Missing | 🟢 LOW |

## Detailed Feature Gaps

### 1. Box Score Processing (CRITICAL)

**JSON Implementation:**
- Processes player lineups for both home/away teams (`processPlayerLineUp`)
- Creates/updates Player records with position inference
- Detailed stat breakdown (passing, rushing, receiving, kicking)
- Creates BoxScore records with:
  - `ActualPoints` and `ProjectedPoints`
  - `SlotPosition`
  - Full `GameStats` (13 stat categories)
- Handles bye weeks and missing stats
- Links players to matchups and teams

**YAML Gap:**
- Box scores defined in ETL models (`models.go:74-84`)
- Processing logic completely missing
- No player lineup processing
- No stats breakdown implementation
- TODO comment at line 208

**Impact:** Without box scores, you cannot:
- Track individual player performance
- Calculate team scores from player contributions
- Generate player statistics reports
- Support fantasy analysis features

### 2. Draft Selection Processing ✅ IMPLEMENTED

**JSON Implementation:**
- Reads draft picks with round/pick numbers
- Creates players if they don't exist
- Links draft picks to teams via ESPN ID
- Handles keeper league scenarios implicitly
- Updates existing draft selections

**YAML Implementation:**
- ✅ Draft structure defined (`models.go:106-121`)
- ✅ Includes keeper flag (improvement over JSON version!)
- ✅ Processing logic implemented (`new_upload.go:241-321`)
- ✅ Creates players if they don't exist
- ✅ Maps ESPN team IDs to internal IDs
- ✅ Uses upsert for idempotent processing
- ✅ Handles all draft picks with detailed logging

**Status:** Feature complete! Draft selections are now properly imported and linked to teams and players.

### 3. Transaction Processing (CRITICAL - Next Priority)

**JSON Implementation:**
- Processes adds, drops, trades, free agent pickups
- Handles bid amounts (FAAB/waiver budget)
- Parses dates with custom format
- Creates players if they don't exist
- Links transactions to teams and players

**YAML Gap:**
- Transaction structure defined (`models.go:131-146`)
- Uses actions-based model (more flexible than JSON)
- Supports transaction types: ADD, DROP, TRADE, FREE_AGENT_ADD
- Custom `YAMLTime` type for date parsing
- No processing logic implemented

**Impact:** Without transaction processing, you cannot:
- Track roster changes over time
- Analyze waiver wire activity
- Support trade review features
- Calculate transaction-based metrics

### 4. Matchup Score Processing ✅ IMPLEMENTED

**JSON Implementation:**
- Stores `HomeTeamFinalScore` and `AwayTeamFinalScore`
- Tracks `HomeTeamESPNProjectedScore` and `AwayTeamESPNProjectedScore`
- Validates completion status (both scores must be > 0)
- Updates existing matchups with new scores

**YAML Implementation:**
- ✅ Creates matchups from `schedule.matchups` section
- ✅ Updates scores from `schedule.boxscores` section (`new_upload.go:208-239`)
- ✅ Processes `HomeScore`, `AwayScore`, `HomeProjectedScore`, `AwayProjectedScore`
- ✅ Sets `Completed` status from boxscore data
- ✅ Logs detailed update information

**Status:** Feature complete! Matchup scores are now properly updated from boxscore data.

### 5. Expected Wins Calculation (HIGH)

**JSON Implementation:**
- Automatic calculation after ETL (`processExpectedWinsAfterETL`)
- Processes weekly expected wins for completed weeks
- Finalizes season expected wins when regular season completes
- Supports recalculation for specific years
- Checks if weeks are already processed to avoid duplicates

**YAML Gap:**
- No expected wins logic
- No simulation integration
- TODO at line 209

**Impact:** Without expected wins:
- No advanced team performance metrics
- No "luck" analysis (actual wins vs. expected)
- Reduced analytical value of the platform

### 6. Player Position Inference (MEDIUM)

**JSON Implementation:**
- Infers position from `EligibleSlots` when not available
- Maps flex positions (FLEX, BE, IR) to actual positions
- Updates player records with inferred positions
- Handles D/ST and kicker positions

**YAML Gap:**
- Player position included in box score data
- No fallback logic for missing positions
- Relies on external player data (Sleeper API)

**Impact:**
- Less robust for historical data
- Requires external API for complete player data
- May fail for players not in Sleeper database

### 7. Pure Matchups Support (LOW)

**JSON Implementation:**
- Separate file type for schedule-only matchups
- Used for pre-season schedule uploads
- No scores or box scores required

**YAML Gap:**
- Basic matchup creation exists
- May already satisfy this use case

**Impact:** Minor - likely satisfied by existing YAML matchup creation

## Data Model Improvements in YAML

### Advantages

1. **Structured Hierarchy**
   - Single `ETLLeague` contains all related data
   - Clear relationships between entities
   - Better data validation potential

2. **Transaction Actions Model**
   - More flexible than JSON's flat transaction structure
   - Supports complex multi-player trades
   - Explicit action types (ADD, DROP, TRADE)

3. **Keeper League Support**
   - Draft picks include `keeper` flag
   - Native support for keeper leagues

4. **Player Data Management**
   - Automatic player database updates from Sleeper API
   - Local JSON cache for offline development (`internal/etl/data/players.json`)
   - Manual editing support for fixing missing/incorrect data
   - Force refresh option via `--refresh-players` flag
   - Reduces dependency on external APIs
   - Version controlled player data for consistency

5. **Multi-League Support**
   - Can process multiple leagues in parallel
   - No hardcoded league IDs
   - Better for scaling

6. **Modern Database Operations**
   - Uses `clause.OnConflict` for upserts
   - More efficient than manual check-and-update
   - Better handling of race conditions

### Considerations

1. **File Size**
   - Single YAML file contains all season data
   - Could be large for leagues with extensive history
   - JSON approach distributes data across files

2. **Partial Updates**
   - JSON allows updating specific data types (e.g., only matchups)
   - YAML requires full league file
   - May need incremental update strategy

## Migration Roadmap

### Phase 1: Core Functionality (CRITICAL)
- [ ] Implement box score processing
- [ ] Add player lineup and stats processing
- [ ] Process matchup scores and completion status
- [ ] Implement draft selection processing

### Phase 2: Transactions & History (CRITICAL)
- [ ] Implement transaction processing
- [ ] Handle transaction actions (ADD/DROP/TRADE)
- [ ] Process bid amounts and dates
- [ ] Link transactions to teams and players

### Phase 3: Analytics (HIGH)
- [ ] Integrate expected wins calculation
- [ ] Add weekly expected wins processing
- [ ] Implement season finalization logic
- [ ] Support recalculation for specific years

### Phase 4: Simulations (HIGH)
- [ ] Integrate simulation engine
- [ ] Run playoff simulations
- [ ] Generate simulation results
- [ ] Store simulation outputs

### Phase 5: Testing & Validation (HIGH)
- [ ] Create comprehensive test suite
- [ ] Validate data parity with JSON approach
- [ ] Test multi-league support
- [ ] Performance benchmarking

### Phase 6: Migration Support (MEDIUM)
- [ ] Create JSON-to-YAML conversion tool
- [ ] Support gradual migration
- [ ] Maintain backward compatibility
- [ ] Documentation for data format

## Testing Strategy

### Data Validation
1. Process same season with both JSON and YAML
2. Compare database state after each ETL run
3. Validate:
   - Team records
   - Player records
   - Matchup completeness
   - Box score accuracy
   - Draft selection correctness
   - Transaction history
   - Expected wins calculations

### Edge Cases to Test
- Players not in Sleeper database
- Bye week handling
- Incomplete matchups
- Mid-season trades
- Keeper league scenarios
- Multiple leagues with overlapping teams
- Historical data migration

## Recommendations

### Immediate Actions
1. **Implement Box Score Processing** - Highest priority, blocks most features
2. **Add Score Updates to Matchups** - Required for expected wins
3. **Implement Draft Processing** - Critical for league history

### Short-term Actions
4. **Add Transaction Processing** - Important for completeness
5. **Integrate Expected Wins** - Core analytical feature
6. **Add Test Suite** - Ensure data quality

### Long-term Actions
7. **Create Migration Tool** - Support transition from JSON
8. **Performance Optimization** - Test with large datasets
9. **Documentation** - YAML schema and examples

## Risks & Mitigation

| Risk | Impact | Likelihood | Mitigation |
|------|--------|-----------|-----------|
| Data loss during migration | HIGH | MEDIUM | Maintain JSON pipeline until YAML is validated |
| Performance issues with large YAML files | MEDIUM | MEDIUM | Implement streaming parser, file splitting strategy |
| Incomplete player data from Sleeper | MEDIUM | LOW | Fallback to position inference logic |
| Breaking changes to existing features | HIGH | MEDIUM | Comprehensive testing, gradual rollout |
| Multi-league conflicts | MEDIUM | LOW | Add league ID validation, isolation testing |

## Conclusion

The YAML-based approach provides significant architectural improvements:
- Better data structure and organization
- Multi-league support
- Modern database operations
- Automatic player data management

However, it is currently **~50% complete** with critical gaps in:
1. Box score processing (blocks analytics)
2. Transactions (blocks roster tracking)
3. Expected wins (blocks key feature)

**Recommendation:** Treat this as a ground-up rewrite rather than a simple format migration. Prioritize implementing core data processing features before deprecating the JSON approach.

**Estimated Effort:**
- Phase 1 (Core): 2-3 weeks
- Phase 2 (Transactions): 1 week
- Phase 3 (Analytics): 1-2 weeks
- Phase 4 (Simulations): 1 week
- Phase 5 (Testing): 1-2 weeks
- **Total: 6-9 weeks** for production-ready YAML pipeline

**Next Steps:**
1. Create feature branch for YAML development
2. Implement box score processing with test data
3. Add integration tests comparing JSON vs YAML outputs
4. Validate with single league before multi-league rollout
