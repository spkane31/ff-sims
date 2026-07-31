# Design-system Phase 2 + 3 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Install the real shadcn/ui + Radix component library, replace the app's shared nav/chrome globally with a production `AppShell`, and migrate the League overview page's content onto the new system — with zero behavior or API change anywhere.

**Architecture:** shadcn/ui CLI-generated primitives (`frontend/src/components/ui/`) aliased to this repo's existing Phase-1 CSS tokens; a router-driven `AppShell` centralized in `_app.tsx` (not per-page) so every route inherits it for free; League overview's six existing sections regrouped into "This season" / "League history" bands using new `DataTable`/`StatCard`/`Card` components, with all current data-fetching, sorting, and filtering logic preserved verbatim.

**Tech Stack:** Next.js 15 (Pages Router), React 19, TypeScript, Tailwind v4, shadcn/ui CLI, Radix primitives, existing `recharts`/service-hook layer (untouched).

## Global Constraints

- No backend/API changes. `leaguesService`, `teamsService`, `scheduleService`, and the hooks built on them (`useTeams`, `useSchedule`, `useLeague`, `useLeagues`) are consumed as-is, never modified.
- No product-behavior change anywhere: every sortable column, computed stat, filter, loading state, and error state that exists today must work identically after migration. Where this plan quotes existing logic verbatim, copy it exactly — do not "improve" it as part of this migration.
- The 12 non-League-overview production pages keep their current inner content. Only their chrome changes (via the shell swap in Task 6).
- Nav destinations must match `frontend/src/components/Header.tsx`'s current logic exactly — no destinations added or removed. In particular: `/admin` and `/sleeper/transactions` are not linked from nav today and must not become linked now.
- No new sub-routes. No new information/metrics beyond what League overview shows today.
- No test framework is introduced. This repo's frontend has no test runner today, and this migration is refactor-flavored (preserve behavior, swap implementation) rather than new-feature work, so per this project's own testing convention, verification per task is `npm run build` + `npm run lint` + a manual browser check — not automated tests.
- Build only the components this sub-project actually needs (see Task 2-4). Tabs, filters, dialogs, chart container, and generic combobox/inputs beyond the league switcher are explicitly out of scope until a later page needs them.
- shadcn/ui's own generated CSS variables (`--background`, `--foreground`, `--primary`, `--card`, `--border`, `--ring`, etc.) must be aliased to this repo's existing tokens (`--surface-canvas`, `--text-primary`, `--action-primary`, `--surface-raised`, `--border-subtle`, `--focus-ring`) rather than left at their own defaults — one token system, not two.
- Commit at the end of each task as usual (`git add` + `git commit`) — the controller running this plan will squash everything back to unstaged working-tree changes at the very end; per-task commits during implementation are still expected and needed for the review process.

---

### Task 1: Install shadcn/ui + Radix, unify theming, add theme persistence

