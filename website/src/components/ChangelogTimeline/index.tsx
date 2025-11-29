import React, { useState, useMemo } from 'react';
import FilterBar from './FilterBar';
import TimelineRelease from './TimelineRelease';
import {
  groupBlogPostsByRelease,
  extractTags,
  filterBlogPostsByTag,
  type BlogPostItem,
} from './utils';
import styles from './styles.module.css';

interface ChangelogTimelineProps {
  items: BlogPostItem[];
}

export default function ChangelogTimeline({
  items,
}: ChangelogTimelineProps): JSX.Element {
  const [selectedTag, setSelectedTag] = useState<string | null>(null);

  // Extract available tags for the filter.
  const tags = useMemo(() => extractTags(items), [items]);

  // Filter by tag, then group by release version.
  const filteredItems = useMemo(
    () => filterBlogPostsByTag(items, selectedTag),
    [items, selectedTag]
  );

  const groupedItems = useMemo(
    () => groupBlogPostsByRelease(filteredItems),
    [filteredItems]
  );

  const hasResults = groupedItems.length > 0;

  // Calculate running index for alternating positions across releases.
  let runningIndex = 0;

  return (
    <div className={styles.changelogTimeline}>
      <FilterBar
        tags={tags}
        selectedTag={selectedTag}
        onTagChange={setSelectedTag}
      />

      {hasResults ? (
        <div className={styles.timeline}>
          <div className={styles.timelineLine} aria-hidden="true" />
          {groupedItems.map((releaseGroup) => {
            const startIndex = runningIndex;
            runningIndex += releaseGroup.items.length;
            return (
              <TimelineRelease
                key={releaseGroup.release}
                releaseGroup={releaseGroup}
                startIndex={startIndex}
              />
            );
          })}
        </div>
      ) : (
        <div className={styles.emptyState}>
          <p>No changelog entries found matching your filters.</p>
          <button
            className={styles.resetButton}
            onClick={() => {
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
