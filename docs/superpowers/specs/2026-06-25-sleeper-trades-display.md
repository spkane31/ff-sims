# Spec: Sleeper Trades Display — Per-Roster Player Names

**Date:** 2026-06-25  
**Status:** Approved

## Problem

The `/sleeper/trades` page shows "Players Added" and "Players Dropped" as identical lists of raw player IDs for every trade. This happens because `adds` and `drops` in Sleeper's data model are both `{player_id: roster_id}` maps — the same players appear in both since a trade simultaneously adds a player to one roster and drops them from another. The display extracted only the JSON keys, making both columns identical. Additionally, player IDs are meaningless to users; names and positions are needed.

## Goal

Replace the broken "Players Added / Players Dropped" columns with "Side A / Side B" columns showing the player names (and positions) each roster received in the trade.

## Data Model

`sleeper_transactions.adds` is JSONB with shape `{player_id: roster_id}`. Grouping the map by its values (roster IDs) produces the two sides of the trade. `sleeper_players` has `full_name` and `position` keyed by `sleeper_player_id`. There is no `sleeper_rosters` table, so sides are identified by their integer roster ID only — team/user name resolution is out of scope.

## Backend Changes (`v2/backend/internal/api/handlers/sleeper.go`)

### New response types

```go
type TradeSidePlayer struct {
    ID       string `json:"id"`
    Name     string `json:"name"`
    Position string `json:"position"`
}

type TradeSide struct {
    RosterID int               `json:"roster_id"`
    Players  []TradeSidePlayer `json:"players"`
}
```

`SleeperTradeItem` replaces `Adds`/`Drops json.RawMessage` with `Sides []TradeSide`.

### Enrichment logic in `GetSleeperTrades`

After scanning the page of trade rows:

1. Decode each row's `adds` JSONB into `map[string]int` (player_id → roster_id).
2. Collect all unique player IDs across all trades on the page into a slice.
3. Batch query: `SELECT sleeper_player_id, full_name, position FROM sleeper_players WHERE sleeper_player_id = ANY(?)`.
4. Build an in-memory lookup map `playerID → {name, position}`.
5. For each trade, group `adds` entries by roster_id value into `[]TradeSide`, sorted by roster_id ascending for stable ordering.
6. Players absent from `sleeper_players` (not yet synced) fall back to `name: player_id, position: ""`.

`drops` is not included in the response — it is redundant for trade display.

Pagination, filtering (`type=trade AND status=complete`), ordering, and `total`/`total_pages` fields are unchanged.

## Frontend Changes

### `src/types/models.ts`

Remove `adds` and `drops` from `SleeperTrade`. Add:

```typescript
export interface TradeSidePlayer {
  id: string;
  name: string;
  position: string;
}

export interface TradeSide {
  roster_id: number;
  players: TradeSidePlayer[];
}

// In SleeperTrade:
sides: TradeSide[];
```

### `src/pages/sleeper/trades.tsx`

- Remove `playerList()` helper.
- Replace "Players Added" and "Players Dropped" `<th>` with "Side A" and "Side B".
- Each side cell renders `trade.sides[n]?.players.map(p => p.position ? \`${p.name} (${p.position})\` : p.name).join(", ")` or `"—"` if absent.
- Column count stays at 5: Date, League, Season, Side A, Side B.
- Side columns keep `max-w-xs truncate` for compact rows.

## Out of Scope

- Resolving roster IDs to team/user display names (requires a `sleeper_rosters` table not yet built).
- Showing `draft_picks` exchanged in trades.
- Waiver and free-agent transaction types.
