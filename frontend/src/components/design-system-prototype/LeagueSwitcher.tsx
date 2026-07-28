import { useState, useRef, useId, type KeyboardEvent } from 'react';
import { ChevronDownIcon } from './icons';

interface LeagueSwitcherProps {
  leagueName: string;
}

const FOCUS_RING =
  'outline-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]';

/**
 * League context/switcher trigger for the top bar.
 *
 * Per task-5-brief.md this is a "mock league name + a non-functional
 * dropdown affordance" — no real league-switching logic. It still behaves
 * like a real disclosure control (keyboard operable, Escape closes,
 * aria-expanded) so it exercises the same interaction shape a future real
 * switcher would use.
 */
export default function LeagueSwitcher({ leagueName }: LeagueSwitcherProps) {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const menuId = useId();

  const close = () => {
    setOpen(false);
    triggerRef.current?.focus();
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Escape' && open) {
      event.stopPropagation();
      close();
    }
  };

  return (
    <div className="relative" onKeyDown={handleKeyDown}>
      <button
        ref={triggerRef}
        type="button"
        aria-haspopup="true"
        aria-expanded={open}
        aria-controls={menuId}
        onClick={() => setOpen((value) => !value)}
        className={`flex min-h-11 items-center gap-2 rounded-md border px-3 text-sm font-semibold ${FOCUS_RING}`}
        style={{
          borderColor: 'var(--border-subtle)',
          backgroundColor: 'var(--surface-raised)',
          color: 'var(--text-primary)',
        }}
      >
        <span className="max-w-[10rem] truncate sm:max-w-xs">
          {leagueName}
        </span>
        <ChevronDownIcon className="h-4 w-4 shrink-0" />
      </button>

      {open && (
        <div
          id={menuId}
          role="menu"
          aria-label="League switcher (prototype only, non-functional)"
          className="absolute left-0 z-20 mt-1 w-56 rounded-md border p-1 shadow-lg"
          style={{
            borderColor: 'var(--border-subtle)',
            backgroundColor: 'var(--surface-raised)',
          }}
        >
          <div
            role="menuitem"
            aria-disabled="true"
            className="rounded px-3 py-2 text-sm font-medium"
            style={{ color: 'var(--text-primary)' }}
          >
            {leagueName}
          </div>
          <p
            className="px-3 pb-1 pt-1 text-xs"
            style={{ color: 'var(--text-muted)' }}
          >
            Only one mock league — switching isn&apos;t wired up yet.
          </p>
        </div>
      )}
    </div>
  );
}
