import { findSection, normalizePath } from "./navigation.mjs";

export const historyStateKey = "atmosSidebar";
/** Read an occurrence's tree identity, which remains stable when filtering. */
export const entryId = (item) => item.customProps?.navigationId;
const reference = (item) =>
  item.customProps?.yamlReference || item.customProps?.navigationReference;

/** Identify occurrences before filtering, so aliases and search results keep their identity. */
export function identifyItems(items, parent = "") {
  const counts = new Map();
  return items.map((item) => {
    const label = encodeURIComponent(item.label || item.type);
    const count = counts.get(label) || 0;
    counts.set(label, count + 1);
    const id = `${parent}/${label}:${count}`;
    return {
      ...item,
      customProps: { ...item.customProps, navigationId: id },
      ...(item.items ? { items: identifyItems(item.items, id) } : {}),
    };
  });
}

/** Walk entries in sidebar order, retaining each occurrence of a shared URL. */
export function flattenItems(items) {
  return items.flatMap((item) => [item, ...flattenItems(item.items || [])]);
}

/** Resolve an explicit occurrence first, then fall back to a canonical link. */
export function findEntry(items, url, preferred) {
  const matches = flattenItems(items).filter(
    (item) => item.href && normalizePath(item.href) === normalizePath(url),
  );
  return (
    matches.find((item) => entryId(item) === preferred) ||
    matches.find((item) => !reference(item)) ||
    matches[0]
  );
}

/** Create canonical navigation for a direct arrival without a return trail. */
export function initialNavigation(items, sidebar, url) {
  const sectionIndex = findSection(items, url);
  const section = sectionIndex === null ? null : entryId(items[sectionIndex]);
  const entry = findEntry(
    sectionIndex === null ? items : [items[sectionIndex]],
    url,
  );
  return expandEntry({
    version: 1,
    sidebar,
    url,
    section,
    entry: entryId(entry || {}),
    expanded: {},
    scroll: {},
    trail: [],
  });
}

/** Open the selected entry's ancestors without closing unrelated branches. */
function expandEntry(state) {
  const expanded = { ...state.expanded };
  const parts = state.entry?.split("/") || [];
  for (let i = 2; i <= parts.length; i++)
    expanded[parts.slice(0, i).join("/")] = true;
  return { ...state, expanded };
}

/** Reject saved context belonging to another URL, sidebar, or obsolete tree. */
export function validNavigation(state, items, sidebar, url) {
  if (
    !state ||
    state.version !== 1 ||
    state.sidebar !== sidebar ||
    state.url !== url ||
    !state.expanded ||
    !state.scroll ||
    !Array.isArray(state.trail)
  )
    return false;
  const ids = new Set(flattenItems(items).map(entryId));
  return (
    (state.section === null ||
      items.some((item) => entryId(item) === state.section)) &&
    (!state.entry || ids.has(state.entry)) &&
    state.trail.every(
      (frame) =>
        frame &&
        typeof frame.url === "string" &&
        frame.url.startsWith("/") &&
        !frame.url.startsWith("//") &&
        items.some((item) => entryId(item) === frame.section),
    )
  );
}

/** A route transition preserves local aliases and unwinds previously visited sections. */
export function transitionNavigation(previous, items, sidebar, url, intent) {
  const canonical = initialNavigation(items, sidebar, url);
  if (!previous || previous.sidebar !== sidebar) return canonical;
  if (!intent && normalizePath(previous.url) === normalizePath(url)) {
    return { ...previous, url };
  }
  const target = items.find((item) => entryId(item) === canonical.section);
  const preferred = intent?.url === url ? intent.entry : undefined;
  let entry = findEntry(target ? [target] : items, url, preferred);
  // Content links within the same branch should not unexpectedly select another alias.
  if (!preferred && canonical.section === previous.section && previous.entry) {
    const candidates = flattenItems(target ? [target] : items).filter(
      (item) => item.href && normalizePath(item.href) === normalizePath(url),
    );
    const proximity = (item) => {
      const a = entryId(item).split("/");
      const b = previous.entry.split("/");
      let n = 0;
      while (n < a.length && a[n] === b[n]) n++;
      return n;
    };
    candidates.sort((a, b) => proximity(b) - proximity(a));
    entry = candidates[0] || entry;
  }
  let trail = previous.trail;
  if (canonical.section !== previous.section) {
    const earlier = trail.findIndex(
      (frame) => frame.section === canonical.section,
    );
    if (earlier >= 0) trail = trail.slice(0, earlier);
    else if (previous.section && canonical.section) {
      const { trail: ignored, ...frame } = previous;
      trail = [...trail, frame];
    } else trail = [];
  }
  return expandEntry({
    ...canonical,
    entry: entryId(entry || {}),
    expanded: previous.expanded,
    scroll: previous.scroll,
    trail,
  });
}

