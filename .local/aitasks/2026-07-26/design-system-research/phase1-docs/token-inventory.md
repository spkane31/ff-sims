# Token inventory — Phase 1, Task 1

Source of truth: `frontend/src/styles/globals.css`. All new tokens are
defined in oklch(). Contrast ratios below were computed with a throwaway
Node script (oklch → linear sRGB via the standard OKLab matrices → WCAG
relative luminance → contrast ratio), not eyeballed. The script lived at
`/private/tmp/.../scratchpad/contrast-final.mjs` and was deleted after use
(not committed).

Method note: a few colors at high chroma/lightness combinations
(e.g. `--action-primary`, `--focus-ring`, the `--status-*` pairs) fall
slightly outside the sRGB gamut at the given oklch coordinates. The script
clamps out-of-gamut linear-RGB channels to `[0, 1]` before computing
luminance, which is a reasonable proxy for what a browser's gamut mapping
does but not identical to the CSS Color 4 algorithm. Rows affected by this
are marked "(gamut-clamped estimate)" below; all of them still clear their
minimum with enough margin (smallest is 4.52:1 against a 3:1 minimum, or
4.91:1 against a 4.5:1 minimum) that the approximation error is not a
concern.

## 1a. Manual theme override layer

`.light` / `.dark` classes on `<html>` (or a wrapping element) set
`color-scheme` and carry a full mirror of the semantic tokens below,
including the seven pre-existing `--chart-*` tokens (copied verbatim from
`:root`/the dark media block — see "Resolved gap" at the bottom for
history). They are placed *after* the `@media (prefers-color-scheme: dark)` block in
source order so that, at equal CSS specificity (`:root` pseudo-class vs.
`.light`/`.dark` class both score `(0,1,0)`), the manual class wins
regardless of OS preference. No class on `<html>` means "follow OS
preference" (unchanged existing behavior).

## 1b. Semantic tokens

### Surfaces

| Token | Purpose | Light | Dark |
|---|---|---|---|
| `--surface-canvas` | App background | `oklch(0.98 0.004 250)` | `oklch(0.19 0.01 255)` |
| `--surface-raised` | Cards/panels above canvas | `oklch(1 0 0)` | `oklch(0.24 0.012 255)` |
| `--surface-sunken` | Recessed wells/inputs | `oklch(0.965 0.004 250)` | `oklch(0.16 0.01 255)` |
| `--border-subtle` | Low-emphasis dividers | `oklch(0.90 0.006 250)` | `oklch(0.32 0.012 255)` |
| `--border-strong` | High-emphasis dividers | `oklch(0.82 0.008 250)` | `oklch(0.42 0.014 255)` |

Not flagged "must pass" — no computed ratios required by the brief.

### Text

| Token | Purpose | Light | Dark |
|---|---|---|---|
| `--text-primary` | Body/heading text | `oklch(0.24 0.02 255)` | `oklch(0.96 0.004 255)` |
| `--text-secondary` | De-emphasized text | `oklch(0.38 0.015 255)` | `oklch(0.85 0.008 255)` |
| `--text-muted` | Least-emphasis text | `oklch(0.55 0.012 255)` | `oklch(0.65 0.012 255)` |
| `--text-inverse` | Text on inverted/filled surfaces | `oklch(0.98 0.004 250)` | `oklch(0.20 0.02 255)` |

**Must-pass pairs (text-primary, 4.5:1 normal text):**

| Pair | Mode | Ratio | Min | Result |
|---|---|---|---|---|
| text-primary vs surface-canvas | light | 15.53:1 | 4.5:1 | PASS |
| text-primary vs surface-raised | light | 16.44:1 | 4.5:1 | PASS |
| text-primary vs surface-canvas | dark | 16.44:1 | 4.5:1 | PASS |
| text-primary vs surface-raised | dark | 14.64:1 | 4.5:1 | PASS |

No adjustment needed — brief's starting values already clear 4.5:1 by a
wide margin in both modes.

### Action accent

| Token | Purpose | Light | Dark |
|---|---|---|---|
| `--action-primary` | Primary button/link fill | `oklch(0.53 0.18 250)` **(adjusted, see below)** | `oklch(0.68 0.17 250)` |
| `--action-primary-hover` | Hover state | `oklch(0.50 0.19 250)` | `oklch(0.73 0.16 250)` |
| `--action-primary-active` | Active/pressed state | `oklch(0.45 0.19 250)` | `oklch(0.78 0.15 250)` |
| `--action-on-primary` | Text/icon on action-primary fill | `oklch(0.98 0.01 250)` | `oklch(0.15 0.02 250)` |

