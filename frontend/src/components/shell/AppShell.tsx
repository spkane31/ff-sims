import type { ReactNode } from "react";
import Link from "next/link";
import TopBar from "./TopBar";
import BottomNav from "./BottomNav";

interface AppShellProps {
  children: ReactNode;
}

/**
 * Production app shell, wired once in `_app.tsx` so every route inherits
 * it. Derives everything it needs (league context, active nav) from the
 * router itself rather than taking props, since it's no longer
 * instantiated per-page.
 */
export default function AppShell({ children }: AppShellProps) {
  return (
    <div
      className="flex min-h-screen flex-col"
      style={{ backgroundColor: "var(--surface-canvas)", color: "var(--text-primary)" }}
    >
      <TopBar />

      <main className="mx-auto w-full max-w-7xl flex-1 px-3 pb-20 pt-4 sm:px-4 lg:pb-6">
        {children}
      </main>

      <footer
        className="border-t px-4 py-8"
        style={{ borderColor: "var(--border-subtle)" }}
      >
        <div className="mx-auto flex max-w-7xl flex-col items-center gap-4 text-center">
          <div>
            <h2 className="text-2xl font-bold md:text-3xl" style={{ color: "var(--action-primary)" }}>
              Powered by Male Friendship™
            </h2>
            <p className="mt-2 text-sm italic" style={{ color: "var(--text-muted)" }}>
              Because nothing brings the boys together like arguing over waiver wire pickups
            </p>
          </div>
          <div
            className="flex w-full flex-col items-center gap-4 border-t pt-4 md:flex-row md:justify-between"
            style={{ borderColor: "var(--border-subtle)" }}
          >
            <p className="text-sm" style={{ color: "var(--text-muted)" }}>
              © {new Date().getFullYear()} FF Sims. All rights reserved.
            </p>
            <div className="flex gap-6">
              <Link href="/about" className="text-sm hover:underline" style={{ color: "var(--text-muted)" }}>
                About
              </Link>
              <Link
                href="https://github.com/spkane31/ff-sims"
                className="text-sm hover:underline"
                style={{ color: "var(--text-muted)" }}
              >
                Contact
              </Link>
            </div>
          </div>
        </div>
      </footer>

      <BottomNav />
    </div>
  );
}