/** Suppress URL-based highlighting outside the selected occurrence's branch. */
export function activeEntryPath(item, activePath, selected) {
  if (!selected) return activePath;
  const id = entryId(item);
  return id && (selected === id || selected.startsWith(`${id}/`))
    ? activePath
    : "";
}

/** Include query and fragment when identifying a browser history destination. */
export const locationUrl = (location) =>
  location.pathname + (location.search || "") + (location.hash || "");

/** Shared by desktop and mobile; snapshots belong to individual browser history entries. */
export function createNavigationStore(history, persistence) {
  let items = [];
  let sidebar = "";
  let snapshot = null;
  let location = history.location;
  let intent;
  let unlisten;
  let persistTimer;
  const listeners = new Set();
  const entries = new Map();
  const emit = () => listeners.forEach((listener) => listener());
  /** Cache immediately, debouncing native history writes only for scroll updates. */
  function save(persist = true) {
    entries.set(location.key, snapshot);
    clearTimeout(persistTimer);
    if (persist) persistence?.write?.(snapshot);
    else persistTimer = setTimeout(() => persistence?.write?.(snapshot), 150);
  }
  /** Restore a visited history entry or derive context for a new destination. */
  function arrive(next, action) {
    location = next;
    const url = locationUrl(next);
    const saved = entries.get(next.key) || next.state?.[historyStateKey];
    // Another sidebar registers after the route renders. Do not overwrite its
    // history snapshot using the departing sidebar's tree in the meantime.
    snapshot =
      saved?.version === 1 && saved.url === url && saved.sidebar !== sidebar
        ? saved
        : validNavigation(saved, items, sidebar, url)
          ? saved
          : transitionNavigation(
              action === "POP" &&
                normalizePath(snapshot?.url) !== normalizePath(url)
                ? null
                : snapshot,
              items,
              sidebar,
              url,
              intent,
            );
    intent = undefined;
    save();
    emit();
  }
  const store = {
    getSnapshot: () => snapshot,
    /** Share one router listener across desktop and mobile subscribers. */
    subscribe(listener) {
      listeners.add(listener);
      if (!unlisten) unlisten = history.listen(arrive);
      return () => {
        listeners.delete(listener);
        if (!listeners.size) {
          clearTimeout(persistTimer);
          unlisten?.();
          unlisten = undefined;
        }
      };
    },
    /** Initialize the current route from saved history or canonical navigation. */
    register(nextItems, nextSidebar) {
      items = nextItems;
      sidebar = nextSidebar;
      const current = history.location;
      const url = locationUrl(current);
      if (validNavigation(snapshot, items, sidebar, url)) return;
      location = current;
      const saved =
        entries.get(current.key) ||
        current.state?.[historyStateKey] ||
        persistence?.read?.();
      snapshot = validNavigation(saved, items, sidebar, url)
        ? saved
        : initialNavigation(items, sidebar, url);
      save();
      emit();
    },
    /** Persist a partial snapshot, optionally avoiding scroll-driven rerenders. */
    update(patch, notify = true) {
      if (!snapshot) return;
      snapshot = { ...snapshot, ...patch };
      save(notify);
      if (notify) emit();
    },
    /** Apply even pre-effect toggles to the current route's latest expansion map. */
    setExpanded(nextItems, nextSidebar, id, expanded) {
      if (
        snapshot?.sidebar !== nextSidebar ||
        snapshot.url !== locationUrl(history.location)
      ) {
        store.register(nextItems, nextSidebar);
      }
      store.update({ expanded: { ...snapshot.expanded, [id]: expanded } });
    },
    /** Remember clicked aliases, including switches between identical URLs. */
    intend(url, entry) {
      intent = { url, entry };
      if (snapshot?.url === url) {
        snapshot = transitionNavigation(snapshot, items, sidebar, url, intent);
        intent = undefined;
        save();
        emit();
      }
    },
    /** Restore the previous section's article and state while shortening the trail. */
    back() {
      if (!snapshot?.trail.length) return;
      const frame = snapshot.trail[snapshot.trail.length - 1];
      history.push(frame.url, {
        [historyStateKey]: { ...frame, trail: snapshot.trail.slice(0, -1) },
      });
    },
  };
  return store;
}
