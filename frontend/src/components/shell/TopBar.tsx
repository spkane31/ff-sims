import Link from "next/link";
import { useRouter } from "next/router";
import {
  LEAGUE_NAV_ITEMS,
  LEAGUE_MORE_ITEMS,
  GLOBAL_NAV_ITEMS,
  NAV_ICONS,
} from "./nav-items";
import LeagueSwitcher from "./LeagueSwitcher";
import MoreMenu from "./MoreMenu";
import ThemeToggle from "./ThemeToggle";
import { FOCUS_RING } from "@/components/design-system/focus-ring";

/**
 * Top bar: league switcher (league-scoped routes only) at every width,
 * plus the primary nav link group which is hidden below `lg` (1024px) and
 * shown from `lg` up. Below `lg`, BottomNav is the primary nav instead.
 */
export default function TopBar() {
  const router = useRouter();
  const leagueId =
    typeof router.query.leagueId === "string" ? router.query.leagueId : undefined;

  const navItems = leagueId ? LEAGUE_NAV_ITEMS : GLOBAL_NAV_ITEMS;
  const moreItems = leagueId
    ? LEAGUE_MORE_ITEMS.map((item) => ({ label: item.label, href: item.href(leagueId) }))
    : [];

  const isActive = (href: string) =>
    router.asPath === href || router.asPath.startsWith(`${href}/`);

  return (
    <header
      className="sticky top-0 z-30 flex items-center justify-between gap-3 border-b px-3 py-2 sm:px-4 lg:grid lg:grid-cols-[1fr_auto_1fr]"
      style={{
        backgroundColor: "var(--surface-raised)",
        borderColor: "var(--border-subtle)",
      }}
    >
      <div className="flex items-center gap-3 lg:justify-self-start">
        {leagueId && <LeagueSwitcher leagueId={leagueId} />}
      </div>

      <nav
        aria-label="Primary"
        className="hidden items-center justify-center gap-1 lg:flex lg:justify-self-center"
      >
        {navItems.map((item) => {
          const Icon = NAV_ICONS[item.id];
          const href = item.href(leagueId);
          const active = isActive(href);

          return (
            <Link
              key={item.id}
              href={href}
              aria-current={active ? "page" : undefined}
              className={`flex min-h-11 items-center gap-2 rounded-full px-3 text-sm transition-shadow ${FOCUS_RING}`}
              style={
                active
                  ? {
                      backgroundColor: "var(--action-primary)",
                      color: "var(--action-on-primary)",
                      fontWeight: 600,
                      boxShadow:
                        "0 4px 10px oklch(0 0 0 / 0.45), 0 1px 2px oklch(0 0 0 / 0.35), inset 0 1px 0 oklch(1 0 0 / 0.4)",
                    }
                  : {
                      color: "var(--text-secondary)",
                      fontWeight: 500,
                    }
              }
            >
              <Icon className="h-4 w-4" active={active} />
              {item.label}
            </Link>
          );
        })}
        {moreItems.length > 0 && <MoreMenu items={moreItems} />}
      </nav>

      <div className="flex items-center gap-2 lg:justify-self-end">
        <ThemeToggle />
      </div>
    </header>
  );
}
