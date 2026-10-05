import { normalizePath } from "./navigation.mjs";

/** Repeated YAML references remain navigable without expanding every alias branch. */
export function sidebarActivePath(item, activePath) {
  const path = normalizePath(activePath);
  let alias = false;
  function hasCanonicalMatch(node) {
    if (node.href && normalizePath(node.href) === path) {
      if (!node.customProps?.yamlReference) return true;
      alias = true;
    }
    return (node.items || []).some(hasCanonicalMatch);
  }
  return hasCanonicalMatch(item) || !alias ? activePath : "";
}

/** Breadcrumbs should describe a page's canonical location, not its first cross-link. */
export function canonicalSidebar(items) {
  return items
    .filter((item) => !item.customProps?.yamlReference)
    .map((item) =>
      item.items ? { ...item, items: canonicalSidebar(item.items) } : item,
    );
}