**Adjustment made:** the brief's starting light value for
`--action-primary` was `oklch(0.55 0.18 250)`, which measured 4.52:1
against `--action-on-primary` — technically passing 4.5:1 but with only a
0.4% margin, and this pair is one of the "gamut-clamped estimate" cases
where the true browser-rendered ratio could differ slightly from the
clamped approximation. Per the brief's instruction to hold H/C constant
and move L until the pair clears its minimum with confidence, L was
lowered from `0.55` to `0.53` (darkening it slightly, same direction as
`-hover`/`-active` which are already darker), giving 4.91:1 — a safer
margin. `-hover`/`-active`/dark values were left as specified since they
already had comfortable margins or aren't part of a must-pass pair
themselves.

**Must-pass pair (action-on-primary vs action-primary, 4.5:1):**

| Pair | Mode | Ratio | Min | Result |
|---|---|---|---|---|
| action-on-primary vs action-primary | light | 4.91:1 | 4.5:1 | PASS (gamut-clamped estimate) |
| action-on-primary vs action-primary | dark | 6.84:1 | 4.5:1 | PASS |

### Focus

| Token | Purpose | Light | Dark |
|---|---|---|---|
| `--focus-ring` | Focus outline color | `oklch(0.55 0.18 250)` | `oklch(0.75 0.18 250)` |

**Must-pass pairs (focus-ring visible against both surfaces, 3:1):**

| Pair | Mode | Ratio | Min | Result |
|---|---|---|---|---|
| focus-ring vs surface-canvas | light | 4.52:1 | 3:1 | PASS (gamut-clamped estimate) |
| focus-ring vs surface-raised | light | 4.79:1 | 3:1 | PASS (gamut-clamped estimate) |
| focus-ring vs surface-canvas | dark | 7.99:1 | 3:1 | PASS (gamut-clamped estimate) |
| focus-ring vs surface-raised | dark | 7.11:1 | 3:1 | PASS (gamut-clamped estimate) |

No adjustment needed — all four pairs clear 3:1 with comfortable margin
(smallest is 4.52:1, ~50% above the minimum).

### Status (reserved for meaning, never decoration)

| Token | Purpose | Light | Dark |
|---|---|---|---|
| `--status-success-fg` | Success text/icon | `oklch(0.35 0.12 145)` | `oklch(0.85 0.12 145)` |
| `--status-success-bg` | Success fill | `oklch(0.95 0.05 145)` | `oklch(0.20 0.05 145)` |
| `--status-warning-fg` | Warning text/icon | `oklch(0.35 0.13 80)` | `oklch(0.85 0.13 80)` |
| `--status-warning-bg` | Warning fill | `oklch(0.95 0.06 80)` | `oklch(0.20 0.06 80)` |
| `--status-danger-fg` | Danger text/icon | `oklch(0.40 0.18 25)` | `oklch(0.80 0.18 25)` |
| `--status-danger-bg` | Danger fill | `oklch(0.95 0.05 25)` | `oklch(0.20 0.05 25)` |
| `--status-info-fg` | Info text/icon | `oklch(0.38 0.12 250)` | `oklch(0.83 0.12 250)` |
| `--status-info-bg` | Info fill | `oklch(0.95 0.04 250)` | `oklch(0.20 0.04 250)` |

Dark values were derived by lightening `-fg` and darkening `-bg` from the
light values by a comparable delta to the surface/text dark shift (roughly
mirroring L around the midpoint, keeping H/C constant), per the brief's
guidance.

**Must-pass pairs (fg vs bg, 4.5:1):**

| Pair | Mode | Ratio | Min | Result |
|---|---|---|---|---|
| status-success-fg vs -bg | light | 9.40:1 | 4.5:1 | PASS (gamut-clamped estimate) |
| status-warning-fg vs -bg | light | 9.73:1 | 4.5:1 | PASS (gamut-clamped estimate) |
| status-danger-fg vs -bg | light | 7.84:1 | 4.5:1 | PASS (gamut-clamped estimate) |
| status-info-fg vs -bg | light | 8.56:1 | 4.5:1 | PASS (gamut-clamped estimate) |
| status-success-fg vs -bg | dark | 11.80:1 | 4.5:1 | PASS |
| status-warning-fg vs -bg | dark | 11.35:1 | 4.5:1 | PASS (gamut-clamped estimate) |
| status-danger-fg vs -bg | dark | 7.98:1 | 4.5:1 | PASS (gamut-clamped estimate) |
| status-info-fg vs -bg | dark | 10.55:1 | 4.5:1 | PASS |

No adjustment needed — all 8 pairs clear 4.5:1 comfortably (smallest is
7.84:1, ~74% above the minimum).

## 1c. Chart series + data-meaning tokens

