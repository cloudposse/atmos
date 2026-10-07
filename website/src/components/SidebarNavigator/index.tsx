import React, { useEffect, useId, useMemo, useRef, useState } from "react";
import Link from "@docusaurus/Link";
import { FiArrowLeft, FiChevronRight, FiSearch, FiX } from "react-icons/fi";
import OriginalDocSidebarItems from "@theme-original/DocSidebarItems";
import type { Props } from "@theme/DocSidebarItems";
import type { PropSidebarItem } from "@docusaurus/plugin-content-docs";
import RouteStatusDot from "@site/src/components/FeatureStatusDot/RouteStatusDot";
import {
  filterItems,
  normalizePath,
  prepareItems,
  sidebarLabel,
} from "./navigation.mjs";
import {
  entryId,
  identifyItems,
  initialNavigation,
  locationUrl,
} from "./state.mjs";
import { NavigationContext, useNavigationStore } from "./context";
import { useLocation } from "@docusaurus/router";
import styles from "./styles.module.css";

type NavigatorProps = Props & { sidebarName: string };

/** Render focused menus while preserving each visited section's expansion state. */
export default function SidebarNavigator({
  items,
  activePath,
  onItemClick,
  sidebarName,
}: NavigatorProps): JSX.Element {
  const location = useLocation();
  const identified = useMemo(() => identifyItems(items), [items]);
  const prepared = useMemo(
    () => prepareItems(identified, activePath),
    [identified, activePath],
  );
  const { store, snapshot } = useNavigationStore(identified, sidebarName);
  const current =
    snapshot?.sidebar === sidebarName && snapshot.url === locationUrl(location)
      ? snapshot
      : initialNavigation(identified, sidebarName, locationUrl(location));
  const sectionIndex = prepared.findIndex(
    (item) => entryId(item) === current.section,
  );
  const section = sectionIndex < 0 ? null : sectionIndex;
  const [query, setQuery] = useState("");
  const filtering = query.trim().length > 0;
  const results = useMemo(
    () => filterItems(prepared, query),
    [prepared, query],
  );
  const inputId = useId();
  const rootRef = useRef<HTMLLIElement>(null);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const previousSection = useRef(section);

  useEffect(() => {
    const changed = previousSection.current !== section;
    previousSection.current = section;
    if (
      !changed ||
      filtering ||
      window.matchMedia("(prefers-reduced-motion: reduce)").matches
    )
      return;
    const panel = rootRef.current?.querySelector<HTMLElement>(
      section === null ? "[data-sections]" : `[data-section="${section}"]`,
    );
    const animation = panel?.animate(
      [
        {
          opacity: 0,
          transform: `translateX(${section === null ? -8 : 8}px)`,
          filter: "blur(2px)",
        },
        { opacity: 1, transform: "translateX(0)", filter: "blur(0)" },
      ],
      { duration: 200, easing: "ease-out" },
    );
    return () => animation?.cancel();
  }, [section, filtering]);

  useEffect(() => setQuery(""), [location.key]);

  // Each menu has its own scroll offset. A hidden desktop menu must not overwrite
  // the mobile drawer's saved position (or vice versa).
  const scrollKey = current.section || "index";
  const scrollPosition = useRef(current.scroll[scrollKey] || 0);
  scrollPosition.current = current.scroll[scrollKey] || 0;
  useEffect(() => {
    const menu = rootRef.current?.closest<HTMLElement>(".menu");
    if (!menu) return;
    menu.scrollTop = filtering ? 0 : scrollPosition.current;
    const saveScroll = () => {
      if (!menu.getClientRects().length || filtering) return;
      const saved = store.getSnapshot();
      if (
        !saved ||
        (saved.section || "index") !== scrollKey ||
        saved.scroll[scrollKey] === menu.scrollTop
      )
        return;
      store.update(
        {
          scroll: { ...saved.scroll, [scrollKey]: menu.scrollTop },
        },
        false,
      );
    };
    menu.addEventListener("scroll", saveScroll, { passive: true });
    return () => menu.removeEventListener("scroll", saveScroll);
  }, [store, scrollKey, location.key, filtering]);

  // Reveal only after navigation, not when clearing a filter or restoring a menu.
  const lastRevealed = useRef<string>();
  useEffect(() => {
    if (filtering || section === null) return;
    const target = `${location.key}/${current.entry}/${section}`;
    if (lastRevealed.current === target) return;
    lastRevealed.current = target;
    const timer = window.setTimeout(() => {
      const panel = rootRef.current?.querySelector<HTMLElement>(
        `[data-section="${section}"]`,
      );
      const active = panel?.querySelector<HTMLElement>('[aria-current="page"]');
      const menu = rootRef.current?.closest<HTMLElement>(".menu");
      if (!active || !menu || !active.getClientRects().length) return;
      const linkRect = active.getBoundingClientRect();
      const menuRect = menu.getBoundingClientRect();
      const toolbarHeight =
        rootRef.current?.querySelector<HTMLElement>(`.${styles.toolbar}`)
          ?.offsetHeight || 0;
      const top = menuRect.top + toolbarHeight + 8;
      const bottom = menuRect.bottom - 8;
      if (linkRect.top < top) menu.scrollTop += linkRect.top - top;
      else if (linkRect.bottom > bottom)
        menu.scrollTop += linkRect.bottom - bottom;
    }, 200);
    return () => window.clearTimeout(timer);
  }, [location.key, current.entry, section, filtering]);

  /** Announce a menu change after React renders its heading, without scrolling. */
  function focusHeading() {
    window.requestAnimationFrame(() =>
      headingRef.current?.focus({ preventScroll: true }),
    );
  }

  /** Selecting from the index starts a fresh navigation trail. */
  function selectSection(index: number) {
    const item = prepared[index];
    store.update({ section: entryId(item), entry: entryId(item), trail: [] });
    setQuery("");
    focusHeading();
  }

  function returnToSections() {
    store.update({ section: null, trail: [] });
    setQuery("");
    focusHeading();
  }

  // Docusaurus' leaf callback omits the event. Capture modifiers before it fires
  // so opening a new tab never changes this tab's navigation or mobile drawer.
  const modifiedClick = useRef(false);
  function navigate(item: PropSidebarItem) {
    if (modifiedClick.current || !("href" in item) || !item.href) return;
    const url = new URL(item.href, window.location.href);
    if (url.origin !== window.location.origin) return;
    setQuery("");
    store.intend(url.pathname + url.search + url.hash, entryId(item));
    onItemClick?.(item);
  }

  const selected = section === null ? null : prepared[section];
  const overview = prepared.find(
    (item) => item.type === "link" && item.customProps?.navigationOverview,
  );
  const headingLink =
    selected?.type === "category"
      ? selected.href && !selected.linkUnlisted
        ? selected
        : undefined
      : section === null && overview?.type === "link"
        ? overview
        : undefined;
  const headingLabel =
    selected?.label ||
    (sidebarName === "cli" && overview?.label) ||
    `All ${sidebarLabel(sidebarName)}`;
  const navigation = {
    entry: current.entry,
    expanded: current.expanded,
    filtering: false,
    toggle: (id: string, expanded: boolean) =>
      store.update({ expanded: { ...current.expanded, [id]: expanded } }),
  };
  return (
    <NavigationContext.Provider value={navigation}>
      <li
        ref={rootRef}
        className={styles.navigator}
        onClickCapture={(event) => {
          modifiedClick.current =
            event.button !== 0 ||
            event.metaKey ||
            event.ctrlKey ||
            event.shiftKey ||
            event.altKey;
        }}
      >
        <div className={styles.toolbar}>
          <label className={styles.srOnly} htmlFor={inputId}>
            Filter navigation
          </label>
          <div className={styles.filterRow}>
            <FiSearch
              className={styles.searchIcon}
              size={16}
              aria-hidden="true"
            />
            <input
              ref={inputRef}
              id={inputId}
              type="search"
              value={query}
              placeholder="Filter sidebar..."
              autoComplete="off"
              onChange={(event) => setQuery(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Escape" && query) {
                  event.preventDefault();
                  event.stopPropagation();
                  setQuery("");
                }
              }}
            />
            {query && (
              <button
                className={styles.clear}
                type="button"
                aria-label="Clear navigation filter"
                onClick={() => {
                  setQuery("");
                  inputRef.current?.focus();
                }}
              >
                <FiX size={14} aria-hidden="true" />
              </button>
            )}
          </div>
          {!filtering && section !== null && current.trail.length > 0 && (
            <button
              type="button"
              className={styles.back}
              onClick={() => {
                store.back();
                focusHeading();
              }}
            >
              <FiArrowLeft size={16} aria-hidden="true" />
              Back to{" "}
              {
                prepared.find(
                  (item) =>
                    entryId(item) ===
                    current.trail[current.trail.length - 1].section,
                )?.label
              }
            </button>
          )}
          {!filtering && section !== null && (
            <button
              type="button"
              className={styles.back}
              onClick={returnToSections}
            >
              <FiArrowLeft size={16} aria-hidden="true" />
              All {sidebarLabel(sidebarName)}
            </button>
          )}
          <h2
            ref={headingRef}
            tabIndex={-1}
            className={filtering ? styles.srOnly : styles.heading}
          >
            {filtering ? (
              `Matches in ${sidebarLabel(sidebarName)}`
            ) : headingLink ? (
              <Link
                className={styles.sectionOverview}
                to={headingLink.href}
                aria-current={
                  normalizePath(headingLink.href) === normalizePath(activePath)
                    ? "page"
                    : undefined
                }
                onClick={() =>
                  navigate({
                    type: "link",
                    label: headingLabel,
                    href: headingLink.href!,
                    customProps: headingLink.customProps,
                  })
                }
              >
                {headingLabel}
                <RouteStatusDot href={headingLink.href} />
              </Link>
            ) : (
              headingLabel
            )}
          </h2>
        </div>
        <div data-sections hidden={filtering || section !== null}>
          <ul className="menu__list">
            {prepared.map((item, index) =>
              item.customProps?.navigationOverview ? null : item.type ===
                "category" ? (
                <li key={index} className="menu__list-item">
                  {item.href && !item.linkUnlisted ? (
                    <Link
                      className={`menu__link ${styles.sectionLink}`}
                      to={item.href}
                      onClick={(event) => {
                        if (
                          event.button !== 0 ||
                          event.metaKey ||
                          event.ctrlKey ||
                          event.shiftKey ||
                          event.altKey
                        )
                          return;
                        selectSection(index);
                        // Load the overview through Link, but keep the drawer open
                        // so the selected section's local menu remains available.
                      }}
                    >
                      <span>
                        {item.label}
                        <RouteStatusDot href={item.href} />
                      </span>
                      <FiChevronRight
                        className={styles.sectionChevron}
                        size={16}
                        aria-hidden="true"
                      />
                    </Link>
                  ) : (
                    <button
                      type="button"
                      className={`menu__link ${styles.sectionLink}`}
                      onClick={() => selectSection(index)}
                    >
                      <span>{item.label}</span>
                      <FiChevronRight
                        className={styles.sectionChevron}
                        size={16}
                        aria-hidden="true"
                      />
                    </button>
                  )}
                </li>
              ) : (
                <OriginalDocSidebarItems
                  key={index}
                  items={[item]}
                  activePath={activePath}
                  level={2}
                  onItemClick={navigate}
                />
              ),
            )}
          </ul>
        </div>
        {prepared.map((item, index) =>
          item.type === "category" && section === index ? (
            <div
              key={index}
              data-section={index}
              hidden={filtering || section !== index}
            >
              <ul className="menu__list">
                <OriginalDocSidebarItems
                  items={item.items}
                  activePath={activePath}
                  level={2}
                  onItemClick={navigate}
                />
              </ul>
            </div>
          ) : null,
        )}
        {filtering && (
          <NavigationContext.Provider
            value={{ ...navigation, filtering: true }}
          >
            <div className={styles.results}>
              <p className={styles.srOnly} role="status">
                {results.length
                  ? "Matching navigation entries below."
                  : "No matching navigation entries."}
              </p>
              {results.length ? (
                <ul className="menu__list">
                  <OriginalDocSidebarItems
                    key={query}
                    items={results}
                    activePath={activePath}
                    level={2}
                    onItemClick={navigate}
                  />
                </ul>
              ) : (
                <p className={styles.empty}>
                  No matching pages. Try another term or use the site search to
                  search page contents.
                </p>
              )}
            </div>
          </NavigationContext.Provider>
        )}
      </li>
    </NavigationContext.Provider>
  );
}
