# Component / state matrix — Phase 1, Task 2

Scope: the 14 components named in the approved plan
(`.local/aitasks/2026-07-26/design-system-research/plan.md`, "Components:"
line under "Proposed visual direction for the recommended option"). No
components added or dropped.

Token names referenced below are copied verbatim from
`.local/aitasks/2026-07-26/design-system-research/phase1-docs/token-inventory.md`
(Task 1). That includes the seven pre-existing chart tokens
(`--chart-axis-text`, `--chart-grid`, `--chart-legend-text`,
`--chart-tooltip-bg`, `--chart-tooltip-text`, `--chart-tooltip-border`,
`--chart-zero-line`), which the inventory names explicitly as "untouched"
rather than tabulating with light/dark values — they still count as
existing, named tokens for cross-reference purposes.

`N/A` means the state does not apply to that component's baseline
behavior. Where a component only reaches a given state through an
optional interactive variant (e.g. a stat card that doubles as a
chart-metric selector), that's called out in the cell rather than left
blank.

## 1. State matrix

| Component | Default | Hover | Focus-visible | Active/Pressed | Disabled | Loading | Error | Selected | Empty |
|---|---|---|---|---|---|---|---|---|---|
| Buttons | Solid fill, base label | Fill shifts to `--action-primary-hover` | `--focus-ring` outline, offset from fill | Fill shifts to `--action-primary-active` | Muted fill/text, `aria-disabled`, removed from tab order | Spinner replaces label, `aria-busy`, pointer events off | N/A — destructive is a style variant, not this axis | N/A — plain buttons aren't toggled (see Tabs/Filters) | N/A — a button always renders a label/icon |
| Inputs | `--surface-sunken` fill, `--border-subtle` outline | Outline shifts to `--border-strong` | `--focus-ring` outline + `--border-strong` | N/A — no state beyond focus | Muted border/text, not focusable | N/A — async feedback lives in an adjacent control, not the input itself | Border/text shift to `--status-danger-fg`, `aria-invalid` | N/A — text selection isn't a UI state | Placeholder text in `--text-muted` when empty |
| Select/combobox | Closed trigger, base style | Trigger/option row background shift | `--focus-ring` on trigger and on the active option | Trigger pressed/open | Muted trigger, not focusable | Loading row shown while options fetch | Invalid selection, `--status-danger-fg` border | Chosen option marked with `--action-primary` indicator | "No results" row when the filter matches nothing |
| Badges | `--status-*-fg`/`--status-*-bg` pairing | N/A — non-interactive by default | N/A | N/A | N/A | N/A | Danger variant, `--status-danger-fg`/`--status-danger-bg` | N/A | N/A |
| Tabs | Base label, `--text-secondary` | Label shifts toward `--text-primary` | `--focus-ring` on the focused tab | Momentary press feedback | Muted, unreachable tab | N/A — the tab control itself doesn't load; its panel content may, independently | N/A | Current tab: `--action-primary` indicator + `aria-selected` | N/A |
| Filters | Closed/base chip or panel | Chip/trigger background shift | `--focus-ring` on chip or panel control | Chip/trigger pressed | Filter unavailable for current view | Options list loading (e.g. team/player list fetch) | Invalid combination or failed options fetch, `--status-danger-fg` | Active filter chip, `--action-primary` fill | No filters applied yet / "no matching options" |
| Data table | Base row/header rendering | Row background to `--surface-sunken` | `--focus-ring` on focused header/cell/row | Sortable header pressed | N/A — table itself isn't disabled; a row action's disabled state belongs to that action | Skeleton rows in place of data | Failed-to-load banner in place of rows, `--status-danger-fg` | Row checkbox selected, tint + `--action-primary` | "No results" row/state |
| Stat card | Value + label, `--text-primary`/`--text-secondary` | Applies only if used as an interactive metric selector | Applies only if used as an interactive metric selector | Applies only if used as an interactive metric selector | N/A | Skeleton number while computing | Failed-to-compute indicator, `--status-danger-fg` | Applies only if used as an interactive metric selector (chosen metric highlighted) | No data available for the stat |
| Chart container | Series rendered in `--chart-series-1`…`--chart-series-6` | Tooltip on data-point hover (`--chart-tooltip-bg`/`--chart-tooltip-text`) | Focusable data point/legend item, `--focus-ring` | Legend swatch pressed | N/A | Skeleton/placeholder frame | Inline error + retry, `--status-danger-fg` | Legend toggles which series are shown/dimmed | No data to plot |
| Empty state | Illustration/message, `--text-secondary` | Applies only to its optional CTA button | Applies only to its optional CTA button | Applies only to its optional CTA button | N/A | N/A — that's the Loading state component's job | N/A — that's the Error state component's job | N/A | N/A — this component is itself the empty representation |
| Error state | Message/illustration, `--status-danger-fg` | Applies only to its retry/CTA button | Applies only to its retry/CTA button | Applies only to its retry/CTA button | N/A | N/A | N/A — this component is itself the error representation | N/A | N/A |
| Loading state | Skeleton/spinner render, `--surface-sunken` | N/A — non-interactive | N/A | N/A | N/A | N/A — this component is itself the loading representation | N/A | N/A | N/A |
| Mobile navigation | Base bar, `--surface-raised` | Applies on hybrid-pointer devices | `--focus-ring` on the focused item | Touch-press feedback | N/A | N/A | N/A | Current route, `--action-primary` + `aria-current=page` | N/A |
| Dialogs | Panel open, `--surface-raised` | Applies to interactive elements inside (buttons, inputs) | Applies to interactive elements inside | Applies to interactive elements inside | Applies — e.g. primary action disabled until form is valid | Submit-in-progress state inside the dialog | Validation error surfaced inside, `--status-danger-fg` | N/A — unless the dialog contains a selectable list, out of scope for base dialog chrome | N/A |

