/**
 * Minimal inline icon set for the production app shell.
 *
 * There is no icon library in this repo's own dependency tree for this
 * purpose (shadcn's own components use lucide-react, but the shell's nav
 * icons predate that and pair a color change with an actual shape change,
 * outline -> filled, satisfying the "never color alone" rule), so these are
 * small hand-written SVGs, carried over verbatim from the Phase 1 prototype
 * (`design-system-prototype/icons.tsx`).
 */

export interface IconProps {
  className?: string;
  active?: boolean;
}

export function OverviewIcon({ className, active }: IconProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      className={className}
      aria-hidden="true"
      fill={active ? 'currentColor' : 'none'}
      stroke="currentColor"
      strokeWidth={active ? 0 : 1.75}
    >
      <path d="M4 11.5 12 4l8 7.5V20a1 1 0 0 1-1 1h-4.5v-6h-5v6H5a1 1 0 0 1-1-1Z" />
    </svg>
  );
}

export function ScheduleIcon({ className, active }: IconProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      className={className}
      aria-hidden="true"
      fill={active ? 'currentColor' : 'none'}
      stroke="currentColor"
      strokeWidth={active ? 0 : 1.75}
    >
      <rect x="4" y="5" width="16" height="15" rx="2" />
      <path
        d="M4 10h16M8 3v4M16 3v4"
        stroke={active ? 'var(--action-on-primary)' : 'currentColor'}
        strokeWidth="1.75"
        strokeLinecap="round"
        fill="none"
      />
    </svg>
  );
}

export function PlayersIcon({ className, active }: IconProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      className={className}
      aria-hidden="true"
      fill={active ? 'currentColor' : 'none'}
      stroke="currentColor"
      strokeWidth={active ? 0 : 1.75}
    >
      <circle cx="9" cy="8" r="3" />
      <path d="M3 20c0-3.31 2.69-6 6-6s6 2.69 6 6" />
      <circle cx="17" cy="9" r="2.4" opacity={active ? 0.7 : 1} />
      <path d="M15.5 13.2c2.6.4 4.5 2.6 4.5 5.3" opacity={active ? 0.7 : 1} />
    </svg>
  );
}

export function TeamsIcon({ className, active }: IconProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      className={className}
      aria-hidden="true"
      fill={active ? 'currentColor' : 'none'}
      stroke="currentColor"
      strokeWidth={active ? 0 : 1.75}
    >
      <path d="M12 3.5 5 6v5.2c0 4.6 3 8.2 7 9.3 4-1.1 7-4.7 7-9.3V6l-7-2.5Z" />
    </svg>
  );
}

export function MoreIcon({ className, active }: IconProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      className={className}
      aria-hidden="true"
      fill={active ? 'currentColor' : 'none'}
      stroke="currentColor"
      strokeWidth={active ? 0 : 1.75}
    >
      <rect x="4" y="4" width="7" height="7" rx="1.2" />
      <rect x="13" y="4" width="7" height="7" rx="1.2" />
      <rect x="4" y="13" width="7" height="7" rx="1.2" />
      <rect x="13" y="13" width="7" height="7" rx="1.2" />
    </svg>
  );
}

export function ChevronDownIcon({ className }: IconProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      className={className}
      aria-hidden="true"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="m6 9 6 6 6-6" />
    </svg>
  );
}

export function SunIcon({ className }: IconProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      className={className}
      aria-hidden="true"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.75}
      strokeLinecap="round"
    >
      <circle cx="12" cy="12" r="4" />
      <path d="M12 2v2.5M12 19.5V22M4.2 4.2l1.8 1.8M18 18l1.8 1.8M2 12h2.5M19.5 12H22M4.2 19.8l1.8-1.8M18 6l1.8-1.8" />
    </svg>
  );
}

export function MoonIcon({ className }: IconProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      className={className}
      aria-hidden="true"
      fill="currentColor"
      stroke="none"
    >
      <path d="M20.5 14.5A8.5 8.5 0 0 1 9.5 3.5a8.5 8.5 0 1 0 11 11Z" />
    </svg>
  );
}

export function SystemThemeIcon({ className }: IconProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      className={className}
      aria-hidden="true"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.75}
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <rect x="3" y="4.5" width="18" height="12" rx="1.5" />
      <path d="M8.5 20h7M12 16.5V20" />
    </svg>
  );
}
