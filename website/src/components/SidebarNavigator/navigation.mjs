/** Pure navigation operations shared by desktop and mobile. */
export function normalizePath(href = "") {
  return href.split(/[?#]/)[0].replace(/\/$/, "") || "/";
}

// A canonical doc/category owns a route. Cross-links in another section must
// not steal it (for example Steps -> Using steps -> Workflows).
export function findSection(items, activePath) {
  const path = normalizePath(activePath);
  function score(item) {
    const own =
      item.href && normalizePath(item.href) === path
        ? item.type === "category" || item.docId
          ? 2
          : 1
        : 0;
    return Math.max(own, ...(item.items || []).map(score));
  }
  let best = 0;
  let section = null;
  items.forEach((item, index) => {
    const match = score(item);
    if (match > best) {
      best = match;
      section = item.type === "category" ? index : null;
    }
  });
  return section;
}

export function prepareItems(items, activePath) {
  return items
    .filter(
      (item) =>
        !item.unlisted ||
        normalizePath(item.href) === normalizePath(activePath),
    )
    .map((item) =>
      item.type === "category"
        ? {
            ...item,
            collapsible: true,
            collapsed: true,
            items: prepareItems(item.items, activePath),
          }
        : item,
    );
}

export function filterItems(items, query, ancestors = []) {
  const terms = query.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean);
  if (!terms.length) return items;
  function expand(item) {
    return item.type === "category"
      ? { ...item, collapsed: false, items: item.items.map(expand) }
      : item;
  }
  return items.flatMap((item) => {
    if (item.type === "html") return [];
    const labels = [...ancestors, item.label];
    const text = labels.join(" ").toLocaleLowerCase();
    if (terms.every((term) => text.includes(term))) return [expand(item)];
    if (item.type !== "category") return [];
    const children = filterItems(item.items, query, labels);
    return children.length
      ? [{ ...item, collapsed: false, items: children }]
      : [];
  });
}

export function sidebarLabel(name) {
  return (
    {
      cli: "reference",
      docs: "Learn",
      tutorials: "tutorials",
      community: "community",
    }[name] || "sections"
  );
}
