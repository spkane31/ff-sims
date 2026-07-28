/**
 * Shared focus-ring Tailwind class string for the design-system shell
 * prototype.
 *
 * Previously copy-pasted verbatim into every component/page that needed a
 * visible keyboard focus indicator. Centralized here so the class string
 * (and the `--focus-ring` token it references, per Task 1's token
 * inventory and accessibility-checklist.md Section 2) only needs to be
 * defined/updated in one place.
 */
export const FOCUS_RING =
  'outline-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]';
