import React from 'react';
import styles from './styles.module.css';

interface FilterBarProps {
  tags: string[];
  selectedTag: string | null;
  onTagChange: (tag: string | null) => void;
}

export default function FilterBar({
  tags,
  selectedTag,
  onTagChange,
}: FilterBarProps): JSX.Element {
  return (
    <div className={styles.filterBar}>
      <div className={styles.filterRow}>
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
