import { useEffect, useState } from "react";
import { SunIcon, MoonIcon, SystemThemeIcon } from "./icons";
import { FOCUS_RING } from "@/components/design-system/focus-ring";

export type ThemeMode = "system" | "light" | "dark";

const NEXT_MODE: Record<ThemeMode, ThemeMode> = {
  system: "light",
  light: "dark",
  dark: "system",
};

const MODE_META: Record<ThemeMode, { label: string; Icon: typeof SunIcon }> = {
  system: { label: "System", Icon: SystemThemeIcon },
  light: { label: "Light", Icon: SunIcon },
  dark: { label: "Dark", Icon: MoonIcon },
};

/**
 * Three-state theme control: system -> light -> dark -> system, applied to
 * `<html>` (not a local wrapper) so it affects every page's `dark:`
 * utilities globally, and persisted to localStorage so `_document.tsx`'s
 * blocking init script picks it up on the next load without a flash.
 */
export default function ThemeToggle() {
  const [mode, setMode] = useState<ThemeMode>("system");

  useEffect(() => {
    const root = document.documentElement;
    if (root.classList.contains("dark")) {
      setMode("dark");
    } else if (root.classList.contains("light")) {
      setMode("light");
    }
  }, []);

  const applyMode = (next: ThemeMode) => {
    const root = document.documentElement;
    root.classList.remove("light", "dark");
    if (next === "light" || next === "dark") {
      root.classList.add(next);
      localStorage.setItem("theme", next);
    } else {
      localStorage.removeItem("theme");
    }
    setMode(next);
  };

  const { label, Icon } = MODE_META[mode];

  return (
    <button
      type="button"
      onClick={() => applyMode(NEXT_MODE[mode])}
      aria-pressed={mode !== "system"}
      aria-label={`Theme: ${label}. Activate to switch to ${MODE_META[NEXT_MODE[mode]].label.toLowerCase()}.`}
      className={`inline-flex min-h-11 min-w-11 items-center gap-2 rounded-md border px-3 text-sm font-medium transition-colors motion-reduce:transition-none ${FOCUS_RING}`}
      style={{
        borderColor: "var(--border-subtle)",
        color: "var(--text-secondary)",
        backgroundColor: "var(--surface-raised)",
      }}
      title="Cycle theme: system -> light -> dark"
    >
      <Icon className="h-5 w-5" />
      <span className="hidden sm:inline">Theme: {label}</span>
    </button>
  );
}
