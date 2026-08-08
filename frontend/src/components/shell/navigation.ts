export function isActiveNavItem(
  currentPath: string,
  href: string,
  exact = false,
) {
  return currentPath === href || (!exact && href !== "/" && currentPath.startsWith(`${href}/`));
}
