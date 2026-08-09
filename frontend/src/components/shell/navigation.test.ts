import assert from "node:assert/strict";
import test from "node:test";
import { LEAGUE_NAV_ITEMS } from "./nav-items";
import { isActiveNavItem } from "./navigation";

test("only the exact league overview route activates Overview", () => {
  const overviewHref = "/league/42";

  assert.equal(isActiveNavItem(overviewHref, overviewHref, true), true);
  assert.equal(isActiveNavItem("/league/42/schedule", overviewHref, true), false);
  assert.equal(isActiveNavItem("/league/42/teams", overviewHref, true), false);
});

test("the global home route does not activate on another global route", () => {
  assert.equal(isActiveNavItem("/players", "/"), false);
});

test("nested routes keep their parent tab active", () => {
  assert.equal(
    isActiveNavItem("/league/42/schedule/99", "/league/42/schedule"),
    true,
  );
});

test("league navigation exposes a direct Home destination", () => {
  const homeItem = LEAGUE_NAV_ITEMS.find((item) => item.id === "home");

  assert.equal(homeItem?.label, "Home");
  assert.equal(homeItem?.href(), "/");
});
