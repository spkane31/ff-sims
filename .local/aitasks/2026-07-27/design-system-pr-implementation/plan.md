Status: in-progress

# PR implementation plan — design-system migration

## Goal

Ship the approved Option A design system in two focused PRs: first establish test and design-token guardrails, then migrate the shared shell and Players pilot. A third PR is available only if the second becomes too large; it is not the default plan.

## Decisions made before implementation

- **Test runner:** add Vitest, jsdom, React Testing Library, `@testing-library/jest-dom`, and `@testing-library/user-event`; configure a `test` script and shared test setup before writing any feature test.
- **Accessibility:** use `vitest-axe` for automated primitive and Players-page checks. Keyboard behavior is additionally exercised with Testing Library user events.
- **Theme control:** add a simple header toggle that cycles light, dark, and system themes. Persist the choice in browser storage and apply it before paint to avoid a visible theme flash.
- **Screenshots:** use manual local-browser captures at desktop and mobile widths in light and dark system themes; attach them to the PR description. Browser automation/visual-regression infrastructure is explicitly out of scope for this migration.
- **Tailwind v4/shadcn:** retain Tailwind v4's CSS-first configuration. Do not rely on `shadcn add` or a JavaScript `theme` block; manually adapt the selected app-owned component source to the CSS-variable and `@theme` model, validating every imported primitive against the installed Tailwind version.
- **Shell rollout:** update the existing `Layout` adapter so every current route receives the new app shell immediately. Only the Players *content* is redesigned in this PR, preventing a navigation-shell bounce between routes.
- **Mobile information architecture:** use five context-aware destinations. In league context: **Overview**, **Schedule**, **Teams**, **Players**, and **More**; More contains Simulations, Transactions, and All Leagues. Outside league context: **Home**, **Players**, **Trade Data**, **Draft Data**, and **More**. More opens an accessible modal bottom sheet. Desktop retains the complete contextual navigation.
- **Shell API:** `AppShell` derives league context and active navigation state from `useRouter` and `useLeague`, just as the current header does. Existing pages continue to render `<Layout>{children}</Layout>` without new props.
- **Legacy route styles:** do not rewrite raw `dark:*` classes outside the shell and Players pilot. Record them in a migration inventory, leave their page content visually legacy for now, and remove them route-by-route in follow-up PRs.
- **Legacy layout safety:** preserve the current desktop content geometry (`container`, horizontal padding, and vertical padding) inside the `Layout` adapter for this PR. The mobile shell owns bottom safe-area padding equal to its fixed navigation height so legacy content cannot be obscured.

## PR sequence and scope

### PR 1 — Test and design-token guardrails

**Include**

- Vitest, jsdom, React Testing Library, `@testing-library/jest-dom`, `@testing-library/user-event`, and `vitest-axe` configuration.
- Test scripts, shared test setup, and a verified baseline test that proves the runner and accessibility matcher operate correctly.
- `check:design-tokens` source check scoped to future migrated paths, with an explicit legacy allowlist.
- A chart-color and legacy `dark:*` migration inventory; this is documentation only, not a visual migration.

**Exclude**

- Component, shell, layout, navigation, token-value, or product-page changes.
- Radix/shadcn dependencies; add them with the UI migration that consumes them.

**Exit criteria**

- `npm run test` and `npm run check:design-tokens` run in CI/local development.
- The test baseline is observed failing before its setup/configuration is complete, then passing afterward.
- Existing source is explicitly allowlisted rather than silently exempted.

### PR 2 — Theme, shared shell, and Players pilot

**Include**

- Semantic color, type, spacing, radius, shadow, chart, and focus tokens.
- Light and dark theme token values, with system as the initial default and a persisted user override.
- App-owned UI primitives using shadcn/ui conventions and Radix behavior where interaction is complex.
- Shared desktop/mobile application shell on every existing route: league context header, responsive primary navigation, and fixed mobile bottom navigation.
- One representative page migrated as a visual proof point: the Players page.
- Component, interaction, and accessibility tests using the PR 1 infrastructure.

**Exclude**

- API, data-model, or behavior changes.
- Full migration of league, schedule, teams, transactions, simulations, and admin pages.
- Visual migration of feature content outside Players. Those pages use the new shell but retain their legacy page-level styling temporarily.

### Optional PR 3 — Players pilot split

Create this only if PR 2 exceeds a reviewable size. PR 2 then contains tokens, primitives, theme toggle, and the all-route shared shell; PR 3 contains only the Players content migration and its tests. Both PRs retain the same compatibility and verification rules below.

## Implementation phases

### Phase 1 — PR 1: establish executable guardrails

1. Add and configure Vitest, jsdom, React Testing Library, `@testing-library/jest-dom`, `@testing-library/user-event`, and `vitest-axe`. Add `npm run test` and test setup files.
2. Add `npm run check:design-tokens`: a scoped source check that rejects raw Tailwind palette/color utilities and arbitrary visual color values in future migrated paths (`components/ui`, `AppShell`, and Players). Keep an explicit legacy allowlist so pre-existing routes do not block this PR.
3. Audit `ExpectedWinsChart`, `TeamExpectedWinsChart`, and `SleeperGrowthCharts` for colors that bypass existing chart tokens. Inventory other raw `dark:*` styles. Record only the deferred route-migration work.

```tsx
expect(await axe(container)).toHaveNoViolations()
```

