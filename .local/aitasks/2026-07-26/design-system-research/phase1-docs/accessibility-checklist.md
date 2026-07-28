# Accessibility checklist — Phase 1, Task 4

Target conformance level: WCAG 2.2 AA. Use this checklist against any
component or page built on this system — Task 2's component/state matrix
and Task 3's responsive rules are the two docs it cross-references most,
since focus/contrast/target-size/non-color decisions were already made
there and are not re-litigated here.

Every item cites the specific WCAG 2.2 success criterion (SC) number where
one exists. A small number of items (motion handling, some ARIA
authoring-practice requirements) don't map to a numbered SC at the AA
level — those are marked explicitly rather than given an invented number.

## 1. Keyboard reachability and operability

- [ ] Every interactive control (buttons, links, inputs, tabs, table row
      actions, chart legend toggles, filter chips, dialog triggers) can
      receive focus via Tab/Shift+Tab, in an order matching visual/reading
      order. (SC 2.1.1 Keyboard; SC 2.4.3 Focus Order)
- [ ] Every interactive control can be activated using only the keyboard
      (Enter/Space for buttons; arrow keys for roving-tabindex widgets
      such as Tabs/Select/Combobox, per their Radix primitive's default
      behavior). (SC 2.1.1 Keyboard)
- [ ] No component traps keyboard focus unintentionally. Dialogs and
      filter overlays intentionally trap focus while open (expected
      modal behavior) but must release it on close. (SC 2.1.2 No
      Keyboard Trap)
- [ ] Opening a dialog/filter overlay moves focus into it; closing it
      (Esc, close button, backdrop click) returns focus to the
      triggering control. (Supports SC 2.1.2 and SC 2.4.3; this specific
      open/close focus-return behavior is an ARIA Authoring Practices
      requirement, not itself a separately numbered SC.)
- [ ] Any custom control that isn't a native element or Radix primitive
      (e.g. a bespoke chart-legend toggle) responds to both click and
      keyboard activation — no pointer-only `onClick` handler without a
      corresponding keyboard path. (SC 2.1.1 Keyboard)
- [ ] A sticky/fixed element (top bar, bottom nav) never fully obscures
      the currently focused control while tabbing through the page. (SC
      2.4.11 Focus Not Obscured (Minimum) — new in WCAG 2.2)

## 2. Visible focus indicator

- [ ] Every focusable control shows a visible focus indicator on
      keyboard focus (`:focus-visible`), using the `--focus-ring` token
      per Task 1's token inventory — never suppressed via `outline: none`
      without a replacement indicator. (SC 2.4.7 Focus Visible)
- [ ] The focus indicator has at least 3:1 contrast against the adjacent
      surface color, in both light and dark mode. Task 1's inventory
      already verified `--focus-ring` clears 3:1 against both
      `--surface-canvas` and `--surface-raised` in both modes (smallest
      margin 4.52:1, gamut-clamped estimate) — reuse `--focus-ring`
      as-is rather than defining a new focus color per component. (SC
      1.4.11 Non-text Contrast)
- [ ] The focus indicator is not clipped or hidden by `overflow:
      hidden`/`overflow: auto` on a parent — a common failure mode on
      horizontally-scrollable tables (Task 3, Section 3) and chip rows.
      (Supports SC 2.4.7; not itself a separately numbered SC.)

## 3. Programmatic labels and names

- [ ] Every control has a programmatically determinable accessible name
      — a visible `<label for>`, `aria-label`, or `aria-labelledby` —
      never a placeholder attribute alone. (SC 4.1.2 Name, Role, Value;
      SC 1.3.1 Info and Relationships; SC 3.3.2 Labels or Instructions)
- [ ] Icon-only controls (filter entry-point button, dialog close
      button, table row action icons, chart legend swatch toggles) carry
      an `aria-label` describing the action, not just a `title`
      attribute. (SC 4.1.2 Name, Role, Value)
