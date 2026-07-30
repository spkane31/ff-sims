Status: in-progress

# Phase 1 (PR 1) task breakdown — Foundation and interaction model

Source plan: `.local/aitasks/2026-07-26/design-system-research/plan.md`. This
file breaks that plan's Phase 1 into concrete, independently-reviewable
tasks for subagent-driven-development. It does not replace the source plan;
it operationalizes it.

Decisions confirmed with the human partner before this breakdown was written:

- Pilot page for the prototype: **League overview**.
- No Figma source of truth — code (CSS custom properties + Tailwind) is the
  only source of truth for Phase 1.
- The prototype is an isolated route inside the existing frontend app
  (`frontend/src/pages/design-system/**`), reachable by direct URL, but not
  linked from any production navigation.

## Global Constraints (binding on every task below)

1. **No new npm dependencies.** Do not install shadcn/ui, Radix, or any other
   package. Phase 1 is token + prototype only; Phase 2 is where the real
   component library (shadcn/ui + Radix) gets installed. Use plain
   React/Tailwind for the prototype.
2. **No production page or route changes.** Do not touch anything under
   `frontend/src/pages/` outside the new `frontend/src/pages/design-system/`
   tree. Do not modify `frontend/src/pages/_app.tsx` or any existing
   component in `frontend/src/components/` used by production pages. Do not
   add links to the new route from any production nav/menu.
3. **No application/API/data changes.** All prototype content uses local
   mock data defined inline or in a small local fixtures file under the new
   `design-system` tree. No real API calls.
4. **Tokens are the only source of visual values inside the prototype and
   docs.** No raw hex/oklch/px literals inside prototype component
   JSX/CSS other than in the token definitions themselves (Task 1). Spacing
   may use Tailwind's default scale (already token-like); do not hardcode
   arbitrary pixel values for colors, radii, or shadows.
5. **Accessibility is verified, not assumed.** Every color pair listed in
   Task 1 must be checked against its stated WCAG 2.2 contrast minimum and
   the actual computed ratio recorded in the token inventory doc. Every
   interactive element built in Tasks 5-6 must be reachable and operable by
   keyboard alone, with a visible focus indicator using `--focus-ring`.
6. **Respect `prefers-reduced-motion`** for any transition/animation added
   in Tasks 5-6.
7. **Doc deliverables live under**
   `.local/aitasks/2026-07-26/design-system-research/phase1-docs/` as
   markdown files (this directory does not exist yet — create it in Task 1).

---

## Task 1: Token taxonomy — primitive/semantic CSS custom properties + inventory doc

Extend `frontend/src/styles/globals.css`, which already defines a small set
of chart tokens (`--chart-axis-text`, `--chart-grid`, `--chart-legend-text`,
`--chart-tooltip-bg`, `--chart-tooltip-text`, `--chart-tooltip-border`,
`--chart-zero-line`) with a light default in `:root` and a dark override
under `@media (prefers-color-scheme: dark)`. Keep that existing convention
and pattern — do not restructure it — and add to it.

### 1a. Add a manual theme override layer

Above the existing `:root { ... }` block, keep the system-preference dark
override as-is, and additionally add manual override classes so the
prototype (Task 5) can offer a theme toggle that does not depend on the OS
setting:

```css
/* manual overrides take precedence over the OS-preference media query */
.light {
  color-scheme: light;
}
.dark {
  color-scheme: dark;
}
```

The `.light`/`.dark` class blocks below carry the actual token values (see
1b). A `<html>` (or wrapping element) with class `dark` or `light` applied
by the prototype's theme toggle overrides the media-query default; no class
means "follow OS preference," matching existing behavior for the rest of
the app.

### 1b. Add the following semantic tokens

Add these new custom properties, each with a light value (in `:root`, and
mirrored in `.light`) and a dark value (in the existing
`@media (prefers-color-scheme: dark) { :root { ... } }` block, and mirrored
in `.dark`). Do not duplicate the existing `--chart-*` tokens — only add the
new ones listed under "Chart series" below alongside them.

