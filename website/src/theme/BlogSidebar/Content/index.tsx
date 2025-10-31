/**
 * Copyright (c) Facebook, Inc. and its affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */
import React, {memo, useState} from 'react';
import {useThemeConfig} from '@docusaurus/theme-common';
import type {BlogSidebarItem} from '@docusaurus/plugin-content-blog';
import Heading from '@theme/Heading';

// Custom function to group by year and month.
function groupBlogSidebarItemsByYearMonth(
  items: BlogSidebarItem[],
): Map<string, Map<string, BlogSidebarItem[]>> {
  // Temporary structure to store month data with numeric values for sorting.
  interface MonthData {
    items: BlogSidebarItem[];
    monthNum: number;
  }
  const yearMonthMap = new Map<string, Map<string, MonthData>>();

  items.forEach((item) => {
    const date = new Date(item.date);

    // Validate date.
    if (isNaN(date.getTime())) {
      console.warn(`Invalid date for blog item: ${item.date}`);
      return;
    }

    const year = `${date.getFullYear()}`;
    // Use 'default' locale to respect site's i18n configuration.
    const month = date.toLocaleString('default', { month: 'long' });
    const monthNum = date.getMonth(); // 0-11

    if (!yearMonthMap.has(year)) {
      yearMonthMap.set(year, new Map());
    }

    const monthMap = yearMonthMap.get(year)!;
    if (!monthMap.has(month)) {
      monthMap.set(month, { items: [], monthNum });
    }

    monthMap.get(month)!.items.push(item);
  });

  // Sort years descending.
  const sortedYears = Array.from(yearMonthMap.keys()).sort((a, b) => parseInt(b) - parseInt(a));
  const result = new Map<string, Map<string, BlogSidebarItem[]>>();

  sortedYears.forEach((year) => {
    const monthMap = yearMonthMap.get(year)!;
    // Sort months chronologically within each year (descending) using numeric month values.
    const sortedMonths = Array.from(monthMap.entries()).sort((a, b) => {
      return b[1].monthNum - a[1].monthNum; // Descending order (11, 10, ..., 0)
    });

    const sortedMonthMap = new Map<string, BlogSidebarItem[]>();
    sortedMonths.forEach(([month, monthData]) => {
      sortedMonthMap.set(month, monthData.items);
    });

    result.set(year, sortedMonthMap);
  });

  return result;
}

function BlogSidebarYearGroup({year, yearGroupHeadingClassName, children}: {
  year: string;
  yearGroupHeadingClassName?: string;
  children: React.ReactNode;
}) {
  return (
    <div role="group">
      <Heading as="h3" className={yearGroupHeadingClassName}>
        {year}
      </Heading>
      {children}
    </div>
  );
}

function CollapsibleMonthGroup({
  year,
  month,
  monthGroupHeadingClassName,
  children,
  isDefaultOpen,
}: {
  year: string;
  month: string;
  monthGroupHeadingClassName?: string;
  children: React.ReactNode;
  isDefaultOpen: boolean;
}) {
  const [isOpen, setIsOpen] = useState(isDefaultOpen);

  return (
    <div role="group" style={{marginLeft: '0.5rem'}}>
      <button
        onClick={() => setIsOpen(!isOpen)}
        style={{
          display: 'flex',
          alignItems: 'center',
          width: '100%',
          background: 'none',
          border: 'none',
          padding: 0,
          cursor: 'pointer',
          textAlign: 'left',
          marginTop: '0.5rem',
          marginBottom: '0.5rem',
        }}
        aria-expanded={isOpen}
      >
        <svg
          viewBox="0 0 16 16"
          width="12"
          height="12"
          style={{
            marginRight: '0.5rem',
            transition: 'transform 0.2s',
            transform: isOpen ? 'rotate(90deg)' : 'rotate(0deg)',
          }}
        >
          <path
            fill="currentColor"
            d="M4.646 1.646a.5.5 0 0 1 .708 0l6 6a.5.5 0 0 1 0 .708l-6 6a.5.5 0 0 1-.708-.708L10.293 8 4.646 2.354a.5.5 0 0 1 0-.708z"
          />
        </svg>
        <Heading
          as="h4"
          className={monthGroupHeadingClassName}
          style={{
            fontSize: '0.875rem',
            margin: 0,
            fontWeight: 600,
            textTransform: 'uppercase',
            letterSpacing: '0.05em',
            color: 'var(--ifm-color-gray-600)',
            opacity: 0.8,
          }}
        >
          {month}
        </Heading>
      </button>
      {isOpen && <div>{children}</div>}
    </div>
  );
}

function BlogSidebarContent({items, yearGroupHeadingClassName, ListComponent}: {
  items: BlogSidebarItem[];
  yearGroupHeadingClassName?: string;
  ListComponent: React.ComponentType<{items: BlogSidebarItem[]}>;
}) {
  const themeConfig = useThemeConfig();
  if (themeConfig.blog.sidebar.groupByYear) {
    const itemsByYearMonth = groupBlogSidebarItemsByYearMonth(items);

    // Determine the most recent month (first in iteration order).
    let firstMonthKey: string | null = null;
    for (const [year, monthMap] of itemsByYearMonth.entries()) {
      for (const month of monthMap.keys()) {
        firstMonthKey = `${year}-${month}`;
        break;
      }
      if (firstMonthKey) break;
    }

    return (
      <>
        {Array.from(itemsByYearMonth.entries()).map(([year, monthMap]) => (
          <BlogSidebarYearGroup
            key={year}
            year={year}
            yearGroupHeadingClassName={yearGroupHeadingClassName}>
            {Array.from(monthMap.entries()).map(([month, monthItems]) => {
              const monthKey = `${year}-${month}`;
              const isDefaultOpen = monthKey === firstMonthKey;

              return (
                <CollapsibleMonthGroup
                  key={monthKey}
                  year={year}
                  month={month}
                  monthGroupHeadingClassName={yearGroupHeadingClassName}
                  isDefaultOpen={isDefaultOpen}>
                  <ListComponent items={monthItems} />
                </CollapsibleMonthGroup>
              );
            })}
          </BlogSidebarYearGroup>
        ))}
      </>
    );
  } else {
    return <ListComponent items={items} />;
  }
}
export default memo(BlogSidebarContent);
