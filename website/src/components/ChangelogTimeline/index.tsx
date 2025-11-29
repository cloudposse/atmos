import React, { useState, useMemo } from 'react';
import FilterBar from './FilterBar';
import TimelineYear from './TimelineYear';
import {
  groupBlogPostsByYearMonth,
  extractYears,
  extractTags,
  filterBlogPosts,
  type BlogPostItem,
} from './utils';
import styles from './styles.module.css';

interface ChangelogTimelineProps {
  items: BlogPostItem[];
}

export default function ChangelogTimeline({
  items,
}: ChangelogTimelineProps): JSX.Element {
  const [selectedYear, setSelectedYear] = useState<string | null>(null);
  const [selectedTag, setSelectedTag] = useState<string | null>(null);

  // Extract available years and tags for the filter.
  const years = useMemo(() => extractYears(items), [items]);
  const tags = useMemo(() => extractTags(items), [items]);

  // Filter and group items.
  const filteredItems = useMemo(
    () => filterBlogPosts(items, selectedYear, selectedTag),
    [items, selectedYear, selectedTag]
  );

  const groupedItems = useMemo(
    () => groupBlogPostsByYearMonth(filteredItems),
    [filteredItems]
  );

  const hasResults = groupedItems.length > 0;

  return (
    <div className={styles.changelogTimeline}>
      <FilterBar
        years={years}
        tags={tags}
        selectedYear={selectedYear}
        selectedTag={selectedTag}
        onYearChange={setSelectedYear}
        onTagChange={setSelectedTag}
      />

      {hasResults ? (
        <div className={styles.timeline}>
          <div className={styles.timelineLine} aria-hidden="true" />
          {groupedItems.map((yearGroup) => (
            <TimelineYear key={yearGroup.year} yearGroup={yearGroup} />
          ))}
        </div>
      ) : (
        <div className={styles.emptyState}>
          <p>No changelog entries found matching your filters.</p>
          <button
            className={styles.resetButton}
            onClick={() => {
              setSelectedYear(null);
              setSelectedTag(null);
            }}
          >
            Clear filters
          </button>
        </div>
      )}
    </div>
  );
}

export { type BlogPostItem } from './utils';
