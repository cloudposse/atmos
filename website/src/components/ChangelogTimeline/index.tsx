import React, { useState, useMemo } from 'react';
import FilterBar from './FilterBar';
import TimelineRelease from './TimelineRelease';
import {
  groupBlogPostsByRelease,
  extractTags,
  extractYears,
  filterBlogPostsMulti,
  type BlogPostItem,
} from './utils';
import styles from './styles.module.css';

interface ChangelogTimelineProps {
  items: BlogPostItem[];
}

export default function ChangelogTimeline({
  items,
}: ChangelogTimelineProps): JSX.Element {
  const [selectedYears, setSelectedYears] = useState<string[]>([]);
  const [selectedTags, setSelectedTags] = useState<string[]>([]);

  // Extract available years and tags for the filters.
  const years = useMemo(() => extractYears(items), [items]);
  const tags = useMemo(() => extractTags(items), [items]);

  // Filter by years and tags, then group by release version.
  const filteredItems = useMemo(
    () => filterBlogPostsMulti(items, selectedYears, selectedTags),
    [items, selectedYears, selectedTags]
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
        years={years}
        selectedYears={selectedYears}
        onYearsChange={setSelectedYears}
        tags={tags}
        selectedTags={selectedTags}
        onTagsChange={setSelectedTags}
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
              setSelectedYears([]);
              setSelectedTags([]);
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
