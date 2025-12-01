import React from 'react';
import styles from './styles.module.css';

interface FilterBarProps {
  years: string[];
  selectedYears: string[];
  onYearsChange: (years: string[]) => void;
  tags: string[];
  selectedTags: string[];
  onTagsChange: (tags: string[]) => void;
}

export default function FilterBar({
  years,
  selectedYears,
  onYearsChange,
  tags,
  selectedTags,
  onTagsChange,
}: FilterBarProps): JSX.Element {
  const hasActiveFilters = selectedYears.length > 0 || selectedTags.length > 0;

  const toggleYear = (year: string) => {
    if (selectedYears.includes(year)) {
      onYearsChange(selectedYears.filter((y) => y !== year));
    } else {
      onYearsChange([...selectedYears, year]);
    }
  };

  const toggleTag = (tag: string) => {
    if (selectedTags.includes(tag)) {
      onTagsChange(selectedTags.filter((t) => t !== tag));
    } else {
      onTagsChange([...selectedTags, tag]);
    }
  };

  return (
    <div className={styles.filterBarWrapper}>
      <div className={styles.filterBar}>
        <div className={styles.filterSection}>
        <span className={styles.filterLabel}>Year</span>
        <div className={styles.pillGroup}>
          {years.map((year) => (
            <button
              key={year}
              onClick={() => toggleYear(year)}
              className={`${styles.filterPill} ${selectedYears.includes(year) ? styles.filterPillActive : ''}`}
            >
              {year}
            </button>
          ))}
        </div>
      </div>
      <div className={styles.filterSection}>
        <span className={styles.filterLabel}>Tag</span>
        <div className={styles.pillGroup}>
          {tags.map((tag) => (
            <button
              key={tag}
              onClick={() => toggleTag(tag)}
              className={`${styles.filterPill} ${selectedTags.includes(tag) ? styles.filterPillActive : ''}`}
            >
              {tag}
            </button>
          ))}
        </div>
      </div>
      {hasActiveFilters && (
        <button
          className={styles.clearButton}
          onClick={() => {
            onYearsChange([]);
            onTagsChange([]);
          }}
        >
          Clear all
        </button>
      )}
      </div>
    </div>
  );
}
