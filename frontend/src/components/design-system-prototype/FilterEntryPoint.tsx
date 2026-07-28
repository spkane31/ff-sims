import { useState } from 'react';
import { FilterIcon } from './icons';
import { FOCUS_RING } from './focus-ring';

/**
 * Filter entry-point affordance for the top bar.
 *
 * responsive-rules.md Section 4 specifies the actual overlay/inline
 * behavior (bottom sheet below `lg`, inline panel at `lg`+) as "Radix
 * Dialog territory in Phase 2; out of scope to implement in this doc" —
 * so this component is intentionally just the entry-point button plus a
 * lightweight open/closed affordance, not a real filter panel.
 */
export default function FilterEntryPoint() {
  const [active, setActive] = useState(false);

  return (
    <button
      type="button"
      aria-pressed={active}
      aria-label="Filters"
      onClick={() => setActive((value) => !value)}
      className={`inline-flex min-h-11 min-w-11 items-center gap-2 rounded-md border px-3 text-sm font-medium ${FOCUS_RING}`}
      style={{
        borderColor: active ? 'var(--action-primary)' : 'var(--border-subtle)',
        backgroundColor: active
          ? 'var(--action-primary)'
          : 'var(--surface-raised)',
        color: active ? 'var(--action-on-primary)' : 'var(--text-secondary)',
      }}
      title="Filters (prototype affordance only)"
    >
      <FilterIcon className="h-4 w-4" />
      <span className="hidden sm:inline">Filters</span>
    </button>
  );
}
