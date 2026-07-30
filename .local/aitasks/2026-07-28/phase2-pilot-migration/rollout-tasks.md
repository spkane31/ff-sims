# Full design-system rollout — task breakdown

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Finish what the pilot started — every remaining production page and the three shared sub-components it left untouched get migrated onto the design system, so the app is visually one coherent system instead of a new shell wrapped around old content.

**Why now, not later:** the pilot (global shell + League-overview-only content) was implemented and looked fine in isolation, but live in a browser it's a real clash — bright `bg-white`/pastel-gradient old-Tailwind cards sitting inside an otherwise dark, neutral, editorial-toned shell. Confirmed visually on the homepage (stark white league cards on near-black canvas) and on League overview itself (Hall of Fame's yellow gradient card and Wall of Shame's red gradient card, plus the Head-to-Head grid's light pastel win/loss cells, all sitting directly below the already-migrated dark "This season" band). See `plan.md`'s "Scope change: full rollout" section for the full decision record.

## Global Constraints

- No backend/API changes anywhere. Every hook/service call stays exactly as it is today.
- **No product-behavior change, on any page.** Every computed value, filter, sort comparator, tab/toggle state, loading condition, and error condition must produce identical output and identical interactive behavior to what exists today. Only the JSX markup and class names change. If you are not sure a piece of logic is purely presentational, treat it as business logic and preserve it verbatim.
- No new routes. No new information/metrics beyond what a given page shows today.
- Component library stays incremental: use the primitives already installed (`Button`, `Badge`, `Card`/`CardContent`, `Skeleton`, `EmptyState`, `ErrorState`, `StatCard`, `DataTable`, `Select`, `DropdownMenu` — all under `frontend/src/components/ui/` or `frontend/src/components/design-system/`). If a page genuinely needs a primitive that doesn't exist yet (Tabs, Dialog, etc.), install it via `npx shadcn@latest add <name>` as part of that task — don't hand-roll a substitute, and don't install it speculatively in a task that doesn't need it.
- No test framework introduction (same rationale as the pilot: no test runner exists in this repo, this is refactor-flavored work). Verification per task is `npm run build` + `npm run lint`, both clean, plus the manual browser check described below.
- Tokens are the only source of visual values in touched files — no raw hex/Tailwind-palette color classes (`text-blue-600`, `bg-gray-700`, `bg-red-100`, etc.) left in any file this rollout touches. Spacing/sizing utilities (padding, gaps, text size) can stay as Tailwind classes; only *color* values must route through tokens.
- Git workflow: commit at the end of each task as usual — the controller squashes everything to unstaged working-tree changes at the very end, per the same process used for the pilot.

## Migration recipe (apply this to every task below)

This is the standard transformation. Each task's own section only calls out what's specific to that file — assume this recipe applies everywhere unless a task says otherwise.

| Old pattern | New pattern |
|---|---|
| `bg-white dark:bg-gray-700/800 ... rounded-lg shadow-md` card container | `<Card><CardContent className="p-6">...</CardContent></Card>` (`@/components/ui/card`) |
| `text-blue-600` / `hover:text-blue-800 dark:hover:text-blue-400` links | `style={{ color: "var(--action-primary)" }}` + `hover:underline` (drop the color-shift-on-hover classes; underline is the hover cue instead) |
| `text-gray-500 dark:text-gray-400` / similar muted text | `style={{ color: "var(--text-muted)" }}` (or `--text-secondary` for less-muted subordinate text — use judgment same as the League overview page's migration, `frontend/src/pages/league/[leagueId]/index.tsx`) |
| Primary heading/value text (currently unstyled-color or `text-gray-900 dark:text-gray-100`) | `style={{ color: "var(--text-primary)" }}` |
| Ad hoc spinner (`<div className="... border-blue-600 border-t-transparent rounded-full animate-spin" />`) | `<Skeleton className="..." />` shaped to the content being loaded (`@/components/ui/skeleton`) |
| Ad hoc red error banner (`bg-red-100 dark:bg-red-900 ... text-red-700`) | `<ErrorState message={...} />` (`@/components/design-system/ErrorState`) — pass `onRetry` only if the page already has a retry action today; do not add one if it doesn't |
| Sortable/tabular data table | `<DataTable>` (`@/components/design-system/DataTable`) — see the League overview page's migration (`frontend/src/pages/league/[leagueId]/index.tsx`, already committed, part of the pilot rather than this rollout's own task numbering) for the pattern: define `DataTableColumn[]`, keep sort *state* owned by the page, `DataTable` only renders headers/calls back |
| A matrix/grid-shaped table that doesn't fit rows-of-columns (e.g. head-to-head grids) | Keep custom `<table>` markup, just retokenize colors — don't force it into `DataTable`'s shape |
| Win/loss/status-colored cells or pills (green/red/yellow Tailwind backgrounds) | Use the semantic status tokens: `--status-success-fg/-bg`, `--status-warning-fg/-bg`, `--status-danger-fg/-bg`, `--status-info-fg/-bg` — reserve for actual status meaning (a win, a loss, a warning), not decoration |
| Hall-of-Fame/Wall-of-Shame-style full pastel-gradient background cards | `<Card>` with a colored left border/accent icon using `--status-success-fg` (positive/best) or `--status-danger-fg` (negative/worst) instead of a full gradient wash — keeps the meaningful color-coding without a light pastel panel clashing with the dark neutral canvas |
| Badges/pills for simple status labels | `<Badge>` (`@/components/ui/badge`) where it fits, or an inline token-colored `<span>` if `Badge`'s variants don't fit the specific case |
| Tab-style view switching (if present) | If not already using a real tab primitive, install `npx shadcn@latest add tabs` and use it — check for this specifically in `players/[playerId].tsx`, `league/[leagueId]/teams/[teamId].tsx`, and `league/[leagueId]/transactions/index.tsx`, which all showed tab-like patterns in a pre-scan |

