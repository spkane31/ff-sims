# Responsive rules — Phase 1, Task 3

Source plan: `.local/aitasks/2026-07-26/design-system-research/plan.md`
("mobile-first, with the approved compact overview and persistent bottom
navigation"; "Do not transform every table into stacked cards"). This doc
turns that direction into rules with concrete breakpoints, for direct use
by Task 5 (app shell prototype) and Task 6 (league overview prototype
page). Every rule below names a pixel value — none are left as "small
screens" / "on mobile" without a number.

## 1. Breakpoint scale

Use Tailwind's default v4 scale, unmodified. Confirmed against
`frontend/tailwind.config.js` (no `theme.screens` override present) and
`frontend/postcss.config.mjs` (`@tailwindcss/postcss`, no custom preset) —
there is no existing project reason to deviate, so this doc does not
introduce one.

| Name | Min-width (px) | Min-width (rem) | Tailwind prefix |
|---|---|---|---|
| (base/mobile) | 0 | 0 | none — unprefixed utilities |
| sm | 640 | 40rem | `sm:` |
| md | 768 | 48rem | `md:` |
| lg | 1024 | 64rem | `lg:` |
| xl | 1280 | 80rem | `xl:` |
| 2xl | 1536 | 96rem | `2xl:` |

All rules below are stated as one of these named breakpoints plus its
literal px value, so Tasks 5-6 can use the matching Tailwind prefix
directly (e.g. `lg:hidden`, `md:flex-row`) with no invented intermediate
values.

## 2. Shell breakpoint: mobile shell vs. desktop shell

**Rule: the shell switches from mobile chrome to desktop chrome at `lg`
(1024px), using `lg:` / below-`lg` Tailwind prefixes.**

- Below 1024px (base through `md`, i.e. phones and portrait/small
  tablets): mobile shell — persistent five-item bottom navigation bar
  fixed to the viewport bottom, plus a top bar carrying league
  context/switcher and a filter entry-point affordance. No persistent
  primary nav in the top bar at these widths (the bottom bar is primary
  navigation).
- At 1024px and above (`lg` and up): desktop shell — the five-item bottom
  navigation bar is removed from the layout (not just visually hidden;
  do not leave it in the tab order), and the top bar instead carries the
  persistent primary nav links directly, alongside the league
  context/switcher and filter entry-point.
- Implementation shape for Task 5: bottom nav container uses `lg:hidden`;
  the top bar's primary-nav link group uses `hidden lg:flex`.

**Why 1024px and not 768px (`md`):** the product surfaces this shell
wraps (schedules, players/rankings, teams/H2H) are column-dense (see
Section 3). A portrait tablet (768-1023px) still benefits from the
mobile-optimized bottom-nav pattern because a horizontal top nav at that
width has to fit the league switcher, 5+ primary nav destinations, and a
filter entry-point without wrapping or truncating labels — 1024px is
where that horizontal chrome comfortably fits without crowding. This
keeps one shell-level threshold (see Section 4, which reuses it for
filter placement) instead of tuning nav and filters at two different
breakpoints.

## 3. Table treatment by product surface

**General rule:** below `md` (768px), every data table in the product
must use one of two mobile treatments — ranked-list-row layout, or a
horizontally-scrollable table with the identifying column pinned. Neither
treatment is "stacked cards with every column repeated as a labelled
field" — that pattern is explicitly excluded per the approved plan ("Do
not transform every table into stacked cards"). At `md` and above, tables
render as standard multi-column tables; a table that uses the
pinned-column horizontal-scroll pattern may continue to use it above
`md`/`lg` as well, if its column count still exceeds available width —
that pattern is not mobile-only, it is the permanent shape for
wide comparison tables at any breakpoint.

**Decision test** (apply to any new table, not just the ones enumerated
below):
- If each row is a single ranked or chronological entity read primarily
  by 1-2 headline values (with any other column being secondary detail
  that can progressively disclose, e.g. an expand affordance) → **ranked-
  list-row** below `md`.
- If the table's primary read is comparing multiple metric columns
  side-by-side across the same set of rows, such that collapsing to
  stacked rows would prevent scanning a column down the page → **horizontally-
  scrollable table, identifying column pinned** at every breakpoint.

Applying that test to the product surface list from the source plan:

| Product surface | Table(s) | Treatment | Breakpoint(s) |
|---|---|---|---|
| League overview / historical records | Standings list (rank, team, W-L, points for) | Ranked-list-row | Below `md` (768px); compact table at `md`+ |
| Teams / head-to-head data | All-teams season comparison (wins, losses, PF, PA, streak per team) | Horizontally-scrollable, team-name column pinned | All breakpoints (already present below `sm`, columns fit without scroll from `xl`/1280px up depending on stat count) |
| Schedules / individual matchups | Weekly matchup list (two teams, score, date/status per row) | Ranked-list-row | Below `md` (768px); compact table at `md`+ |
| Players, rankings, and filters | Player rankings/stats table (rank, name, position, team, points, ADP, trend, ...) | Horizontally-scrollable, player-name column pinned | All breakpoints |
| Transactions, trades, drafts, and simulations | Transaction/trade log (chronological, one event per row) | Ranked-list-row | Below `md` (768px); compact table at `md`+ |
| Transactions, trades, drafts, and simulations | Draft board (round × team grid) | Horizontally-scrollable, round-number column pinned | All breakpoints — a draft board is inherently a matrix, so it is explicitly excluded from ranked-list-row even though it sits under the same surface bullet as the transaction log |
| Administrative data views | Admin/ops tables (sync logs, identity-conflict review, raw record inspection) | Horizontally-scrollable, primary key/identifying column pinned | All breakpoints — admin views are for power users scanning many raw columns, not a simplified summary, so they never convert to ranked-list-row |

Simulations (grouped in the same surface bullet as
transactions/trades/drafts) do not introduce a new table shape in this
doc — simulation output tables (e.g. per-team playoff-odds columns) follow
the same decision test above: if simulation output is presented as
one-team-per-row with a few headline probabilities, treat as ranked-list-
row; if it is presented as many scenario columns per team, treat as
horizontally-scrollable with the team column pinned. This is deferred to
whichever prototype/build task first implements a concrete simulations
table, since the plan doesn't fix a single simulations table shape.

## 4. Filter placement

**Rule: filters render as a bottom sheet or full-screen filter view below
`lg` (1024px), and inline (in the top bar / page toolbar) at `lg` and
above.** This reuses the same 1024px shell breakpoint from Section 2
rather than introducing an independently-tuned filter breakpoint, so the
app has a single mobile/desktop threshold that governs nav chrome and
filter chrome together.

- Below 1024px: the top bar's filter entry-point affordance (an icon
  button, minimum 44px touch target per Section 5) opens a bottom sheet
  or full-screen filter view as an overlay. The overlay must trap focus
  while open and return focus to the entry-point button on close/apply
  (Radix Dialog territory in Phase 2; out of scope to implement in this
  doc).
