import { SunIcon, MoonIcon, SystemThemeIcon } from './icons';

export type ThemeMode = 'system' | 'light' | 'dark';

interface ThemeToggleProps {
  mode: ThemeMode;
  onChange: (mode: ThemeMode) => void;
  className?: string;
}

const NEXT_MODE: Record<ThemeMode, ThemeMode> = {
  system: 'light',
  light: 'dark',
  dark: 'system',
};

const MODE_META: Record<
  ThemeMode,
  { label: string; Icon: typeof SunIcon }
> = {
  system: { label: 'System', Icon: SystemThemeIcon },
  light: { label: 'Light', Icon: SunIcon },
  dark: { label: 'Dark', Icon: MoonIcon },
};

const FOCUS_RING =
  'outline-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]';

/**
 * Three-state theme control: system -> light -> dark -> system.
 *
 * "system" applies no class (falls back to OS preference, per Task 1's
 * token inventory: "No class on <html> means follow OS preference").
 * "light"/"dark" apply the matching class, which the caller (AppShell)
 * puts on its top-level wrapper element.
 */
export default function ThemeToggle({
  mode,
  onChange,
  className = '',
}: ThemeToggleProps) {
  const { label, Icon } = MODE_META[mode];

  return (
    <button
      type="button"
      onClick={() => onChange(NEXT_MODE[mode])}
      aria-pressed={mode !== 'system'}
      className={`inline-flex min-h-11 min-w-11 items-center gap-2 rounded-md border px-3 text-sm font-medium transition-colors ${FOCUS_RING} ${className}`}
      style={{
        borderColor: 'var(--border-subtle)',
        color: 'var(--text-secondary)',
        backgroundColor: 'var(--surface-raised)',
      }}
      title="Cycle theme: system -> light -> dark"
    >
      <Icon className="h-5 w-5" />
      <span className="hidden sm:inline">Theme: {label}</span>
    </button>
  );
}