## 2. Radix primitive need, tokens consumed, accessibility requirement

| Component | Needs a Radix primitive? | Semantic tokens consumed | Accessibility requirement |
|---|---|---|---|
| Buttons | No — native `<button>` covers the interaction model | `--action-primary`, `--action-primary-hover`, `--action-primary-active`, `--action-on-primary`, `--focus-ring`, `--text-muted`, `--border-subtle` | Native `<button>` with a visible accessible name; disabled state removes it from the tab order (`aria-disabled`) without hiding the label; loading sets `aria-busy`. |
| Inputs | No — native `<input>`/`<label>` covers it | `--surface-sunken`, `--border-subtle`, `--border-strong`, `--text-primary`, `--text-muted`, `--focus-ring`, `--status-danger-fg`, `--status-danger-bg` | Label programmatically associated via `<label for>`/`aria-labelledby`; error state sets `aria-invalid` and links to the error text via `aria-describedby`, not color alone. |
| Select/combobox | Yes — Radix Select/Combobox for listbox semantics, roving focus, typeahead | `--surface-raised`, `--surface-sunken`, `--border-subtle`, `--text-primary`, `--text-muted`, `--action-primary`, `--focus-ring`, `--status-danger-fg`, `--status-danger-bg` | Combobox/listbox ARIA roles with roving focus and arrow-key navigation; Esc closes and returns focus to the trigger. |
| Badges | No — simple, usually non-interactive status marker | `--status-success-fg`, `--status-success-bg`, `--status-warning-fg`, `--status-warning-bg`, `--status-danger-fg`, `--status-danger-bg`, `--status-info-fg`, `--status-info-bg` | Status conveyed by icon/text plus color, never color alone, since badges are usually non-interactive and can't offer a hover tooltip. |
| Tabs | Yes — Radix Tabs for roving tabindex and panel linkage | `--text-secondary`, `--text-primary`, `--action-primary`, `--border-subtle`, `--focus-ring`, `--text-muted` | Roving tabindex across the tab list; arrow keys move focus; the selected tab exposes `aria-selected` and `aria-controls` to its panel. |
| Filters | Yes, for the overlay/panel — Radix Popover (desktop) or Dialog-based sheet (mobile), plus Radix Checkbox/RadioGroup for individual controls | `--surface-raised`, `--border-subtle`, `--action-primary`, `--text-muted`, `--status-danger-fg`, `--status-danger-bg`, `--focus-ring` | Filter panels presented as an overlay follow the dialog/popover focus-trap and Esc-to-close pattern; every control keeps a visible, programmatically associated label. |
| Data table | Partial — base `<table>` markup does not need Radix; row-selection checkboxes use Radix Checkbox | `--surface-raised`, `--surface-sunken`, `--border-subtle`, `--text-primary`, `--text-secondary`, `--text-muted`, `--action-primary`, `--status-danger-fg`, `--status-danger-bg`, `--focus-ring` | Native `<table>` markup with scoped column headers; sortable headers are keyboard-operable buttons exposing `aria-sort`. |
| Stat card | No | `--surface-raised`, `--text-primary`, `--text-secondary`, `--text-muted`, `--chart-positive`, `--chart-negative`, `--action-primary`, `--status-danger-fg` | The numeric value and its label form one accessible name/description pair, not conveyed by visual position alone. |
| Chart container | No — the charting library owns rendering; any legend-toggle control is a plain button | `--chart-series-1`, `--chart-series-2`, `--chart-series-3`, `--chart-series-4`, `--chart-series-5`, `--chart-series-6`, `--chart-positive`, `--chart-negative`, `--chart-axis-text`, `--chart-grid`, `--chart-legend-text`, `--chart-tooltip-bg`, `--chart-tooltip-text`, `--chart-tooltip-border`, `--chart-zero-line`, `--surface-raised`, `--focus-ring` | Chart data has a text alternative (adjacent table or summary); series are distinguishable by shape/pattern/label, not hue alone. |
| Empty state | No | `--surface-raised`, `--text-secondary`, `--text-muted`, `--action-primary` | Rendered in a labelled region (or `aria-live`) so it's announced when it replaces table/chart content, not silently invisible. |
| Error state | No | `--status-danger-fg`, `--status-danger-bg`, `--surface-raised`, `--text-secondary`, `--action-primary` | Rendered with `role=alert` or `aria-live=assertive` so the failure is announced without requiring the user to see it. |
| Loading state | No | `--surface-sunken`, `--border-subtle`, `--text-muted` | The region being replaced sets `aria-busy`/`aria-live=polite`, and any shimmer/spinner respects `prefers-reduced-motion`. |
| Mobile navigation | No — native `<nav>`/links cover it; Radix Toggle Group is optional, not required | `--surface-raised`, `--border-subtle`, `--action-primary`, `--text-muted`, `--focus-ring` | Wrapped in a `<nav>` landmark with an accessible label; the current destination is marked `aria-current=page`; each target meets the 44px minimum touch size (WCAG 2.2). |
| Dialogs | Yes — Radix Dialog for focus trap, `aria-modal`, and dismiss handling | `--surface-raised`, `--border-subtle`, `--text-primary`, `--text-secondary`, `--action-primary`, `--focus-ring`, `--status-danger-fg`, `--status-danger-bg` | Focus is trapped inside while open, Esc closes it, and focus returns to the triggering element on close. |

## Verification

- All 14 required components (buttons, inputs, select/combobox, badges,
  tabs, filters, data table, stat card, chart container, empty state,
  error state, loading state, mobile navigation, dialogs) have a row in
  both tables above — none added, none dropped.
- Every row in both tables references at least one token name copied
  verbatim from `token-inventory.md` (checked by eye against the
  inventory's token tables and its "untouched" chart-token list; no token
  name below was invented).