**Files:**
- Modify: `frontend/package.json`, `frontend/package-lock.json` (new dependencies from shadcn CLI + whatever Radix packages it pulls in for the components added in later tasks — CLI-managed, do not hand-edit versions)
- Create: `frontend/components.json` (shadcn CLI config)
- Create: `frontend/src/lib/utils.ts` (shadcn's standard `cn()` class-merge helper — this is what the CLI itself generates; do not hand-write a different one)
- Modify: `frontend/src/styles/globals.css`
- Create: `frontend/src/pages/_document.tsx` (does not exist today — default Next.js document is used)
- Modify: `frontend/tailwind.config.js` only if the installed shadcn/Tailwind-v4 version requires it (recent shadcn+Tailwind v4 setups are CSS-first via `@theme`/`@custom-variant` and may need no JS config changes — verify against the installed CLI version's own instructions and report what was actually needed)

**Interfaces:**
- Produces: the `cn(...)` helper other component tasks import from `@/lib/utils`; the `--background`/`--foreground`/`--primary`/`--card`/`--border`/`--ring` (etc.) CSS variables that shadcn's generated components consume, aliased to existing tokens; a `dark`/`light` class mechanism on `<html>` that both shadcn components and this repo's existing Phase-1 tokens respond to identically.

- [ ] **Step 1: Run the shadcn/ui CLI init**

  From `frontend/`, run `npx shadcn@latest init`. Use non-interactive/default-equivalent choices if prompted: TypeScript yes, Tailwind CSS yes (already configured), CSS variables yes, base color `neutral` (closest to this repo's existing neutral surface tokens — it will be overridden anyway in Step 2). Check the CLI's own current flags with `npx shadcn@latest init --help` first in case a non-interactive form (`--yes`, `--base-color`, etc.) is available and preferred over answering prompts live.

  This is a Pages Router project (no `app/` directory). If the CLI assumes App Router in a way that blocks setup (rather than just adjusting file suggestions), stop and report BLOCKED with the exact error — do not force a workaround that could scaffold incorrect files.

- [ ] **Step 2: Alias shadcn's CSS variables to this repo's existing tokens**

  The CLI will have added its own `:root` CSS variables (things like `--background`, `--foreground`, `--card`, `--card-foreground`, `--primary`, `--primary-foreground`, `--muted`, `--muted-foreground`, `--border`, `--input`, `--ring`, `--radius`) to `globals.css`, likely inside a `@theme inline` block or a plain `:root`/`.dark` pair, depending on CLI version. Replace the *values* of whichever of these variables exist with references to this repo's existing tokens, for example:

  ```css
  --background: var(--surface-canvas);
  --foreground: var(--text-primary);
  --card: var(--surface-raised);
  --card-foreground: var(--text-primary);
  --primary: var(--action-primary);
  --primary-foreground: var(--action-on-primary);
  --muted: var(--surface-sunken);
  --muted-foreground: var(--text-muted);
  --border: var(--border-subtle);
  --input: var(--border-subtle);
  --ring: var(--focus-ring);
  --destructive: var(--status-danger-fg);
  --destructive-foreground: var(--status-danger-bg);
  ```

  Do this once, in whichever block(s) the CLI generated (there may be a light block and a `.dark` block — alias both to the same `var(--token)` names, since those existing tokens already carry their own light/dark values). Do **not** delete or rename any existing Phase-1 token (`--surface-canvas`, `--text-primary`, etc.) — only add these new aliases alongside them. If the CLI didn't add a `--radius` variable, add one (`--radius: 0.5rem` is a reasonable default matching the existing prototype's `rounded-md` usage) since shadcn's `card`/`button`/etc. components reference it.

- [ ] **Step 3: Add the `@custom-variant dark` rule so `dark:` utilities follow the manual class, not just OS preference**

  In `globals.css`, near the top (after the `@import "tailwindcss"` line), add:

  ```css
  @custom-variant dark (&:where(.dark, .dark *));
  ```

  This makes every `dark:` utility class in the whole codebase (all ~1200 existing usages across the 13 production pages, plus anything shadcn generates) respond to a `.dark` class ancestor instead of (or in addition to) `prefers-color-scheme`. Keep the existing `@media (prefers-color-scheme: dark) { :root { ... } }` block as-is — it still supplies the *default* values before any manual class is present; only the *trigger mechanism* for `dark:` utility classes changes.

- [ ] **Step 4: Add a blocking theme-init script via `_document.tsx`**

  Create `frontend/src/pages/_document.tsx`:

  ```tsx
  import { Html, Head, Main, NextScript } from "next/document";

  const THEME_INIT_SCRIPT = `
  (function () {
    try {
      var stored = localStorage.getItem("theme");
      var mode = stored === "light" || stored === "dark" ? stored : null;
      if (mode) {
        document.documentElement.classList.add(mode);
      }
    } catch (e) {}
  })();
  `;

  export default function Document() {
    return (
      <Html lang="en">
        <Head>
          <script dangerouslySetInnerHTML={{ __html: THEME_INIT_SCRIPT }} />
        </Head>
        <body>
          <Main />
          <NextScript />
        </body>
      </Html>
    );
  }
  ```

  This runs before hydration, so a returning visitor who picked "light" or "dark" (not "system") never sees a flash of the wrong theme. No class is added for "system" (or no stored value) — that case already renders correctly via the `prefers-color-scheme` media query with no class needed.

**Verification:**
- `cd frontend && npm run build` succeeds.
- `cd frontend && npm run lint` passes with no new errors/warnings.
- Manually load the app locally, open devtools, run `localStorage.setItem('theme','dark')` then hard-refresh: page should render dark with no flash of light content first. Repeat for `'light'`. Remove the key and confirm it follows OS preference again.
- Confirm no existing page's visual appearance changed (spot check `/`, `/players`, one league page) — this step only adds plumbing, nothing consumes it yet.

- [ ] **Step 5: Commit**

```bash
git add frontend/package.json frontend/package-lock.json frontend/components.json frontend/src/lib/utils.ts frontend/src/styles/globals.css frontend/src/pages/_document.tsx frontend/tailwind.config.js
git commit -m "Install shadcn/ui + Radix, alias tokens, unify dark: variant, add theme persistence"
```

---

### Task 2: Core primitives — Button, Badge, Card, Skeleton, EmptyState, ErrorState

**Files:**
- Create (via `npx shadcn@latest add button badge card skeleton`): `frontend/src/components/ui/button.tsx`, `frontend/src/components/ui/badge.tsx`, `frontend/src/components/ui/card.tsx`, `frontend/src/components/ui/skeleton.tsx`
- Create: `frontend/src/components/design-system/EmptyState.tsx`
- Create: `frontend/src/components/design-system/ErrorState.tsx`
- Create: `frontend/src/components/design-system/focus-ring.ts`

**Interfaces:**
- Consumes: `cn()` from `@/lib/utils` (Task 1); aliased tokens from Task 1 (consumed automatically via the shadcn components' own class names — no extra work needed here).
- Produces: `Button`, `Badge`, `Card`/`CardHeader`/`CardTitle`/`CardContent` (shadcn's standard card sub-component split), `Skeleton` — all imported by later tasks from `@/components/ui/*`. `EmptyState`/`ErrorState` with this exact interface, imported from `@/components/design-system/EmptyState` and `@/components/design-system/ErrorState`. A shared `FOCUS_RING` string constant, imported by Task 3 (`DataTable`) and Task 5 (the shell components) from `@/components/design-system/focus-ring` — this lives here rather than under `shell/` because it's a cross-cutting utility both trees need, not shell-specific, and putting it in this earlier task avoids a forward dependency on Task 5.

  ```tsx
  export interface EmptyStateProps {
    title: string;
    description?: string;
  }

  export interface ErrorStateProps {
    message: string;
    onRetry?: () => void;
  }
  ```

- [ ] **Step 1: Add the shadcn primitives**

  From `frontend/`: `npx shadcn@latest add button badge card skeleton`. This generates the four files above using the aliased tokens from Task 1 automatically (they reference `bg-primary`, `text-muted-foreground`, `bg-card`, etc. — Tailwind utilities tied to the CSS variables Task 1 aliased). Do not hand-edit their internals unless the CLI output has a concrete bug; if you need to change something, note why in your report.

- [ ] **Step 2: Build `EmptyState`**

  ```tsx
  // frontend/src/components/design-system/EmptyState.tsx
  interface EmptyStateProps {
    title: string;
    description?: string;
  }

  export default function EmptyState({ title, description }: EmptyStateProps) {
    return (
      <div
        role="status"
        className="flex flex-col items-center justify-center gap-1 rounded-md border border-dashed px-6 py-10 text-center"
        style={{ borderColor: "var(--border-subtle)" }}
      >
        <p className="text-sm font-medium" style={{ color: "var(--text-primary)" }}>
          {title}
        </p>
        {description && (
          <p className="text-sm" style={{ color: "var(--text-muted)" }}>
            {description}
          </p>
        )}
      </div>
    );
  }
  ```

- [ ] **Step 3: Build `ErrorState`**

  ```tsx
  // frontend/src/components/design-system/ErrorState.tsx
  import { Button } from "@/components/ui/button";

  interface ErrorStateProps {
    message: string;
    onRetry?: () => void;
  }

  export default function ErrorState({ message, onRetry }: ErrorStateProps) {
    return (
      <div
        role="alert"
        className="flex flex-col items-start gap-3 rounded-md border px-4 py-4 sm:flex-row sm:items-center sm:justify-between"
        style={{
          borderColor: "var(--status-danger-fg)",
          backgroundColor: "var(--status-danger-bg)",
          color: "var(--status-danger-fg)",
        }}
      >
        <p className="text-sm font-medium">{message}</p>
        {onRetry && (
          <Button variant="outline" size="sm" onClick={onRetry}>
            Retry
          </Button>
        )}
      </div>
    );
  }
  ```

- [ ] **Step 4: Build the shared `FOCUS_RING` constant**

  ```tsx
  // frontend/src/components/design-system/focus-ring.ts
  export const FOCUS_RING =
    "outline-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]";
  ```

  This is the exact same value already used throughout Phase 1's `design-system-prototype/focus-ring.ts` — same visual result, just a shared, non-prototype home for it.

**Verification:**
- `cd frontend && npm run build` and `npm run lint` clean.
- No consumers exist yet — this task only produces the primitives; nothing to click through in the browser beyond confirming the app still builds/runs.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/ui/button.tsx frontend/src/components/ui/badge.tsx frontend/src/components/ui/card.tsx frontend/src/components/ui/skeleton.tsx frontend/src/components/design-system/EmptyState.tsx frontend/src/components/design-system/ErrorState.tsx frontend/src/components/design-system/focus-ring.ts frontend/package.json frontend/package-lock.json
git commit -m "Add core design-system primitives: Button, Badge, Card, Skeleton, EmptyState, ErrorState, focus-ring"
```

---

### Task 3: `StatCard` and `DataTable`

**Files:**
- Create: `frontend/src/components/design-system/StatCard.tsx`
- Create: `frontend/src/components/design-system/DataTable.tsx`

**Interfaces:**
- Consumes: `Card`/`CardContent` (Task 2), `cn()` (Task 1).
- Produces, for later tasks (League overview migration, Task 7) to import from `@/components/design-system/StatCard` and `@/components/design-system/DataTable`:

  ```tsx
  export interface StatCardProps {
    label: string;
    value: string;
    detail?: string;
  }

  export interface DataTableColumn<T> {
    /** Unique key, also used as the sort field identifier passed to onSort. */
    id: string;
    header: string;
    /** Renders a cell's content for one row. */
    cell: (row: T) => React.ReactNode;
    /** Omit for non-sortable columns. */
    sortable?: boolean;
  }

  export interface DataTableProps<T> {
    columns: DataTableColumn<T>[];
    rows: T[];
    rowKey: (row: T) => string;
    sortField?: string;
    sortDirection?: "asc" | "desc";
    onSort?: (fieldId: string) => void;
  }
  ```

- [ ] **Step 1: Build `StatCard`**

  A small labeled-value tile — this is what League Summary's four stats (Average Score, Highest Score, Closest Matchup, Biggest Blowout) and League Leaders' entries render as, replacing the current page's hand-rolled `<span className="block text-sm ...">` markup.

  ```tsx
  // frontend/src/components/design-system/StatCard.tsx
  import { Card, CardContent } from "@/components/ui/card";

  interface StatCardProps {
    label: string;
    value: string;
    detail?: string;
  }

  export default function StatCard({ label, value, detail }: StatCardProps) {
    return (
      <Card>
        <CardContent className="p-4">
          <p className="text-sm font-medium" style={{ color: "var(--text-muted)" }}>
            {label}
          </p>
          <p className="mt-1 text-2xl font-bold" style={{ color: "var(--text-primary)" }}>
            {value}
          </p>
          {detail && (
            <p className="mt-1 text-sm" style={{ color: "var(--text-secondary)" }}>
              {detail}
            </p>
          )}
        </CardContent>
      </Card>
    );
  }
  ```

- [ ] **Step 2: Build `DataTable`**

  Generic, sortable, with per-column custom cell rendering (needed to reproduce the existing playoff-% bar and the "avg (total)" PF/PA formatting exactly). Sort *state* stays owned by the caller (League overview already has `sortField`/`sortDirection`/`handleSort` — `DataTable` just renders headers as clickable and calls back, it does not sort the array itself):

  ```tsx
  // frontend/src/components/design-system/DataTable.tsx
  import type { ReactNode } from "react";
  import { FOCUS_RING } from "@/components/design-system/focus-ring";

  export interface DataTableColumn<T> {
    id: string;
    header: string;
    cell: (row: T) => ReactNode;
    sortable?: boolean;
  }

  export interface DataTableProps<T> {
    columns: DataTableColumn<T>[];
    rows: T[];
    rowKey: (row: T) => string;
    sortField?: string;
    sortDirection?: "asc" | "desc";
    onSort?: (fieldId: string) => void;
  }

  export default function DataTable<T>({
    columns,
    rows,
    rowKey,
    sortField,
    sortDirection,
    onSort,
  }: DataTableProps<T>) {
    return (
      <div className="overflow-x-auto">
        <table className="min-w-full border-collapse text-sm">
          <thead>
            <tr style={{ borderBottom: "1px solid var(--border-subtle)" }}>
              {columns.map((col) => (
                <th
                  key={col.id}
                  scope="col"
                  className="whitespace-nowrap px-4 py-3 text-left text-xs font-medium uppercase tracking-wider"
                  style={{ color: "var(--text-muted)" }}
                >
                  {col.sortable ? (
                    <button
                      type="button"
                      onClick={() => onSort?.(col.id)}
                      className={`inline-flex items-center gap-1 ${FOCUS_RING}`}
                    >
                      {col.header}
                      {sortField === col.id && (
                        <span aria-hidden="true">
                          {sortDirection === "asc" ? "↑" : "↓"}
                        </span>
                      )}
                    </button>
                  ) : (
                    col.header
                  )}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row, i) => (
              <tr
                key={rowKey(row)}
                style={{
                  backgroundColor:
                    i % 2 === 0 ? "var(--surface-raised)" : "var(--surface-sunken)",
                  borderBottom: "1px solid var(--border-subtle)",
                }}
              >
                {columns.map((col) => (
                  <td key={col.id} className="whitespace-nowrap px-4 py-4">
                    {col.cell(row)}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    );
  }
  ```

  This imports `FOCUS_RING` from `@/components/design-system/focus-ring` (Task 2, already built by the time this task runs).

**Verification:**
- `cd frontend && npm run build` and `npm run lint` clean.
- No consumers yet (Task 7 will exercise these against real data) — build passing is sufficient here.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/design-system/StatCard.tsx frontend/src/components/design-system/DataTable.tsx
git commit -m "Add StatCard and generic sortable DataTable components"
```

---

### Task 4: `Select` (league switcher primitive)

**Files:**
- Create (via `npx shadcn@latest add select`): `frontend/src/components/ui/select.tsx`

**Interfaces:**
- Consumes: `cn()` (Task 1); Radix's `@radix-ui/react-select` (installed automatically by the CLI command).
- Produces: shadcn's standard `Select`/`SelectTrigger`/`SelectValue`/`SelectContent`/`SelectItem` set, imported by Task 5's real `LeagueSwitcher`.

- [ ] **Step 1: Add the shadcn `select` primitive**

  From `frontend/`: `npx shadcn@latest add select`.

**Verification:**
- `cd frontend && npm run build` and `npm run lint` clean.

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/ui/select.tsx frontend/package.json frontend/package-lock.json
git commit -m "Add shadcn Select primitive for the real league switcher"
```

---

### Task 5: Production `AppShell` — real nav, real league switcher, real theme toggle

**Files:**
- Create: `frontend/src/components/shell/icons.tsx` (adapt from `frontend/src/components/design-system-prototype/icons.tsx` — same icon set: Overview, Schedule, Players, Teams, More, plus a ChevronDown already used by the league switcher; check that file for the exact current SVGs and `IconProps` shape and reuse them as-is, just in the new location)
- Create: `frontend/src/components/shell/nav-items.ts`
- Create: `frontend/src/components/shell/LeagueSwitcher.tsx`
- Create: `frontend/src/components/shell/MoreMenu.tsx`
- Create: `frontend/src/components/shell/TopBar.tsx`
- Create: `frontend/src/components/shell/BottomNav.tsx`
- Create: `frontend/src/components/shell/ThemeToggle.tsx`
- Create: `frontend/src/components/shell/AppShell.tsx`

**Interfaces:**
- Consumes: `useLeagues`/`useLeague` (`@/hooks/useLeagues`, unchanged), `useRouter` (`next/router`), `Select`/`SelectTrigger`/etc. (Task 4), `Button` (Task 2).
- Produces, for Task 6 to import:

  ```tsx
  // @/components/shell/AppShell
  export default function AppShell({ children }: { children: React.ReactNode }): JSX.Element;
  ```

  `AppShell` takes **no other props** — unlike the Phase-1 prototype version, it derives everything itself from the router, since it will be centralized once in `_app.tsx` rather than instantiated per page.

This task is the most judgment-heavy one: adapting the Phase-1 prototype's shell (built for a single fixed mock league, explicit props, dead-end nav links) into a real, router-driven shell. Read `frontend/src/components/Header.tsx` in full before starting — it already contains the exact real nav-item logic (league-scoped vs. global item lists, active-state matching via `router.pathname`/`router.asPath`) this task needs to reproduce with new visuals. Also read the existing Phase-1 prototype files under `frontend/src/components/design-system-prototype/` for the visual/interaction patterns (focus ring usage, keyboard handling, breakpoint classes) to carry forward — but note every prop/data source changes from mock to real, detailed below.

- [ ] **Step 1: `icons.tsx`**

  Copy verbatim from the prototype original (`design-system-prototype/icons.tsx`) into `frontend/src/components/shell/icons.tsx` — same icon components and `IconProps` shape, new location. Import `FOCUS_RING` for the components below from `@/components/design-system/focus-ring` (Task 2) — do not create a second copy of it under `shell/`.

- [ ] **Step 2: `nav-items.ts` with real, context-dependent nav sets**

  Unlike the prototype's single fixed 5-item list, production nav depends on whether the route is league-scoped (matching `Header.tsx`'s `lid` logic). Build two lists plus a helper:

  ```tsx
  import type { ComponentType } from "react";
  import {
    type IconProps,
    OverviewIcon,
    ScheduleIcon,
    PlayersIcon,
    TeamsIcon,
    MoreIcon,
  } from "./icons";

  export type ShellNavId = "overview" | "schedule" | "players" | "teams" | "more" | "home";

  export interface ShellNavItem {
    id: ShellNavId;
    label: string;
    href: (leagueId?: string) => string;
    /** Only rendered directly in the primary 4-5 slots; everything else
     * (when relevant) goes in the "More" menu. */
    primary: boolean;
  }

  // League-scoped context (a leagueId is present in the route)
  export const LEAGUE_NAV_ITEMS: ShellNavItem[] = [
    { id: "overview", label: "Overview", href: (id) => `/league/${id}`, primary: true },
    { id: "schedule", label: "Schedule", href: (id) => `/league/${id}/schedule`, primary: true },
    { id: "teams", label: "Teams", href: (id) => `/league/${id}/teams`, primary: true },
    { id: "players", label: "Players", href: () => "/players", primary: true },
  ];

  // Items that only ever live in the "More" menu when league-scoped
  export const LEAGUE_MORE_ITEMS = [
    { label: "Simulations", href: (id: string) => `/league/${id}/simulations` },
    { label: "Transactions", href: (id: string) => `/league/${id}/transactions` },
    { label: "All Leagues", href: () => "/" },
  ];

  // Global context (no leagueId in the route)
  export const GLOBAL_NAV_ITEMS: ShellNavItem[] = [
    { id: "home", label: "Home", href: () => "/", primary: true },
    { id: "players", label: "Players", href: () => "/players", primary: true },
  ];

  export const GLOBAL_MORE_ITEMS = [
    { label: "Trade Data", href: () => "/sleeper/trades" },
    { label: "Draft Data", href: () => "/sleeper/drafts" },
  ];

  export const NAV_ICONS: Record<ShellNavId, ComponentType<IconProps>> = {
    overview: OverviewIcon,
    schedule: ScheduleIcon,
    players: PlayersIcon,
    teams: TeamsIcon,
    more: MoreIcon,
    home: OverviewIcon,
  };
  ```

  Adjust the exact shape above if it turns out awkward once wired into `TopBar`/`BottomNav` in the following steps — the binding requirement is: league-scoped routes show Overview/Schedule/Teams/Players plus a More menu (Simulations/Transactions/All Leagues); global routes show Home/Players plus a More list of Trade Data/Draft Data (or, since that's only 4 destinations total, consider just showing all 4 directly with no "More" needed in the global case — your call, note which you picked and why in your report).

- [ ] **Step 3: `LeagueSwitcher.tsx` — real, wired to `useLeagues`**

  Adapt the prototype's `LeagueSwitcher` (single mock league name, non-functional disclosure) into a real switcher built on shadcn's `Select` (Task 4):

  ```tsx
  // frontend/src/components/shell/LeagueSwitcher.tsx
  import { useRouter } from "next/router";
  import { useLeagues } from "@/hooks/useLeagues";
  import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "@/components/ui/select";
  import { Skeleton } from "@/components/ui/skeleton";

  interface LeagueSwitcherProps {
    leagueId: string;
  }

  export default function LeagueSwitcher({ leagueId }: LeagueSwitcherProps) {
    const router = useRouter();
    const { leagues, isLoading } = useLeagues();

    if (isLoading) {
      return <Skeleton className="h-11 w-40 rounded-md" />;
    }

    return (
      <Select
        value={leagueId}
        onValueChange={(newId) => router.push(`/league/${newId}`)}
      >
        <SelectTrigger className="min-h-11 max-w-[10rem] sm:max-w-xs">
          <SelectValue placeholder="Select a league" />
        </SelectTrigger>
        <SelectContent>
          {leagues.map((league) => (
            <SelectItem key={league.id} value={String(league.id)}>
              {league.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    );
  }
  ```

  Rendered by `TopBar` only when `leagueId` is present in the route (matches `Header.tsx`'s current behavior of only showing league context when `lid` exists) — `TopBar` passes the resolved `leagueId` string in, this component never reads the route itself.

- [ ] **Step 4: `MoreMenu.tsx`**

  Add the Radix-backed dropdown primitive: `npx shadcn@latest add dropdown-menu` (adds `frontend/src/components/ui/dropdown-menu.tsx`). Build the shell's overflow menu on top of it — pin this as `DropdownMenu`, not `Popover`, so it gets standard menu keyboard behavior (arrow keys, `Escape`, roving focus) for free from Radix:

  ```tsx
  // frontend/src/components/shell/MoreMenu.tsx
  import Link from "next/link";
  import {
    DropdownMenu,
    DropdownMenuTrigger,
    DropdownMenuContent,
    DropdownMenuItem,
  } from "@/components/ui/dropdown-menu";
  import { MoreIcon } from "./icons";
  import { FOCUS_RING } from "@/components/design-system/focus-ring";

  interface MoreMenuItem {
    label: string;
    href: string;
  }

  interface MoreMenuProps {
    items: MoreMenuItem[];
  }

  export default function MoreMenu({ items }: MoreMenuProps) {
    return (
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            type="button"
            aria-label="More"
            className={`flex min-h-11 min-w-11 items-center justify-center rounded-md ${FOCUS_RING}`}
          >
            <MoreIcon className="h-5 w-5" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {items.map((item) => (
            <DropdownMenuItem key={item.href} asChild>
              <Link href={item.href}>{item.label}</Link>
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
    );
  }
  ```

  Callers (`TopBar`/`BottomNav`) resolve `items` from `LEAGUE_MORE_ITEMS`/`GLOBAL_MORE_ITEMS` (Step 2) with the current `leagueId` applied to each `href` function before passing them in — `MoreMenu` itself takes plain resolved `{ label, href }` pairs, no knowledge of nav context.

- [ ] **Step 5: `TopBar.tsx` and `BottomNav.tsx`**

  Adapt from the prototype versions (`design-system-prototype/TopBar.tsx`, `BottomNav.tsx`): same visual/breakpoint/focus-ring patterns, but:
  - Active-item detection now uses real `useRouter().pathname`/`asPath` matching (same logic `Header.tsx` already uses: exact match or `startsWith(item.path + "/")`), not a prop passed in.
  - Nav item list comes from `nav-items.ts`'s league-scoped or global set depending on whether `router.query.leagueId` is present.
  - `TopBar` renders `LeagueSwitcher` (Step 3) only when league-scoped, passing the resolved `leagueId`.
  - Both `TopBar` and `BottomNav` render `MoreMenu` (Step 4) as their 5th/overflow slot, passing it `LEAGUE_MORE_ITEMS`/`GLOBAL_MORE_ITEMS` (whichever applies) with each item's `href(leagueId)` already resolved to a plain string before passing the array in.
  - The prototype's `FilterEntryPoint` (filter button) is **not** carried into the production shell — League overview has no filter UI today (that's Players' territory, a later phase), so leave it out rather than build a control nothing uses yet.

- [ ] **Step 6: `ThemeToggle.tsx` — real, persisted**

  Adapt from the prototype (`design-system-prototype/ThemeToggle.tsx`): same three-state (`system`/`light`/`dark`) cycle and `motion-reduce`-guarded transition, but on change: (a) apply the class to `document.documentElement` (`<html>`) instead of a local wrapper `<div>`, since the shell is now global and must affect every page's `dark:` utilities too (this is exactly what Task 1's `@custom-variant dark` rule makes work); (b) persist the choice to `localStorage.setItem("theme", mode)` for `"light"`/`"dark"`, and `localStorage.removeItem("theme")` for `"system"`, so `_document.tsx`'s init script (Task 1) picks it up on next load.

- [ ] **Step 7: `AppShell.tsx`**

  ```tsx
  import type { ReactNode } from "react";
  import { useRouter } from "next/router";
  import TopBar from "./TopBar";
  import BottomNav from "./BottomNav";

  export default function AppShell({ children }: { children: ReactNode }) {
    const router = useRouter();
    const leagueId =
      typeof router.query.leagueId === "string" ? router.query.leagueId : undefined;

    return (
      <div
        className="flex min-h-screen flex-col"
        style={{ backgroundColor: "var(--surface-canvas)", color: "var(--text-primary)" }}
      >
        <TopBar leagueId={leagueId} pathname={router.pathname} />
        <main className="flex-1 px-3 pb-20 pt-4 sm:px-4 lg:pb-6">{children}</main>
        <BottomNav leagueId={leagueId} pathname={router.pathname} />
      </div>
    );
  }
  ```

  Adjust the exact prop-drilling of `leagueId`/`pathname` into `TopBar`/`BottomNav` if you find it cleaner for those components to call `useRouter()` themselves internally instead — either is fine, just be consistent and note your choice.

  Also render the existing `Footer.tsx` content (the "Powered by Male Friendship™" joke copy, About/Contact links — copy the text verbatim from `frontend/src/components/Footer.tsx`) inside `AppShell`, below `<main>`, rebuilt with new tokens instead of the old hardcoded Tailwind gray/blue classes.

**Verification:**
- `cd frontend && npm run build` and `npm run lint` clean.
- Not yet wired into `_app.tsx` (that's Task 6) — nothing to click through yet beyond confirming the new files compile.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/components/shell/
git commit -m "Build production AppShell: real nav mapping, league switcher, theme toggle"
```

---

### Task 6: Wire `AppShell` globally, retire `Layout`/`Header`/`Footer`

**Files:**
- Modify: `frontend/src/pages/_app.tsx`
- Modify (remove the `<Layout>` wrapper only, keep all other content/logic untouched): `frontend/src/pages/index.tsx`, `frontend/src/pages/admin/index.tsx`, `frontend/src/pages/league/[leagueId]/index.tsx`, `frontend/src/pages/league/[leagueId]/schedule/[matchupId].tsx`, `frontend/src/pages/league/[leagueId]/schedule/index.tsx`, `frontend/src/pages/league/[leagueId]/simulations/index.tsx`, `frontend/src/pages/league/[leagueId]/teams/[teamId].tsx`, `frontend/src/pages/league/[leagueId]/teams/index.tsx`, `frontend/src/pages/league/[leagueId]/transactions/index.tsx`, `frontend/src/pages/players/[playerId].tsx`, `frontend/src/pages/players/index.tsx`, `frontend/src/pages/sleeper/drafts.tsx`, `frontend/src/pages/sleeper/trades.tsx`, `frontend/src/pages/sleeper/transactions.tsx`
- Delete: `frontend/src/components/Layout.tsx`, `frontend/src/components/Header.tsx`, `frontend/src/components/Footer.tsx` (once nothing imports them — confirm with a repo-wide grep before deleting)

**Interfaces:**
- Consumes: `AppShell` from `@/components/shell/AppShell` (Task 5).

- [ ] **Step 1: Wire `AppShell` into `_app.tsx`**

  ```tsx
  import type { AppProps } from "next/app";
  import AppShell from "@/components/shell/AppShell";
  import "../styles/globals.css";

  export default function App({ Component, pageProps }: AppProps) {
    return (
      <AppShell>
        <Component {...pageProps} />
      </AppShell>
    );
  }
  ```

- [ ] **Step 2: Remove `<Layout>` wrapping from every page, one file at a time**

  For each of the 13 files listed above: remove the `import Layout from "@/components/Layout"` (or relative-path equivalent) line, remove the `<Layout>`/`</Layout>` wrapper tags, and un-indent the JSX that was inside it. Do not change anything else in these files — no logic, no styling, no reordering. This is a pure mechanical unwrap; each page's actual content stays exactly as it is today (including its own old hardcoded Tailwind color classes — those get their `dark:` behavior from Task 1's variant-unification fix, but their content/logic is out of scope for this sub-project except for League overview, which Task 7 handles separately).

- [ ] **Step 3: Confirm nothing else imports the retired components, then delete them**

  Run `grep -rl "components/Layout\|components/Header\|components/Footer" frontend/src` — it should now return nothing (or only the files themselves). Delete `frontend/src/components/Layout.tsx`, `frontend/src/components/Header.tsx`, `frontend/src/components/Footer.tsx`.

**Verification:**
- `cd frontend && npm run build` and `npm run lint` clean.
- Manual browser check: load `/`, `/players`, `/admin`, one league's overview/schedule/teams/transactions/simulations page, and one `/sleeper/*` page. Confirm every page renders its existing content correctly (visually unchanged content, just inside the new shell chrome), the new top bar/bottom nav appears with the right context-dependent items, and the bottom nav disappears above 1024px in favor of the top bar's inline nav.
- Confirm the theme toggle now changes the appearance of a **non**-League-overview page too (e.g. `/players`) — this is the concrete proof that Task 1's `dark:`-variant unification actually works end-to-end, not just in isolation.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/pages frontend/src/components
git commit -m "Wire AppShell globally in _app.tsx, retire Layout/Header/Footer"
```

---

### Task 7: Migrate League overview content to the new system (Layout B)

**Files:**
- Modify: `frontend/src/pages/league/[leagueId]/index.tsx`
- Modify (visual rebuild only — restyle with new tokens/components, no prop/behavior change; each already has its own internal data plumbing untouched): `frontend/src/components/AllTimeMatchupsGrid.tsx`, `frontend/src/components/HallOfFameWallOfShame.tsx`, `frontend/src/components/AllTimeRecordsTable.tsx`

**Interfaces:**
- Consumes: `DataTable`/`StatCard` (Task 3), `Card`/`Badge`/`Button` (Task 2), `EmptyState`/`ErrorState` (Task 2), the existing `useLeague` hook (`@/hooks/useLeagues`, not `useState`/`useEffect` + `leaguesService.getLeague` directly), `useTeams`/`useSchedule` (unchanged).

This is the task with the most behavior to preserve exactly. The current page (`frontend/src/pages/league/[leagueId]/index.tsx`, already read in full during planning) has this logic that must carry over **unchanged**:

- The owner-exclusion filter: `!team.owner.includes("Knapp") && !team.owner.includes("Landry")`, applied identically everywhere it's applied today (standings, League Leaders' two lists).
- `adjustedRecords` and `headToHeadRecords` derivations (the two `useMemo` blocks computing wins/losses from `schedule.data.matchups`).
- `leagueStats` derivation (`useMemo` computing `highestScore`, `closestMatchup`, `biggestBlowout`, `averageScore`, `totalGames`, `completedGames`).
- The `sortField`/`sortDirection`/`handleSort`/`sortedTeams` sorting logic, including its exact field list (`rank`, `name`, `wins`, `losses`, `pf`, `pa`, `playoffs`, `diff`) and comparator behavior.
- The playoff-% bar's color thresholds (`>75` green, `>50` blue, `>25` yellow, else red).
- The PF/PA cell formatting (`"{avg} ({total})"`, both to 2 decimals).

Copy all of the above verbatim from the current file into the migrated version — same variable names, same computation, same thresholds. Only the *rendering* changes.

- [ ] **Step 1: Replace the inline league-fetch with `useLeague`**

  Delete the `useState<League | null>`/`useEffect`/`leaguesService.getLeague` block and the `isLeagueLoading` state. Replace with:

  ```tsx
  import { useLeague } from "@/hooks/useLeagues";
  // ...
  const { league, isLoading: isLeagueLoading, error: leagueError } = useLeague(id);
  ```

  (`useLeague` takes `number | undefined`; guard `id` the same way the page already does elsewhere.)

- [ ] **Step 2: Rebuild the header + "This season" band**

  Page header: league name (from `league.name`) and week context (`Week {current_week} of {total_weeks}`), loading via `Skeleton`, using `--text-primary`/typography scale instead of the old `text-blue-600` heading — this page no longer needs its own "back to all leagues" footer link, since that now lives in the shell's League nav / More menu (Task 5/6); remove the old `<Link href="/">← All Leagues</Link>` section, it would be a redundant duplicate now that the shell provides it.

  "This season" band, in this order: Standings (rebuilt on `DataTable` — columns: Rank, Team [name + owner, still linking to `/league/{id}/teams/{team.espnId}`], W, L, PF/G, PA/G, Diff, Playoff % [rendered as a small horizontal bar using the exact color thresholds above, plus the numeric `%` as a non-color-redundant label, sortable on every column that's sortable today) → a responsive row of 4 `StatCard`s for League Summary (Average Score, Highest Score, Closest Matchup, Biggest Blowout, each with the same computed values/detail strings the current page shows) → League Leaders as two small ranked lists (Most Points Scored, Most Points Against) using `StatCard` or plain rows, your call on which reads better — either preserves the same top-3-per-list content.

  Loading: while `isLoading` (teams) or `isScheduleLoading`, show `Skeleton` placeholders shaped like the eventual content, matching Task 2's `Skeleton` primitive, replacing today's spinner divs. Error: if `error || scheduleError`, render `ErrorState` with the existing error message(s) and **no** `onRetry` — today's page has no retry action on error, and adding one would be a new behavior, not a preserved one, so leave `onRetry` unset here even though the prop exists on the component.

- [ ] **Step 3: Rebuild the "League history" band**

  A labeled section (`<h2>League history</h2>` styled with the same section-heading treatment as "This season") containing, in this order: `AllTimeMatchupsGrid`, `HallOfFameWallOfShame`, `AllTimeRecordsTable` — same props as today, just wrapped in the new section-heading/`Card` treatment instead of the old ad hoc `<section>` boundaries. Do not modify these three components' internals in this task beyond whatever minimal restyle is needed for them to sit visually consistent inside a `Card`/token-based container — if their internal styling needs more than a wrapper change to look right, that's fine, but keep their props/data logic untouched.

**Verification:**
- `cd frontend && npm run build` and `npm run lint` clean.
- Manual browser check: load a real league's `/league/{id}` page. Confirm: standings render with correct data and are still sortable on every column (click each sortable header, confirm order changes correctly both directions); playoff-% bar renders with correct colors at different thresholds; League Summary/Leaders show the same numbers as before the migration (compare against `git stash`/a second tab on `main` if easy, or just sanity-check the math); Head-to-Head grid, Hall of Fame/Wall of Shame, and All-Time Records still render with real data; loading state (throttle network in devtools) shows skeletons; forcing an error (e.g. temporarily point a service at a bad URL, or simulate via devtools network blocking) shows `ErrorState` with a sensible message, then revert the simulated failure.
- Confirm the "This season"/"League history" band labels are visually present and the two groups read as distinct at a glance (the acceptance criterion from the original source plan: "recognize page hierarchy... within five seconds").

- [ ] **Step 4: Commit**

```bash
git add frontend/src/pages/league/[leagueId]/index.tsx frontend/src/components/AllTimeMatchupsGrid.tsx frontend/src/components/HallOfFameWallOfShame.tsx frontend/src/components/AllTimeRecordsTable.tsx
git commit -m "Migrate League overview to design-system components with This season / League history bands"
```

---

### Task 8: Whole-branch verification pass

**Files:** none (verification only, no code changes expected — if this task finds a real bug, fix it here and note the fix in the report rather than leaving it for a human to catch).

- [ ] **Step 1: Full build + lint**

  `cd frontend && npm run build && npm run lint` — must be completely clean, zero new warnings anywhere in the repo (not just the touched files).

- [ ] **Step 2: Manual browser sweep**

  Start the dev server. In a real browser (or browser automation if available):
  - Load every one of the 13 production routes at least once (a representative league/team/player/matchup id is fine) and confirm each renders without console errors, inside the new shell chrome.
  - On League overview specifically: repeat the full check from Task 7's verification step.
  - Toggle the theme switch through all three states (system/light/dark) on at least 3 different pages (League overview, `/players`, `/admin`), confirming every page's colors — both shell chrome and page content — respond correctly and consistently in all three states.
  - Resize (or use device emulation) across the 1024px breakpoint on at least 2 pages, confirming the bottom nav / top nav swap correctly and no content is clipped or overlapped by the fixed bottom nav on mobile widths.
  - Confirm keyboard-only navigation can reach and operate: every primary nav item, the League switcher (when present), the More menu, and the theme toggle.

- [ ] **Step 3: Report**

  Summarize what was checked and any issues found/fixed. If browser automation isn't available in your environment, say so explicitly rather than silently skipping this step — a human will need to do this pass instead (matching Phase 1's precedent, where the controller performed this check directly when a subagent couldn't).

- [ ] **Step 4: Commit** (only if Step 2 required a fix)

```bash
git add -A
git commit -m "Fix issues found in Phase 2/3 whole-branch verification pass"
```
