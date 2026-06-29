# League Header Context Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show the current league's name and platform badge in the nav header, replacing the generic "The League" app title when inside a `/league/[leagueId]/` route.

**Architecture:** Add a `useLeague` hook that fetches a single league by ID, then consume it in `Header.tsx` to conditionally render league name + platform badge instead of the static title. No other files change.

**Tech Stack:** Next.js 15, React 19, TypeScript 5.8, Tailwind CSS 4. No test framework — verification is `next build` (TypeScript check) + visual inspection.

## Global Constraints

- Working directory for all commands: `v2/frontend/`
- TypeScript must pass (`npm run build`) after each task — treat build errors as test failures
- Tailwind classes only — no inline styles
- No new dependencies

---

### Task 1: Add `useLeague` hook

**Files:**
- Modify: `src/hooks/useLeagues.ts`

**Interfaces:**
- Consumes: `leaguesService.getLeague(leagueId: number): Promise<League>` (already exists in `src/services/leaguesService.ts`)
- Produces: `useLeague(leagueId: number | undefined): { league: League | null, isLoading: boolean, error: Error | null }`

- [ ] **Step 1: Add `useLeague` to `src/hooks/useLeagues.ts`**

Append this function after the existing `useLeagueYears` export. The file already imports `useState`, `useEffect`, `useCallback` from React and `leaguesService`, `League` from the service — no new imports needed.

```typescript
export function useLeague(leagueId: number | undefined) {
  const [league, setLeague] = useState<League | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    if (leagueId === undefined) {
      setLeague(null);
      setIsLoading(false);
      setError(null);
      return;
    }
    setIsLoading(true);
    setError(null);
    leaguesService
      .getLeague(leagueId)
      .then(setLeague)
      .catch((err) =>
        setError(err instanceof Error ? err : new Error("Failed to fetch league"))
      )
      .finally(() => setIsLoading(false));
  }, [leagueId]);

  return { league, isLoading, error };
}
```

- [ ] **Step 2: Verify TypeScript compiles**

```bash
npm run build
```

Expected: build succeeds with no type errors. If it fails, fix type errors before continuing.

- [ ] **Step 3: Commit**

```bash
git add src/hooks/useLeagues.ts
git commit -m "feat: add useLeague hook for single-league fetch"
```

---

### Task 2: Update Header to display league name and platform badge

**Files:**
- Modify: `src/components/Header.tsx`

**Interfaces:**
- Consumes: `useLeague(leagueId: number | undefined)` from Task 1

- [ ] **Step 1: Replace `src/components/Header.tsx` with the updated version**

The changes are: import `useLeague`, add `PlatformBadge` component, call the hook, replace the static title with conditional rendering. Full file:

