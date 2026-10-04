import React, { useEffect, useId, useMemo, useRef, useState } from "react";
import Link from "@docusaurus/Link";
import OriginalDocSidebarItems from "@theme-original/DocSidebarItems";
import type { Props } from "@theme/DocSidebarItems";
import type { PropSidebarItem } from "@docusaurus/plugin-content-docs";
import ExperimentalDot from "@site/src/components/ExperimentalDot";
import { isExperimentalRoute } from "@site/src/data/experimentalRoutes";
import {
  filterItems,
  findSection,
  normalizePath,
  prepareItems,
  sidebarLabel,
} from "./navigation.mjs";
import styles from "./styles.module.css";

type NavigatorProps = Props & { sidebarName: string };

export default function SidebarNavigator({
  items,
  activePath,
  onItemClick,
  sidebarName,
}: NavigatorProps): JSX.Element {
  const prepared = useMemo(
    () => prepareItems(items, activePath),
    [items, activePath],
  );
  const destination = findSection(prepared, activePath);
  const [view, setView] = useState<{ path: string; section: number | null }>({
    path: activePath,
    section: destination,
  });
  const [visited, setVisited] = useState<number[]>(
    destination === null ? [] : [destination],
  );
  const [query, setQuery] = useState("");
  const section = view.path === activePath ? view.section : destination;
  const filtering = view.path === activePath && query.trim().length > 0;
  const results = useMemo(
    () => filterItems(prepared, query),
    [prepared, query],
  );
  const inputId = useId();
  const rootRef = useRef<HTMLLIElement>(null);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    setView({ path: activePath, section: destination });
    setQuery("");
    if (destination !== null)
      setVisited((previous) =>
        previous.includes(destination) ? previous : [...previous, destination],
      );
  }, [activePath, destination]);

  // Only reveal the active link in the visible menu. Never scroll the article
  // or move the reader away from a filter or the all-sections view.
  useEffect(() => {
    if (filtering || section === null) return;
    const timer = window.setTimeout(() => {
      const panel = rootRef.current?.querySelector<HTMLElement>(
        `[data-section="${section}"]`,
      );
      const active = panel?.querySelector<HTMLElement>('[aria-current="page"]');
      const menu = rootRef.current?.closest<HTMLElement>(".menu");
      if (!active || !menu || active.getClientRects().length === 0) return;
      const linkRect = active.getBoundingClientRect();
      const menuRect = menu.getBoundingClientRect();
      const toolbarHeight =
        rootRef.current?.querySelector<HTMLElement>(`.${styles.toolbar}`)
          ?.offsetHeight || 0;
      const top = menuRect.top + toolbarHeight;
      if (linkRect.top < top || linkRect.bottom > menuRect.bottom) {
        menu.scrollTop += linkRect.top - top - 8;
      }
    }, 180);
    return () => window.clearTimeout(timer);
  }, [activePath, section, filtering]);

  function focusHeading() {
    window.requestAnimationFrame(() =>
      headingRef.current?.focus({ preventScroll: true }),
    );
  }

  function selectSection(index: number) {
    setView({ path: activePath, section: index });
    setVisited((previous) =>
      previous.includes(index) ? previous : [...previous, index],
    );
    setQuery("");
    const menu = rootRef.current?.closest<HTMLElement>(".menu");
    if (menu) menu.scrollTop = 0;
    focusHeading();
  }

  function returnToSections() {
    setView({ path: activePath, section: null });
    setQuery("");
    const menu = rootRef.current?.closest<HTMLElement>(".menu");
    if (menu) menu.scrollTop = 0;
    focusHeading();
  }

  function navigate(item: PropSidebarItem) {
    // Native category toggles also invoke this callback. Only article navigation
    // should close the mobile drawer or clear the filter.
    const followsLink =
      item.type === "link" ||
      (item.type === "category" &&
        item.href &&
        normalizePath(item.href) !== normalizePath(activePath));
    if (!followsLink) return;
    setQuery("");
    if ("href" in item && item.href) {
      const targetSection = findSection(prepared, item.href);
      setView({ path: activePath, section: targetSection });
      if (targetSection !== null)
        setVisited((previous) =>
          previous.includes(targetSection)
            ? previous
            : [...previous, targetSection],
        );
    }
    onItemClick?.(item);
  }

  const selected = section === null ? null : prepared[section];
  const mounted = new Set([...visited, ...(section === null ? [] : [section])]);
  return (
    <li ref={rootRef} className={styles.navigator}>
      <div className={styles.toolbar}>
        <label className={styles.srOnly} htmlFor={inputId}>
          Filter navigation
        </label>
        <div className={styles.filterRow}>
          <svg
            className={styles.searchIcon}
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.75"
            strokeLinecap="round"
            aria-hidden="true"
          >
            <circle cx="10.5" cy="10.5" r="6.5" />
            <path d="m16 16 4.5 4.5" />
          </svg>
          <input
            ref={inputRef}
            id={inputId}
            type="search"
            value={query}
            placeholder="Filter sidebar…"
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
              <svg
                width="14"
                height="14"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.75"
                strokeLinecap="round"
                aria-hidden="true"
              >
                <path d="m6 6 12 12M6 18 18 6" />
              </svg>
            </button>
          )}
        </div>
        {!filtering && section !== null && (
          <button
            type="button"
            className={styles.back}
            onClick={returnToSections}
          >
            ← All {sidebarLabel(sidebarName)}
          </button>
        )}
        <h2
          ref={headingRef}
          tabIndex={-1}
          className={filtering ? styles.srOnly : styles.heading}
        >
          {filtering
            ? `Matches in ${sidebarLabel(sidebarName)}`
            : selected?.label || `All ${sidebarLabel(sidebarName)}`}
        </h2>
      </div>
      <div hidden={filtering || section !== null}>
        <ul className="menu__list">
          {prepared.map((item, index) =>
            item.type === "category" ? (
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
                      onItemClick?.(item);
                    }}
                  >
                    <span>
                      {item.label}
                      {isExperimentalRoute(item.href) && <ExperimentalDot />}
                    </span>
                    <span aria-hidden="true">›</span>
                  </Link>
                ) : (
                  <button
                    type="button"
                    className={`menu__link ${styles.sectionLink}`}
                    onClick={() => selectSection(index)}
                  >
                    <span>{item.label}</span>
                    <span aria-hidden="true">›</span>
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
        item.type === "category" && mounted.has(index) ? (
          <div
            key={index}
            data-section={index}
            hidden={filtering || section !== index}
          >
            <ul className="menu__list">
              <OriginalDocSidebarItems
                items={[
                  ...(item.href && !item.linkUnlisted
                    ? [
                        {
                          type: "link" as const,
                          label: "Overview",
                          href: item.href,
                        },
                      ]
                    : []),
                  ...item.items,
                ]}
                activePath={activePath}
                level={2}
                onItemClick={navigate}
              />
            </ul>
          </div>
        ) : null,
      )}
      {filtering && (
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
      )}
    </li>
  );
}
