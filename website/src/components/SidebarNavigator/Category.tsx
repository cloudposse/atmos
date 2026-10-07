import React, { useState } from "react";
import Link from "@docusaurus/Link";
import { Collapsible } from "@docusaurus/theme-common";
import DocSidebarItems from "@theme/DocSidebarItems";
import type { Props } from "@theme/DocSidebarItem/Category";
import { useNavigationContext } from "./context";
import { entryId } from "./state.mjs";
import { normalizePath } from "./navigation.mjs";

/** Controlled expansion survives section changes, browser history, and mobile remounts. */
export default function Category({
  item,
  activePath,
  onItemClick,
  level,
  index,
  ...props
}: Props): JSX.Element {
  const navigation = useNavigationContext()!;
  const id = entryId(item);
  const href = item.linkUnlisted ? undefined : item.href;
  const isCurrent = Boolean(
    activePath &&
    item.href &&
    normalizePath(item.href) === normalizePath(activePath),
  );
  const active =
    Boolean(activePath && navigation.entry?.startsWith(`${id}/`)) || isCurrent;
  const [filterExpanded, setFilterExpanded] = useState(true);
  const expanded = navigation.filtering
    ? filterExpanded
    : (navigation.expanded[id] ?? (active || !item.collapsed));
  const toggle = () =>
    navigation.filtering
      ? setFilterExpanded(!expanded)
      : navigation.toggle(id, !expanded);
  const categoryLabel =
    typeof item.label === "string"
      ? item.label
      : item.customProps?.title || "section";
  const label = <span>{item.label}</span>;
  return (
    <li
      className={`menu__list-item ${!expanded ? "menu__list-item--collapsed" : ""} ${item.className || ""}`}
    >
      <div
        className={`menu__list-item-collapsible ${isCurrent ? "menu__list-item-collapsible--active" : ""}`}
      >
        {href ? (
          <Link
            {...props}
            to={href}
            data-navigation-id={id}
            className={`menu__link menu__link--sublist ${active ? "menu__link--active" : ""}`}
            aria-current={isCurrent ? "page" : undefined}
            aria-expanded={expanded}
            onClick={(event) => {
              if (
                event.button !== 0 ||
                event.metaKey ||
                event.ctrlKey ||
                event.shiftKey ||
                event.altKey
              )
                return;
              if (isCurrent) {
                event.preventDefault();
                toggle();
              } else onItemClick?.(item);
            }}
          >
            {label}
          </Link>
        ) : (
          <button
            {...props}
            type="button"
            data-navigation-id={id}
            className={`menu__link menu__link--sublist menu__link--sublist-caret ${active ? "menu__link--active" : ""}`}
            aria-expanded={expanded}
            onClick={toggle}
          >
            {label}
          </button>
        )}
        {href && (
          <button
            type="button"
            className="clean-btn menu__caret"
            aria-expanded={expanded}
            aria-label={`${expanded ? "Collapse" : "Expand"} sidebar category '${categoryLabel}'`}
            onClick={toggle}
          />
        )}
      </div>
      <Collapsible
        lazy
        as="ul"
        className="menu__list"
        collapsed={!expanded}
      >
        <DocSidebarItems
          items={item.items}
          activePath={activePath}
          level={level + 1}
          tabIndex={expanded ? 0 : -1}
          onItemClick={onItemClick}
        />
      </Collapsible>
    </li>
  );
}
