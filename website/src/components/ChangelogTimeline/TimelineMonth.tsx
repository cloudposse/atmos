import React from 'react';
import TimelineEntry from './TimelineEntry';
import type { MonthGroup } from './utils';
import styles from './styles.module.css';

interface TimelineMonthProps {
  monthGroup: MonthGroup;
  startIndex: number;
}

export default function TimelineMonth({
  monthGroup,
  startIndex,
}: TimelineMonthProps): JSX.Element {
  const { month, items } = monthGroup;

  return (
    <div className={styles.monthGroup}>
      <div className={styles.monthSeparator}>
        <span className={styles.monthText}>{month}</span>
      </div>
      <div className={styles.entriesContainer}>
        {items.map((item, index) => (
          <TimelineEntry
            key={item.content.metadata.permalink}
            item={item}
            position={(startIndex + index) % 2 === 0 ? 'left' : 'right'}
          />
        ))}
      </div>
    </div>
  );
}