```tsx
import Link from "next/link";
import { useRouter } from "next/router";
import { useState } from "react";
import { useLeague } from "@/hooks/useLeagues";

const platformColors: Record<string, string> = {
  espn: "bg-red-600",
};

function PlatformBadge({ platform }: { platform: string }) {
  const color = platformColors[platform.toLowerCase()] ?? "bg-gray-500";
  return (
    <span className={`${color} text-white text-xs px-2 py-0.5 rounded-full font-semibold ml-2`}>
      {platform.toUpperCase()}
    </span>
  );
}

const Header = () => {
  const router = useRouter();
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);

  const { leagueId } = router.query;
  const lid = leagueId as string | undefined;
  const lidNum = lid ? Number(lid) : undefined;

  const { league, isLoading: isLeagueLoading } = useLeague(lidNum);

  const navItems = lid
    ? [
        { name: "League", path: `/league/${lid}` },
        { name: "Simulations", path: `/league/${lid}/simulations` },
        { name: "Schedule", path: `/league/${lid}/schedule` },
        { name: "Teams", path: `/league/${lid}/teams` },
        { name: "Players", path: `/league/${lid}/players` },
        { name: "Transactions", path: `/league/${lid}/transactions` },
      ]
    : [
        { name: "Home", path: "/" },
      ];

  const toggleMobileMenu = () => {
    setIsMobileMenuOpen(!isMobileMenuOpen);
  };

  return (
    <header className="bg-gradient-to-r from-blue-600 to-blue-800 text-white shadow-md">
      <div className="container mx-auto px-4 py-4">
        <div className="flex justify-between items-center">
          <Link href={lid ? `/league/${lid}` : "/"} className="text-2xl font-bold flex items-center">
            {league ? (
              <>
                {league.name}
                <PlatformBadge platform={league.platform} />
              </>
            ) : lid && isLeagueLoading ? (
              <span className="opacity-60">Loading…</span>
            ) : (
              "The League"
            )}
          </Link>

          {/* Desktop Navigation */}
          <nav className="hidden md:block">
            <ul className="flex space-x-8">
              {navItems.map((item) => (
                <li key={item.path}>
                  <Link
                    href={item.path}
                    className={`px-3 py-2 rounded-md transition-colors duration-200 hover:bg-blue-700 ${
                      router.pathname === item.path || router.asPath.startsWith(item.path + "/")
                        ? "bg-blue-700 font-medium"
                        : "hover:bg-blue-700/70"
                    }`}
                  >
                    {item.name}
                  </Link>
                </li>
              ))}
              {lid && (
                <li>
                  <Link
                    href="/"
                    className="px-3 py-2 rounded-md transition-colors duration-200 hover:bg-blue-700/70 text-blue-200"
                  >
                    All Leagues
                  </Link>
                </li>
              )}
            </ul>
          </nav>

          {/* Mobile Menu Button */}
          <button
            className="md:hidden flex flex-col justify-center items-center w-8 h-8 space-y-1"
            onClick={toggleMobileMenu}
            aria-label="Toggle mobile menu"
          >
            <span className={`block w-6 h-0.5 bg-white transition-all duration-300 ${
              isMobileMenuOpen ? "rotate-45 translate-y-2" : ""
            }`}></span>
            <span className={`block w-6 h-0.5 bg-white transition-all duration-300 ${
              isMobileMenuOpen ? "opacity-0" : ""
            }`}></span>
            <span className={`block w-6 h-0.5 bg-white transition-all duration-300 ${
              isMobileMenuOpen ? "-rotate-45 -translate-y-2" : ""
            }`}></span>
          </button>
        </div>

        {/* Mobile Navigation */}
        <nav className={`md:hidden transition-all duration-300 overflow-hidden ${
          isMobileMenuOpen ? "max-h-96 opacity-100 mt-4" : "max-h-0 opacity-0"
        }`}>
          <ul className="flex flex-col space-y-2 py-2">
            {navItems.map((item) => (
              <li key={item.path}>
                <Link
                  href={item.path}
                  className={`block px-4 py-3 rounded-md transition-colors duration-200 hover:bg-blue-700 ${
                    router.asPath === item.path
                      ? "bg-blue-700 font-medium"
                      : "hover:bg-blue-700/70"
                  }`}
                  onClick={() => setIsMobileMenuOpen(false)}
                >
                  {item.name}
                </Link>
              </li>
            ))}
            {lid && (
              <li>
                <Link
                  href="/"
                  className="block px-4 py-3 rounded-md transition-colors duration-200 hover:bg-blue-700/70 text-blue-200"
                  onClick={() => setIsMobileMenuOpen(false)}
                >
                  All Leagues
                </Link>
              </li>
            )}
          </ul>
        </nav>
      </div>
    </header>
  );
};

export default Header;
```

- [ ] **Step 2: Verify TypeScript compiles**

```bash
npm run build
```

Expected: build succeeds. Fix any type errors before continuing.

- [ ] **Step 3: Visual verification — run dev server**

```bash
npm run dev
```

Check the following:
1. Visit `http://localhost:3000/` — header shows "The League" (no badge)
2. Visit `http://localhost:3000/league/1` — header shows the league's name with a colored platform badge (e.g. red `ESPN` pill)
3. Visit `http://localhost:3000/league/1/teams` — same header with league name, badge still present
4. While the league is loading, title should show "Loading…" briefly before the name appears
5. "All Leagues" link is visible in the nav and returns to `/`

- [ ] **Step 4: Commit**

```bash
git add src/components/Header.tsx
git commit -m "feat: show league name and platform badge in header (#56)"
```