- [ ] Custom widgets (Select/combobox, Tabs, Filters overlay) expose the
      correct ARIA role and live state — `aria-selected`,
      `aria-expanded`, `aria-current`, `aria-invalid`, `aria-busy` as
      applicable per Task 2's component/state matrix — not just visual
      styling. (SC 4.1.2 Name, Role, Value)
- [ ] Form field errors are linked to their field via
      `aria-describedby`, not conveyed only by adjacent text with no
      programmatic association. (SC 1.3.1 Info and Relationships; SC
      3.3.1 Error Identification)
- [ ] Grouped/related controls (a filter chip group, a toggle-button
      group) are exposed as a group with an accessible group label
      (`<fieldset>`/`<legend>`, or `role="group"` + `aria-labelledby`).
      (SC 1.3.1 Info and Relationships)

## 4. Text contrast

- [ ] Normal-size body/UI text meets at least 4.5:1 contrast against its
      background. Reuse Task 1's verified `--text-primary` pairs against
      `--surface-canvas`/`--surface-raised` (15.53:1–16.44:1 across both
      modes) rather than introducing a new text color. (SC 1.4.3
      Contrast (Minimum))
- [ ] Large-scale text (≥24px regular weight, or ≥18.66px/14pt bold —
      WCAG's definition of "large text") meets at least 3:1 if a
      component ever renders `--text-secondary`/`--text-muted` at a size
      qualifying for the large-text tier. (SC 1.4.3 Contrast (Minimum))
- [ ] Disabled-state text is exempt from the contrast minimum under
      WCAG's own inactive-component exception — but confirm any "muted"
      styling is only applied to a truly disabled (non-operable)
      control, not used to fake a low-emphasis but still-interactive
      one. (Relates to the SC 1.4.3 exception for inactive UI
      components; not a separate SC.)

## 5. Non-text / UI-component contrast

- [ ] Every UI-component boundary needed to identify it — input borders,
      outline-style button borders, checkbox/radio boundaries, data-table
      dividers that carry structural meaning — meets 3:1 against
      adjacent color(s). Reuse `--border-strong` (not `--border-subtle`,
      which Task 1's inventory does not flag as must-pass) wherever a
      divider needs to clear this bar. (SC 1.4.11 Non-text Contrast)
- [ ] Graphical objects required to understand content — chart axis
      lines, chart data marks/points, status icons — meet 3:1 against
      their background, unless the graphic is purely decorative or is a
      logo/brand mark. (SC 1.4.11 Non-text Contrast)
- [ ] State changes conveyed through a border/outline color shift alone
      (e.g. input `--border-subtle` → `--border-strong` on hover, a
      selected tab's underline) still meet 3:1 in the "on" state. (SC
      1.4.11 Non-text Contrast)

## 6. Target size minimum

- [ ] Every interactive control not covered by WCAG 2.2's exceptions
      (spacing, equivalent, inline, essential, or already governed by
      another SC) has a minimum 44×44px hit area — exceeding SC 2.5.8's
      24×24px floor, per the 44px value already committed to in the
      approved plan and documented in Task 3's responsive-rules.md,
      Section 5. Achieved by padding a smaller visual icon out to the
      44px hit area, not by enlarging the icon itself. (SC 2.5.8 Target
      Size Minimum — new in WCAG 2.2)
- [ ] Applies at minimum to: bottom nav items, top-bar primary nav links
      (mobile), the filter entry-point button and league-switcher
      trigger, table row action icons (at every breakpoint they render,
      not just mobile), filter chips/toggles, tab controls, dialog close
      buttons, and chart legend items that double as series toggles —
      matching the enumerated list in Task 3's responsive-rules.md,
      Section 5. (SC 2.5.8 Target Size Minimum)
- [ ] Small targets that rely on the "spacing" exception instead of being
      enlarged to 44px (e.g. an inline text link within a paragraph) are
      explicitly checked against that exception's spacing math, rather
      than assumed exempt by default. (SC 2.5.8 Target Size Minimum —
      spacing exception)

## 7. Non-color status/selected indication

- [ ] No status, selected, active, or error state is conveyed by a
      color/hue change alone; each pairs the color change with at least
      one non-color cue (icon, underline/border, font-weight change,
      text label), per Task 3's responsive-rules.md, Section 6 pairings.
      (SC 1.4.1 Use of Color)
- [ ] Badges (`--status-*-fg`/`--status-*-bg` pairs) always carry a text
      label or icon alongside their color, since badges are typically
      non-interactive and can't rely on a hover tooltip to disambiguate
      for color-blind users. (SC 1.4.1 Use of Color)
- [ ] Current-page, current-tab, and selected-row indicators pair their
      color/token change with a semantic attribute (`aria-current="page"`,
      `aria-selected`, checkbox checked state) so the non-color cue is
      also programmatically exposed, not only visually present. (SC 1.4.1
      Use of Color; SC 4.1.2 Name, Role, Value)
- [ ] Status messages that appear without a page reload (form
      validation, save confirmation, sync/error banners) are exposed via
      `role="alert"` or an `aria-live` region — not conveyed by a color
      change alone that a screen-reader user would miss entirely. (SC
      4.1.3 Status Messages)

## 8. `prefers-reduced-motion` handling

- [ ] Any non-essential animation or transition (skeleton shimmer, page
      transitions, chart entrance animation, dialog/sheet open-close
      motion) is reduced or removed under `@media
      (prefers-reduced-motion: reduce)` — either dropped entirely or
      replaced with an instant/cross-fade equivalent.
- [ ] Auto-updating or auto-advancing content that isn't essential and
      persists longer than 5 seconds (e.g. an auto-rotating stat
      carousel, if one is ever built) provides a pause/stop/hide
      control, independent of the user's motion preference. (SC 2.2.2
      Pause, Stop, Hide)
- [ ] No content flashes more than three times per second. (SC 2.3.1
      Three Flashes or Below Threshold)
- [ ] **Note on SC coverage:** WCAG 2.2 has no AA-level success criterion
      that directly mandates honoring `prefers-reduced-motion`. The
      closest numbered criterion, SC 2.3.3 Animation from Interactions,
      is AAA (outside the AA conformance target this system otherwise
      aims for) and only covers interaction-triggered motion, not motion
      in general. Treat the two `prefers-reduced-motion` items above as
      this design system's own best-practice requirement, not as
      something the AA target itself requires — flagged here so no SC
      number is misattributed to them.

## Verification

- Every checklist item that corresponds to a numbered WCAG 2.2 success
  criterion cites that number inline (2.1.1, 2.1.2, 2.4.3, 2.4.7, 2.4.11,
  4.1.2, 1.3.1, 3.3.1, 3.3.2, 1.4.3, 1.4.11, 2.5.8, 1.4.1, 4.1.3, 2.2.2,
  2.3.1). The handful of items without a numbered SC (open/close focus
  return, focus-indicator clipping, the disabled-text contrast
  exception, and `prefers-reduced-motion` handling itself) say so
  explicitly rather than citing an invented or approximate number.
- Target-size item (Section 6) cites SC 2.5.8 and states both the SC's
  24px floor and the plan's chosen 44px value, consistent with Task 3's
  responsive-rules.md, Section 5, which cites the same SC number and
  values.
- Focus-ring item (Section 2) references `--focus-ring` and its verified
  contrast ratios from Task 1's token-inventory.md rather than
  redefining focus-ring behavior or re-deriving new ratios.
- Text contrast (Section 4) and non-text/UI contrast (Section 5) items
  cite the 4.5:1 (SC 1.4.3) and 3:1 (SC 1.4.11) minimums respectively,
  matching Task 1's token-inventory.md must-pass thresholds.