Starting oklch values are given below. Treat them as a starting point, not
gospel: compute the actual contrast ratio for every pair marked "must pass"
and adjust the **L (lightness) channel only** (keep H and C constant) until
the ratio clears its minimum. Record final values and measured ratios in the
inventory doc (1c).

```css
/* surfaces */
--surface-canvas:   oklch(0.98 0.004 250);  /* dark: oklch(0.19 0.01 255) */
--surface-raised:   oklch(1    0     0);    /* dark: oklch(0.24 0.012 255) */
--surface-sunken:   oklch(0.965 0.004 250); /* dark: oklch(0.16 0.01 255) */
--border-subtle:    oklch(0.90 0.006 250);  /* dark: oklch(0.32 0.012 255) */
--border-strong:    oklch(0.82 0.008 250);  /* dark: oklch(0.42 0.014 255) */

/* text — text-primary on surface-canvas and surface-raised must pass 4.5:1 (normal text) */
--text-primary:      oklch(0.24 0.02  255); /* dark: oklch(0.96 0.004 255) */
--text-secondary:    oklch(0.38 0.015 255); /* dark: oklch(0.85 0.008 255) */
--text-muted:        oklch(0.55 0.012 255); /* dark: oklch(0.65 0.012 255) */
--text-inverse:      oklch(0.98 0.004 250); /* dark: oklch(0.20 0.02  255) */

/* action accent — action-on-primary on action-primary must pass 4.5:1 */
--action-primary:        oklch(0.55 0.18 250); /* dark: oklch(0.68 0.17 250) */
--action-primary-hover:  oklch(0.50 0.19 250); /* dark: oklch(0.73 0.16 250) */
--action-primary-active: oklch(0.45 0.19 250); /* dark: oklch(0.78 0.15 250) */
--action-on-primary:     oklch(0.98 0.01 250); /* dark: oklch(0.15 0.02 250) */

/* focus — must be visible (>=3:1) against both surface-canvas and surface-raised */
--focus-ring: oklch(0.55 0.18 250); /* dark: oklch(0.75 0.18 250) */

/* status — reserved for meaning, never decoration. -fg must pass 4.5:1 on -bg */
--status-success-fg: oklch(0.35 0.12 145); --status-success-bg: oklch(0.95 0.05 145);
--status-warning-fg: oklch(0.35 0.13 80);  --status-warning-bg: oklch(0.95 0.06 80);
--status-danger-fg:  oklch(0.40 0.18 25);  --status-danger-bg: oklch(0.95 0.05 25);
--status-info-fg:    oklch(0.38 0.12 250); --status-info-bg:  oklch(0.95 0.04 250);
/* dark variants: swap fg/bg roles in the way that preserves the >=4.5:1 pairing,
   e.g. lighten -fg and darken -bg by roughly the same amount used for text/surface above */
```

### 1c. Add chart series + status tokens alongside the existing chart tokens

The existing `--chart-*` tokens (axis/grid/legend/tooltip/zero-line) already
have light+dark values — leave them untouched. Add:

```css
/* colorblind-safe qualitative series palette (Okabe-Ito inspired) */
--chart-series-1: oklch(0.60 0.15 250); /* blue */
--chart-series-2: oklch(0.70 0.15 70);  /* orange */
--chart-series-3: oklch(0.65 0.15 150); /* green */
--chart-series-4: oklch(0.55 0.20 320); /* magenta/purple */
--chart-series-5: oklch(0.75 0.15 100); /* yellow-green */
--chart-series-6: oklch(0.50 0.05 250); /* neutral baseline */

/* distinct from --status-*: these encode data meaning (e.g. positive/negative
   swing in a scoring chart), not UI operation outcomes */
--chart-positive: oklch(0.55 0.15 145);
--chart-negative: oklch(0.55 0.18 25);
```
Provide dark-mode variants for all of the above (lighten by roughly the same
delta used for the existing `--chart-*` dark overrides, keeping hue/chroma),
placed in the same `@media (prefers-color-scheme: dark)` block and mirrored
in `.dark`.

### 1d. Token inventory doc