- At 1024px and above: the same filter controls render inline, in the
  page toolbar area above the table/list content, with no overlay/dialog
  behavior.

## 5. Minimum touch target size

**Rule: every interactive control has a minimum hit area of 44×44px,
independent of its visible icon/label size** (pad a smaller visual icon
out to a 44px hit area with padding rather than enlarging the icon
itself). This exceeds the WCAG 2.2 SC 2.5.8 (Target Size Minimum) 24px
floor; 44px is the value already committed to in the approved plan.

Applies to:
- Bottom navigation items (all five) and any top-bar primary nav link
  rendered as a tap target on touch devices.
- The top bar's filter entry-point button and the league
  context/switcher trigger.
- Table row actions (e.g. an expand/edit/sort icon button in a data
  table row), at every breakpoint the action is rendered, not just below
  `md`.
- Filter chips/toggle controls, whether rendered inline (`lg`+) or inside
  the bottom-sheet/full-screen filter view (below `lg`).
- Tab controls, dialog close buttons, and chart legend items when the
  legend doubles as a series-toggle control.

This is a hit-area rule, not a visual-size rule — visible icon/label size
is a token/typography decision (Task 1/Task 2 concerns), not this doc's.

## 6. Selected/active state: never color alone

**Rule: no component's selected or active state may be conveyed by color
alone.** Every selected/active/current indicator pairs a color change
(e.g. `--action-primary`, per the token inventory) with at least one of:
an icon (e.g. a checkmark or filled dot), a font-weight change, or an
underline/border. This applies at every breakpoint — it is not a
mobile-only or desktop-only rule, since color-only signaling is equally a
problem for low-vision and color-blind users regardless of viewport
width.

Concrete pairings, matching the component/state matrix (Task 2):
- Bottom nav / top bar primary nav current-page indicator: color change
  + `aria-current="page"` + icon fill/weight change (not color swap
  alone).
- Tabs selected tab: color change + `aria-selected` + underline/border
  indicator.
- Filter chip active state: color/fill change + a checkmark or "x to
  remove" icon, not fill alone.
- Data table selected row (checkbox selection): color/tint change +
  checkbox checked state (the checkbox itself is the non-color cue).
- Chart legend series toggle: color change + a strikethrough/dimmed
  treatment on the swatch/label for a hidden series, not opacity or hue
  alone.

## Verification

- Every rule above names a specific breakpoint (640 / 768 / 1024 / 1280 /
  1536px) or pixel value (44px) — no rule uses an unquantified qualifier
  like "small screens" or "on mobile" without a number attached.
- Section 3's table maps every surface from the source plan's product
  surface list (league overview/historical records; teams/H2H;
  schedules/matchups; players/rankings/filters; transactions/trades/
  drafts/simulations; admin views) to a named treatment — none omitted.
