Status: in-progress

# Design-system Phase 2 + 3: real component library + League overview pilot migration

## Objective

Move past Phase 1's isolated prototype (PR #204, merged into `spk/design-redo`) into a real migration: build the actual reusable component library (Phase 2 of the source plan) and use it to migrate one real production page — League overview — plus the shared app shell (Phase 3). This is a deliberately scoped first sub-project; the remaining 12 production pages are out of scope until their own turn in Phase 4.

Source documents:
- `.local/aitasks/2026-07-26/design-system-research/plan.md` — the original direction/vision doc (Option A, "Editorial scoreboard," phases 1-4).
- `.local/aitasks/2026-07-26/design-system-research/phase1-tasks.md` — Phase 1's task breakdown (tokens, docs, isolated prototype), already implemented and merged as PR #204.

## Decisions made during brainstorming (binding on the implementation plan)

1. **Sub-project scope:** build the real component library + migrate exactly one real page (League overview) as pilot. The other 12 pages stay untouched in content, but do inherit the new shell (see #4).
2. **Pilot page:** League overview (`frontend/src/pages/league/[leagueId]/index.tsx`), continuing directly from Phase 1's prototype of the same page.
3. **Component library approach:** run the real shadcn/ui CLI init (adds its config + Radix primitives per-component as pulled in), per the source plan's approved Option A. Not a hand-rolled alternative.
4. **Shell scope:** replace the shared `Layout` component (used by all 13 production pages) with a production `AppShell` globally, now — not scoped to just League overview. Every page immediately gets the new nav/chrome; only League overview's inner content is redesigned this round. This matches the source plan's own Phase 3 sample code (`<AppShell><PlayersPage /></AppShell>`).
5. **Theme mechanism unification:** switch Tailwind's `dark:` variant globally from OS-preference-only to the manual `.light`/`.dark` class mechanism already established in Phase 1's tokens (`@custom-variant dark (&:where(.dark, .dark *))`), plus a persisted (localStorage), flash-avoiding theme-init script and a real toggle in the shell. Required because otherwise toggling the new shell's theme switch would not affect the other 12 not-yet-migrated pages' existing `dark:` utility classes, producing a visibly mismatched shell/content theme. Zero visual change for anyone who never touches the toggle (default follows OS preference either way).
6. **League overview content freedom:** open to reorganizing section order/grouping/emphasis, but **not** to introducing new sub-routes or new information not already on the page today. Single route, same six sections, same data.
7. **League overview layout:** "Layout B" — group the six existing sections into two labeled bands: **This season** (Standings, League Summary stat cards, League Leaders) and **League history** (Head-to-Head Records, Hall of Fame & Wall of Shame, All-Time Team Records). Chosen over (A) same-order-restyled and (C) standings-led dashboard-with-rail via a visual mockup comparison.
8. **Git workflow for this sub-project:** implementation proceeds with internal git commits (needed for the per-task review/fix-loop process used in Phase 1), but the branch is squashed back to fully unstaged working-tree changes (`git reset --soft` to the pre-work commit) before final human review — no commit is meant to persist until the human explicitly signs off, mirroring Phase 1's "don't create the PR until sign-off" but one step earlier (no interim commits survive either).

## What must NOT change

- No backend/API changes. `leaguesService`, `useTeams`, `useSchedule` stay as-is; no new endpoints.
- No product-behavior change on League overview: sortable standings columns (rank/name/wins/losses/PF/PA/diff/playoff%), the playoff-% bar, the `!team.owner.includes("Knapp") && !team.owner.includes("Landry")` owner exclusion filter, and all current loading/error states must work identically to today.
- The other 12 production pages' *content* is untouched — only their chrome (via the shared shell swap) changes visually in this sub-project.

## Real component library to build (Phase 2, scoped to this sub-project's needs)

Installed via shadcn/ui CLI, Radix underneath where the primitive needs managed focus/keyboard behavior. Build only what League overview + the new shell actually consume — everything else in `component-state-matrix.md`'s 14-component list (tabs, filters, dialogs, chart container, inputs, combobox) waits for a page that actually needs it:

- **Button** — nav items, retry/error actions, sort-toggle affordances.
- **Badge** — status/rank indicators where useful (e.g. current-week indicator).
- **Card** — section containers (replaces the current `bg-white dark:bg-gray-700 rounded-lg shadow-md` pattern).
- **StatCard** — League Summary tiles, League Leaders entries.
- **DataTable** — sortable, with custom cell renderers for the playoff-% bar and the PF/PA "(avg) (total)" formatting; used for Standings, and reused as-is or extended for Head-to-Head/All-Time Records if their shape fits.
- **Select** — real league switcher (reads `useLeagues`), and the shell's nav "More" overflow if a plain disclosure isn't sufficient.
- **Skeleton / EmptyState / ErrorState** — replacing the current ad hoc spinners/error banners, one implementation reused across all of League overview's independently-loading sections (league header, standings, schedule-derived stats).
- **AppShell / TopBar / BottomNav / ThemeToggle** — promoted from Phase 1's prototype versions in `frontend/src/components/design-system-prototype/` into real, production-facing components (new location, e.g. `frontend/src/components/shell/`), wired to real routes/league data instead of mock fixtures.

## Shell navigation mapping (real routes)

Corrected against the actual current `frontend/src/components/Header.tsx` (which this replaces) rather than guessed — the existing header already encodes exactly this logic, keyed off `router.query.leagueId`:

- **League-scoped** (`leagueId` present in the route): League/Overview (`/league/{id}`), Simulations (`/league/{id}/simulations`), Schedule (`/league/{id}/schedule`), Teams (`/league/{id}/teams`), Players (`/players`), Transactions (`/league/{id}/transactions`), plus an "All Leagues" link back to `/`.
- **Global** (no `leagueId`: home, `/players`, `/admin`, `/sleeper/*`): Home (`/`), Players (`/players`), Trade Data (`/sleeper/trades`), Draft Data (`/sleeper/drafts`).
- `/admin` and `/sleeper/transactions` are **not** linked from nav today (reachable only by direct URL) — preserve that; do not add nav entries for them.
- **Desktop top bar:** show the full applicable list above (matches current `Header.tsx` desktop behavior — no "More" needed at `lg`+, there's already room).
- **Mobile bottom nav (5 items max, per the source plan's own "persistent five-item bottom navigation"):** curate to the 4 most-used + a "More" 5th item that opens the rest in a sheet/disclosure:
  - League-scoped: Overview, Schedule, Teams, Players primary; More → Simulations, Transactions, All Leagues.
  - Global: Home, Players primary two (+2 free slots — just show Trade Data, Draft Data directly, no More needed since only 4 items total).
- **League switcher:** real `Select` sourced from `useLeagues`; only rendered on league-scoped routes (Overview/Schedule/Teams/Transactions/Simulations) — matches current `Header.tsx`, which only fetches/shows a league name when `leagueId` is present.

## Codebase findings that refine this plan

- `frontend/src/hooks/useLeagues.ts` already exports a `useLeague(leagueId)` hook identical in shape to what the League overview page currently hand-rolls with local `useState`/`useEffect` around `leaguesService.getLeague(id)`. The migrated page should use the existing hook instead of duplicating it — same request, same shape, just less code. This is a legitimate small cleanup incidental to the migration, not a scope expansion.
- No frontend test framework exists in this repo (`frontend/package.json` has no test script; no `*.test.*` files; no jest/vitest config). Per this project's own testing convention ("only write new tests for bug fixes or new features; do not add tests for refactors"), and since this migration is refactor-flavored (preserve existing behavior, new components), this plan does **not** introduce a test framework or automated tests. Task verification is `npm run build` + `npm run lint` + manual browser check, matching Phase 1's precedent.
- `frontend/src/components/Footer.tsx` renders fixed joke copy ("Powered by Male Friendship™" etc.) and About/Contact links — preserve this content verbatim; only its visual implementation changes.
- shadcn/ui's CLI generates its own CSS-variable layer (`--background`, `--foreground`, `--primary`, `--card`, `--border`, `--ring`, etc.) into `globals.css` by default. To avoid running two disconnected token systems side by side, Task 1 aliases shadcn's expected variable names to this repo's existing Phase-1 tokens (`--background: var(--surface-canvas)`, `--foreground: var(--text-primary)`, `--primary: var(--action-primary)`, `--card: var(--surface-raised)`, `--border: var(--border-subtle)`, `--ring: var(--focus-ring)`, etc.) instead of letting shadcn's own default values stand. This keeps shadcn's generated component code untouched while it visually inherits our tokens automatically.

## Verification

- `cd frontend && npm run build` and `npm run lint`, clean.
- Manual browser check (dev server + click-through, per this project's convention for UI changes): League overview renders correctly, shell nav works and switches at 1024px, theme toggle now correctly re-themes *all* pages (spot-check at least one of the other 12 pages in both themes to confirm the `dark:` variant unification didn't regress anything), sortable standings still sort, playoff-% bar still renders, loading/error states still behave as before.
- No new backend calls; confirm via network tab / existing hook behavior that data flow is unchanged.

## Out of scope for this sub-project

- Any new information/metrics not already present on a given page today.
- New sub-routes (e.g. a dedicated standings or all-time-records page).

## Scope change: full rollout (decided after seeing the pilot live)

The pilot (global shell + League-overview-only content migration) was implemented and reviewed as planned, but visually inspecting it surfaced a real problem the plan didn't anticipate: a global new shell wrapping pages that are still 100% old styling (hardcoded Tailwind grays/blues, `shadow-md`/`rounded-lg` cards, gradient Hall-of-Fame/Wall-of-Shame cards) reads as two unrelated design systems stitched together — visible on every one of the 12 non-migrated pages, and even *inside* League overview itself, where three shared sub-components (`AllTimeMatchupsGrid`, `HallOfFameWallOfShame`, `AllTimeRecordsTable`) were deliberately left unmigrated per the original task brief.

Decision: migrate everything now rather than continue incrementally. This supersedes the "single pilot page" framing above — Phase 4 (incremental rollout) is pulled forward into this same sub-project. Every remaining production page and the three shared sub-components get migrated onto the design system in this pass; the old Tailwind-hardcoded styling is fully retired, not left to coexist.

**What stays true from the original plan:** no API/data changes, no product-behavior changes (every computed value, filter, sort, and interactive behavior on every page must survive verbatim), no new information/metrics, component library built incrementally (add a new primitive — e.g. Tabs, Dialog — only when a specific page actually needs it, not speculatively), and the same build+lint+manual-browser verification bar.

**What changes:** scope is now all 12 remaining pages (`/`, `/admin`, `/players`, `/players/[playerId]`, `/league/[leagueId]/schedule`, `/league/[leagueId]/schedule/[matchupId]`, `/league/[leagueId]/simulations`, `/league/[leagueId]/teams`, `/league/[leagueId]/teams/[teamId]`, `/league/[leagueId]/transactions`, `/sleeper/drafts`, `/sleeper/trades`, `/sleeper/transactions`) plus the three shared sub-components, not just League overview.

**Execution:** subagent-driven-development (per-task implementer + reviewer, same as Phase 1), given the volume and the amount of business logic each file carries that must not regress. See `.local/aitasks/2026-07-28/phase2-pilot-migration/rollout-tasks.md` for the task breakdown and the shared migration recipe every task follows.