Create `.local/aitasks/2026-07-26/design-system-research/phase1-docs/token-inventory.md`
listing every primitive/semantic token added above: name, purpose (one
line), light value, dark value, and — for every pair flagged "must pass" —
the computed contrast ratio and pass/fail against its stated minimum. Write
and run a small throwaway script (Node or Python, your choice, delete it
after use or put it in the scratchpad — do not commit it) to compute oklch
→ relative luminance → WCAG contrast ratio; do not eyeball contrast.

### Verification
- `cd frontend && npm run build` succeeds (confirms globals.css is valid
  CSS Next.js can compile).
- Every "must pass" pair in the doc shows a computed ratio meeting its
  minimum (4.5:1 for text pairs, 3:1 for the focus ring and UI-component
  pairs).

---

## Task 2: Component / state matrix

Create
`.local/aitasks/2026-07-26/design-system-research/phase1-docs/component-state-matrix.md`.

Build a markdown table with one row per component and one column per state.
Components (from the approved plan, do not add or drop any):
buttons, inputs, select/combobox, badges, tabs, filters, data table, stat
card, chart container, empty state, error state, loading state, mobile
navigation, dialogs.

States/columns: default, hover, focus-visible, active/pressed, disabled,
loading, error, selected, empty (mark N/A where a state doesn't apply to a
component, e.g. a badge has no "loading" state).

For each component also note:
- Whether it needs a Radix primitive underneath (Phase 2 concern) — e.g.
  dialogs/select/combobox/tabs typically do; buttons/badges typically don't.
- Which semantic tokens from Task 1's inventory it consumes (reference by
  name, e.g. `surface-raised`, `action-primary`).
- One-line accessibility requirement specific to that component (e.g.
  "dialog: focus trapped, Esc closes, returns focus to trigger").

### Verification
- Every component from the required list above has a row.
- Every row references at least one token name that exists in
  `token-inventory.md` from Task 1 (cross-check the names match exactly).

---

## Task 3: Responsive rules

Create
`.local/aitasks/2026-07-26/design-system-research/phase1-docs/responsive-rules.md`.

Document, as concrete rules (not prose essays):
- Breakpoint scale: use Tailwind's default scale (sm 640/md 768/lg
  1024/xl 1280) unless you find a concrete reason not to — state the reason
  if you deviate.
- The breakpoint at which the layout switches from mobile shell (bottom
  nav + top bar with league context) to desktop shell (persistent primary
  nav, no bottom nav). Per the approved plan this is mobile-first with a
  persistent five-item bottom nav as the primary mobile pattern.
- Rules for when a data table becomes a ranked-list-row layout vs. staying
  a horizontally-scrollable table with a pinned identifying column (per
  plan: "Do not transform every table into stacked cards" — state which
  tables in the product surface qualify for which treatment, using the
  product surface list in the source plan: league overview, teams/H2H,
  schedules/matchups, players/rankings, transactions/trades/drafts,
  admin views).
- Filter placement rule: bottom sheet / full-screen filter view on mobile,
  inline on desktop, and the breakpoint where that switch happens.
- Minimum touch target size (44px, per the approved plan) and where it
  applies (nav items, table row actions, filter chips, etc).
- A rule that selected/active state never relies on color alone (must pair
  with an icon, weight change, or underline/border).

### Verification
- Every rule names a specific breakpoint or pixel value, not a vague
  qualifier like "small screens."

---

## Task 4: Accessibility checklist

Create
`.local/aitasks/2026-07-26/design-system-research/phase1-docs/accessibility-checklist.md`.