**Before writing any code**, read the actual current file in full — this brief tells you the transformation rules and what to watch for, not the file's exact current logic. Do not guess at behavior from the table above.

---

## Task 1: `AllTimeMatchupsGrid.tsx`

**File:** `frontend/src/components/AllTimeMatchupsGrid.tsx` (389 lines)

Head-to-head win/loss grid, consumed by the already-migrated League overview page (`frontend/src/pages/league/[leagueId]/index.tsx`) — same props (`teams`, `headToHeadRecords`), do not change its interface. This is the matrix-shaped table from the recipe table above — keep the custom `<table>` markup, retokenize the win/loss/neutral cell backgrounds and the sticky header/total-column shading using the status tokens and `--surface-sunken`/`--border-subtle` instead of the current green-100/red-100/gray Tailwind palette.

---

## Task 2: `HallOfFameWallOfShame.tsx`

**File:** `frontend/src/components/HallOfFameWallOfShame.tsx` (260 lines)

Also consumed by League overview (`leagueId`, `schedule`, `isLoading`, `teams` props — do not change). This is the gradient-card case from the recipe: replace the yellow (`from-yellow-50 to-yellow-100`) and red (`from-red-50 to-red-100`) gradient backgrounds with `Card` + a `--status-success-fg`/`--status-danger-fg` accent border, keeping the 🏆/💩 emoji headers and all the underlying data/logic (season champion, last place, whatever else this component computes — read it in full) exactly as-is.

---

## Task 3: `AllTimeRecordsTable.tsx`

**File:** `frontend/src/components/AllTimeRecordsTable.tsx` (272 lines)

Also consumed by League overview (`leagueId` prop). Has its own `useState`/`useEffect` (own data fetch) and a `.sort(` — preserve exactly. Likely fits `DataTable` (a real column/row table per its name) — use it if the shape fits; if it has structural quirks that don't map cleanly to `DataTable`'s column model, keep custom markup retokenized instead and say why in your report.

---

## Task 4: Home page

**File:** `frontend/src/pages/index.tsx` (80 lines)

Smallest remaining page — hero text, a grid of league cards (`Link` wrapping a card, already seen clashing badly against the dark shell in a live browser check), and a "Sleeper Data" stat-card row. Cards → `Card`/`CardContent`; the 3 stat tiles (Leagues/Trades/Drafts counts) are a good fit for `StatCard` (`@/components/design-system/StatCard`).

---

## Task 5: Sleeper drafts

**File:** `frontend/src/pages/sleeper/drafts.tsx` (185 lines)

Has its own loading/error states and a `<table>`. Check whether the table is sortable (→ `DataTable`) or a plain list (→ retokenize in place).

---

## Task 6: Sleeper transactions

**File:** `frontend/src/pages/sleeper/transactions.tsx` (189 lines)

Same shape as Task 5 — own loading/error states, one table.