### Phase 2 — PR 2: build the foundation and shared chrome

1. Add minimal Radix and shadcn-compatible support dependencies plus local `components/ui` primitives. Manually adapt selected component source to Tailwind v4 CSS variables; do not run the shadcn CLI as an assumed generator.
2. Reconcile the existing chart custom properties with a semantic token taxonomy rather than replacing them blindly.
3. Add `Button`, `Input`, `Select`, `Card`, `Badge`, `Tabs`, `Skeleton`, and `DataTable` foundations.
4. Rebuild the existing shared layout and header as `AppShell`, retaining current route behavior.
5. Make `Layout` a compatibility adapter over `AppShell`, so every one of the current pages that imports `Layout` receives the same responsive shell without individual page edits. Preserve its current desktop container and padding geometry.
6. Have `AppShell` derive `leagueId`, league metadata, active section, and mobile-nav state internally from `useRouter` and `useLeague`; do not add per-page shell props.
7. Provide the approved context-aware five-destination mobile navigation. In league context, put Simulations, Transactions, and All Leagues in More; outside it, retain Home, Players, Trade Data, Draft Data, and More.
8. Reserve scroll-container bottom padding for the fixed mobile navigation and `env(safe-area-inset-bottom)` at the shell level.
9. Use an accessible mobile menu/dialog primitive with focus management and Escape support.

```tsx
// Existing pages remain unchanged.
<Layout>
  <PlayersPage />
</Layout>
```

### Phase 3 — PR 2: migrate the Players pilot

1. Apply the page header, filter toolbar, loading/error/empty states, and compact mobile list/table treatment.
2. Preserve filtering, sorting, pagination, and navigation behavior exactly.
3. On mobile, expose filters through an accessible bottom sheet; retain concise primary columns and make secondary data progressively available.
4. Add component and Players interaction tests first, confirming each new test fails against the pre-change behavior before implementing its corresponding UI change.

```tsx
<PageHeader title="Players" description="Compare historical performance." />
<DataToolbar filters={<PlayerFilters />} />
<PlayerResults responsiveMode="ranked-list" />
```

### Phase 4 — PR 2: verify and document

1. Run Vitest, frontend lint, `check:design-tokens`, and the production build.
2. Run axe checks for the primitives, AppShell, and Players flow; verify keyboard navigation with user-event tests and a manual browser pass.
3. Capture manual local-browser screenshots at desktop/mobile widths in light/dark system modes for Players and shell smoke checks on an unmigrated league overview, Schedule, and Admin route. Check unchanged-page container spacing and ensure mobile content clears the fixed navigation and iOS safe area.
4. Document component usage rules and update the legacy-style migration inventory.

```md
PR validation
- System, light, and dark themes render without contrast regressions
- All primary navigation and filters work with keyboard only
- Players filtering and pagination retain current behavior
```

## File-level change map

- Update the global stylesheet and Tailwind theme configuration to define and expose semantic tokens.
- Add the PR 1 test configuration/setup and `vitest-axe` adapter before adding feature tests.
- Add `frontend/src/components/ui/` for app-owned primitives and `frontend/src/components/AppShell.tsx` for shared chrome.
- Convert [Layout.tsx](/Users/seankane/github.com/ff-sims-design-redo/frontend/src/components/Layout.tsx:1) into the compatibility adapter while preserving its current desktop content geometry. Delete [Header.tsx](/Users/seankane/github.com/ff-sims-design-redo/frontend/src/components/Header.tsx:1) after `AppShell` absorbs its league badge, contextual navigation, and mobile-menu responsibilities. This applies the new shell to all routes, while route content remains legacy until individually migrated.
- Migrate [players/index.tsx](/Users/seankane/github.com/ff-sims-design-redo/frontend/src/pages/players/index.tsx:1) as the pilot and map current chart globals from [globals.css](/Users/seankane/github.com/ff-sims-design-redo/frontend/src/styles/globals.css:1) into the semantic token layer.
- Inventory—not migrate—the other current `dark:*` feature styles and non-token chart colors, including the existing Recharts consumers.

## PR acceptance criteria

- PR 1's Vitest/RTL and `vitest-axe` infrastructure exists, and each new PR 2 UI test was observed failing before its implementation and passing after it.
- The application has a consistent light/dark semantic token system for the shell and migrated components; unmigrated page content is explicitly tracked as legacy.
- Every route receives the same approved editorial, mobile-first shell without changing routes or data behavior.
- The shell preserves the existing desktop content dimensions for legacy routes, and its mobile scroll region clears the fixed navigation plus device safe area.
- The Players pilot is usable at narrow widths, including keyboard navigation and filter access.
- Manual smoke checks pass for Players plus an unmigrated league overview, Schedule, and Admin route in desktop/mobile light/dark modes.
- `check:design-tokens` prevents new raw visual values in migrated paths.
- `npm run test`, `npm run lint`, `npm run check:design-tokens`, and `npm run build` pass; targeted accessibility checks pass.

## Follow-up PRs

Migrate remaining routes in dependency order: League overview → Schedule → Teams → Transactions/Drafts → Simulations → Admin. Each PR reuses the approved primitives and includes only the components necessary for that route.

## Approval gate

After this plan is approved, hand off PR 1. Start PR 2 only after PR 1 is merged or its test/guardrail changes are otherwise available to the implementation branch.
