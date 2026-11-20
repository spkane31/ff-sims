export function isActiveNavItem(
  currentPath: string,
  href: string,
  exact = false,
) {
  const pathname = currentPath.split(/[?#]/, 1)[0];
  return pathname === href || (!exact && href !== "/" && pathname.startsWith(`${href}/`));
}