---

## Task 7: Sleeper trades

**File:** `frontend/src/pages/sleeper/trades.tsx` (270 lines)

Same shape as Tasks 5-6, slightly larger.

---

## Task 8: League transactions

**File:** `frontend/src/pages/league/[leagueId]/transactions/index.tsx` (334 lines)

Pre-scan showed tab-like (`Tab(`) patterns — check whether this page switches between transaction types (trades/waivers/etc.) via hand-rolled tab buttons; if so, install and use shadcn `Tabs` per the recipe.

---

## Task 9: Players list

**File:** `frontend/src/pages/players/index.tsx` (408 lines)

Has real client-side state (9 `useState`, filtering) — likely search/position/team filters over a player table. Preserve every filter's exact behavior. Table → `DataTable` if sortable.

---

## Task 10: League schedule list

**File:** `frontend/src/pages/league/[leagueId]/schedule/index.tsx` (431 lines)

Has `useState`, `.filter(`, `.sort(` — likely a week-by-week matchup list with some filtering/sorting. Preserve exactly.

---

## Task 11: Admin page

**File:** `frontend/src/pages/admin/index.tsx` (502 lines)

Has 4 `<table>` elements — likely several distinct data/admin tables. Go through each one individually; not all four need to become `DataTable` if some are simple non-sortable summaries — use the recipe's judgment call per table.

---

## Task 12: `InteractiveSimulation.tsx`

**File:** `frontend/src/components/InteractiveSimulation.tsx` (504 lines)

Consumed by the Simulations page (Task 16, which comes after this one so it can rely on this component already being migrated). Has its own state, a `bg-gradient`, a `.sort(`/`.filter(`, and likely chart rendering (`recharts`, already used elsewhere in this repo — reuse existing chart-token conventions if this component charts anything, matching how `ScoringMarginChart` in the Phase 1 prototype used `--chart-*` tokens, even though that prototype code isn't reused directly here).

---

## Task 13: League teams list

**File:** `frontend/src/pages/league/[leagueId]/teams/index.tsx` (676 lines)

Has `useMemo`/`useState`/`.sort(` — a sortable team list, likely similar in shape to League overview's Standings table. Reuse the `DataTable` pattern from that migration directly where it fits.

---

## Task 14: League matchup detail

**File:** `frontend/src/pages/league/[leagueId]/schedule/[matchupId].tsx` (858 lines)

Heavy `.filter(`/`.sort(` (11 and 6 occurrences) — likely box-score / roster comparison logic. Read carefully; this is one of the largest remaining files and likely has the most business logic per line to preserve.

---

## Task 15: Player detail

**File:** `frontend/src/pages/players/[playerId].tsx` (951 lines)

Pre-scan showed tab patterns — likely switches between stat views (season/game log/etc.) via hand-rolled tabs. Install and use shadcn `Tabs` if so, per the recipe. Large file — read in full before starting.

---

## Task 16: League simulations page

**File:** `frontend/src/pages/league/[leagueId]/simulations/index.tsx` (1262 lines)

Depends on Task 12 (`InteractiveSimulation.tsx` should already be migrated). The largest page in the app — 17 `useState`, 9 `.sort(`, 6 `.filter(`, 5 `useEffect`, 4 tables. Budget real time to read this one fully before touching anything; do not skim.

---

## Task 17: League team detail

**File:** `frontend/src/pages/league/[leagueId]/teams/[teamId].tsx` (1640 lines)

The largest single file in the whole rollout. Pre-scan showed tab patterns (16 matches) plus heavy filter/sort logic (22 `.filter(`, 10 `.sort(`). Install shadcn `Tabs` if confirmed. Given the size, if you find the task genuinely doesn't fit in one pass, report `DONE_WITH_CONCERNS` with a clear account of what's done vs. remaining rather than silently cutting corners on business-logic preservation.

---

## Verification (every task)

- `cd frontend && npm run build` and `npm run lint`, both clean.
- Manual check: the implementer should describe, from reading the diff, that every piece of business logic identified in "read the actual current file in full" is present unchanged in the new version — this is the single most important thing the task reviewer will verify.

## Final verification (after all 17 tasks)

A last whole-branch pass: full build+lint, then a real browser sweep (the controller has a working claude-in-chrome connection now) of every page in both light and dark theme, confirming no old-Tailwind color classes remain visible anywhere and the app reads as one coherent system end to end.
