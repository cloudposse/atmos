import React from 'react';
import clsx from 'clsx';
import styles from './styles.module.css';

interface FilterBarProps {
  years: string[];
  tags: string[];
  selectedYear: string | null;
  selectedTag: string | null;
  onYearChange: (year: string | null) => void;
  onTagChange: (tag: string | null) => void;
}

export default function FilterBar({
  years,
  tags,
  selectedYear,
  selectedTag,
  onYearChange,
  onTagChange,
}: FilterBarProps): JSX.Element {
  return (
    <div className={styles.filterBar}>
      <div className={styles.filterRow}>
        <div className={styles.yearFilters} role="group" aria-label="Filter by year">
          <button
            className={clsx(styles.filterPill, !selectedYear && styles.filterPillActive)}
            onClick={() => onYearChange(null)}
            aria-pressed={!selectedYear}
          >
            All
          </button>
          {years.map((year) => (
            <button
              key={year}
              className={clsx(
                styles.filterPill,
                selectedYear === year && styles.filterPillActive
              )}
              onClick={() => onYearChange(year)}
              aria-pressed={selectedYear === year}
            >
              {year}
            </button>
          ))}
        </div>

        <div className={styles.tagFilter}>
          <label htmlFor="tag-filter" className={styles.tagFilterLabel}>
            Filter by:
          </label>
          <select
            id="tag-filter"
            value={selectedTag || ''}
            onChange={(e) => onTagChange(e.target.value || null)}
            className={styles.tagSelect}
            aria-label="Filter by tag"
          >
            <option value="">All Tags</option>
            {tags.map((tag) => (
              <option key={tag} value={tag}>
                {tag}
              </option>
            ))}
          </select>
        </div>
      </div>
    </div>
  );
}
