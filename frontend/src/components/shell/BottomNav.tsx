import Link from "next/link";
import { useRouter } from "next/router";
import {
  LEAGUE_NAV_ITEMS,
  LEAGUE_MORE_ITEMS,
  GLOBAL_NAV_ITEMS,
  NAV_ICONS,
} from "./nav-items";
import MoreMenu from "./MoreMenu";
import { isActiveNavItem } from "./navigation";
import { FOCUS_RING } from "@/components/design-system/focus-ring";

/**
 * Persistent bottom navigation (mobile). Removed from the layout (not just
 * visually hidden) at `lg` (1024px) and up via `lg:hidden`, so it also
 * leaves the tab order at that width — TopBar's primary nav takes over.
 * Each item keeps a 44px min hit area; the current item pairs a color
 * change with `aria-current="page"` plus a font-weight + top-border
 * change (never color alone).
 */
export default function BottomNav() {
  const router = useRouter();
  const leagueId =
    typeof router.query.leagueId === "string" ? router.query.leagueId : undefined;

  const navItems = leagueId ? LEAGUE_NAV_ITEMS : GLOBAL_NAV_ITEMS;
  const moreItems = leagueId
    ? LEAGUE_MORE_ITEMS.map((item) => ({ label: item.label, href: item.href(leagueId) }))
    : [];

  const isActive = (href: string, exact?: boolean) =>
    isActiveNavItem(router.asPath, href, exact);

  return (
    <nav
      aria-label="Primary"
      className="fixed inset-x-0 bottom-0 z-30 flex border-t lg:hidden"
      style={{
        backgroundColor: "var(--surface-raised)",
        borderColor: "var(--border-subtle)",
      }}
    >
      {navItems.map((item) => {
        const Icon = NAV_ICONS[item.id];
        const href = item.href(leagueId);
        const active = isActive(href, item.exact);

        return (
          <Link
            key={item.id}
            href={href}
            aria-current={active ? "page" : undefined}
            className={`flex min-h-11 flex-1 flex-col items-center justify-center gap-0.5 border-t-2 py-1.5 text-xs ${FOCUS_RING}`}
            style={{
              borderColor: active ? "var(--action-primary)" : "transparent",
              color: active ? "var(--action-primary)" : "var(--text-muted)",
              fontWeight: active ? 600 : 500,
            }}
          >
            <Icon className="h-5 w-5" active={active} />
            {item.label}
          </Link>
        );
      })}
      {moreItems.length > 0 && (
        <div className="flex flex-1 items-center justify-center">
          <MoreMenu items={moreItems} />
        </div>
      )}
    </nav>
  );
}