Existing `--chart-axis-text`, `--chart-grid`, `--chart-legend-text`,
`--chart-tooltip-bg`, `--chart-tooltip-text`, `--chart-tooltip-border`,
`--chart-zero-line` are untouched (not repeated here). Added:

| Token | Purpose | Light | Dark |
|---|---|---|---|
| `--chart-series-1` | Qualitative series 1 (blue) | `oklch(0.60 0.15 250)` | `oklch(0.72 0.15 250)` |
| `--chart-series-2` | Qualitative series 2 (orange) | `oklch(0.70 0.15 70)` | `oklch(0.82 0.15 70)` |
| `--chart-series-3` | Qualitative series 3 (green) | `oklch(0.65 0.15 150)` | `oklch(0.77 0.15 150)` |
| `--chart-series-4` | Qualitative series 4 (magenta/purple) | `oklch(0.55 0.20 320)` | `oklch(0.67 0.20 320)` |
| `--chart-series-5` | Qualitative series 5 (yellow-green) | `oklch(0.75 0.15 100)` | `oklch(0.87 0.15 100)` |
| `--chart-series-6` | Qualitative series 6 (neutral baseline) | `oklch(0.50 0.05 250)` | `oklch(0.62 0.05 250)` |
| `--chart-positive` | Data-meaning: positive swing (not a status color) | `oklch(0.55 0.15 145)` | `oklch(0.67 0.15 145)` |
| `--chart-negative` | Data-meaning: negative swing (not a status color) | `oklch(0.55 0.18 25)` | `oklch(0.67 0.18 25)` |

**Dark delta method:** the pre-existing `--chart-*` tokens are hex-coded
foreground/background pairs whose light→dark deltas vary wildly (from
roughly +0.56 to -0.72 in oklab L, converted from hex) because several of
them invert a text-color role rather than lighten a mark color, so no
single delta from that set was usable as-is. Instead, an L delta of +0.12
was applied uniformly to the new series/positive/negative tokens (H and C
held constant), matching the delta the brief itself uses for
`--action-primary`'s own light→dark shift (0.55 → 0.68, i.e. +0.13) —
the closest existing analogue for "a saturated accent color that needs to
read clearly against a dark canvas." None of these are "must pass" pairs
per the brief, so no contrast ratios are reported for them; they were
chosen only to stay under `L ≈ 0.90` (avoiding washout) while remaining
visibly lighter than their light-mode counterparts.

## Additional verified pairs (fix round 2)

Found during Phase 1's final cross-task review: `--text-muted` on
`--surface-sunken` measures **4.38:1 in light mode** (fails the 4.5:1
minimum for normal text, SC 1.4.3) and **6.00:1 in dark mode** (passes).
`--text-muted` was never flagged "must pass" against `--surface-sunken`
specifically — only `--text-primary` pairs were in the brief's must-pass
list — so this combination slipped through Task 1 uncaught. `--text-muted`
itself is not being changed, since it passes everywhere else it's used
(against `--surface-raised` and `--surface-canvas`); this is a
combination-specific gap, not a token-value problem.

**Resolution:** do not use `--text-muted` on `--surface-sunken`. Use
`--text-secondary` on `--surface-sunken` instead (9.04:1 light / comfortably
passing dark), which was the fix applied to
`FeaturedMatchupCard.tsx`'s team W-L record row.

| Pair | Mode | Ratio | Min | Result |
|---|---|---|---|---|
| text-muted vs surface-sunken | light | 4.38:1 | 4.5:1 | FAIL |
| text-muted vs surface-sunken | dark | 6.00:1 | 4.5:1 | PASS |
| text-secondary vs surface-sunken | light | 9.04:1 | 4.5:1 | PASS |

## Resolved gap (fix round 1)

The initial version of this task left the seven pre-existing `--chart-*`
tokens (axis/grid/legend/tooltip/zero-line) out of the `.light`/`.dark`
manual-override classes, reasoning that the brief's "do not duplicate the
existing `--chart-*` tokens" instruction (which is about not re-listing
them as if new in section 1b/1c) also covered the override mirror. Review
flagged this as inconsistent: Task 6's league-overview prototype reuses
these exact chart tokens, and without them in `.light`/`.dark` a manual
theme toggle would leave chart colors following the OS preference while
everything else in the UI followed the toggle. Per the task author's
resolution, the seven tokens were added to both `.light` and `.dark`,
copied verbatim from the existing `:root` values (light) and the existing
`@media (prefers-color-scheme: dark) { :root { ... } }` values (dark) — no
new color decisions or contrast checks, since these tokens were never
flagged "must pass." `.light`/`.dark` now mirror every token in the file.