A checklist (checkbox list, not prose) a reviewer can run through against
any future component or page built on this system, targeting WCAG 2.2 AA.
Cover at minimum: keyboard reachability and operability for every
interactive element, visible focus indicator (reference `--focus-ring` from
Task 1), programmatic labels/names for all controls (not just placeholder
text), text contrast (reference the 4.5:1 minimum from Task 1's inventory),
non-text/UI-component contrast (3:1), the WCAG 2.2 target-size minimum
(name the SC number: 2.5.8, and the 44px value already chosen in the
approved plan, which exceeds the SC's 24px floor), non-color status/selected
indication, and `prefers-reduced-motion` handling for any motion.

Cite the specific WCAG 2.2 success criterion number next to each checklist
item where one exists.

### Verification
- Every checklist item that corresponds to a numbered WCAG 2.2 success
  criterion cites that number.

---

## Task 5: App shell prototype

Depends on: Task 1 (tokens must exist).

Create the isolated prototype route and a reusable shell component:

- `frontend/src/pages/design-system/index.tsx` — a small landing page for
  the prototype tree, linking to the League overview prototype (Task 6).
- A shell component (suggested path:
  `frontend/src/components/design-system-prototype/AppShell.tsx`, but
  choose your own organization if cleaner — keep it under
  `design-system-prototype` so it can't be mistaken for a production
  component) implementing:
  - Top bar: league context/switcher (mock league name + a non-functional
    dropdown affordance is fine — no real switching logic needed), primary
    nav links, and a filter entry-point affordance.
  - Persistent five-item bottom navigation, visible below the breakpoint
    decided in Task 3's responsive-rules doc, hidden above it (replaced by
    the top bar's primary nav).
  - A theme toggle control (button or switch) that applies the `light` or
    `dark` class from Task 1 to the top-level wrapper element, defaulting
    to no class (OS preference).
  - All colors/surfaces via the semantic tokens from Task 1 (arbitrary
    Tailwind value syntax, e.g. `bg-[var(--surface-canvas)]`, or plain
    inline `style` — either is fine, just no hardcoded colors).
- Every nav item and the theme toggle must be reachable and operable via
  keyboard (Tab/Enter/Space), with a visible focus outline using
  `--focus-ring`.

Read `component-state-matrix.md` (Task 2) and `responsive-rules.md` (Task 3)
before building — they define what states/breakpoints this shell must
support. If either doc doesn't exist yet when you start, say so in your
report; do not guess at their content.

### Verification
- `cd frontend && npm run build` succeeds.
- `cd frontend && npm run lint` passes on the new files.
- Manually confirm (describe in your report) that: bottom nav disappears
  and top nav appears at the documented breakpoint; theme toggle switches
  all prototype surfaces/text between the light and dark token sets; every
  interactive element in the shell is reachable by keyboard alone.

---

## Task 6: League overview prototype page

Depends on: Task 1 (tokens), Task 5 (shell).

Create `frontend/src/pages/design-system/league-overview.tsx`, wrapped in
the `AppShell` from Task 5, using local mock data only (define a small
fixtures object/file in the same directory — no API calls). Do not copy the
current production league page's styling; only reuse its content categories
as the source of information architecture:

- One current/featured matchup score card.
- A horizontally-scrollable row of summary metric tiles (e.g. total points
  scored, average margin, completed games — use plausible mock numbers).
- A single-series chart (reuse the `recharts` dependency already in the
  project, and the existing `--chart-*` tokens plus the new
  `--chart-series-*`/`--chart-positive`/`--chart-negative` tokens from
  Task 1 — do not introduce a new charting library).
- A compact standings list (5-8 mock teams) with a "view all" affordance
  (does not need to link anywhere real — a disabled-looking or
  no-op affordance is fine, just don't 404).
- Empty, loading, and error state variants of at least one of the above
  elements (e.g. show what the summary metrics row looks like with no data,
  while loading, and on fetch failure) — reachable via a simple
  local state toggle in the page (e.g. buttons that switch a mock
  `viewState`), not a real async flow.

Same accessibility bar as Task 5: keyboard-operable, visible focus,
non-color status cues, `prefers-reduced-motion` respected for any chart
animation or transition.

### Verification
- `cd frontend && npm run build` succeeds.
- `cd frontend && npm run lint` passes on the new files.
- Manually confirm (describe in your report) that the empty/loading/error
  toggle actually changes what's rendered, and that the chart and standings
  list both re-theme correctly when the shell's theme toggle is used.

---

## Out of scope for this task file (unchanged from source plan)

- Any production page migration, dependency installation (shadcn/ui,
  Radix), or navigation changes.
- Phase 2 (real reusable primitives), Phase 3 (real pilot migration), and
  Phase 4 (rollout/governance) work.
