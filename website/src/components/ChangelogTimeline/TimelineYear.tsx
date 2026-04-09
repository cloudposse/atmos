import React from 'react';
import TimelineMonth from './TimelineMonth';
import type { YearGroup } from './utils';
import styles from './styles.module.css';

interface TimelineYearProps {
  yearGroup: YearGroup;
}

export default function TimelineYear({
  yearGroup,
}: TimelineYearProps): JSX.Element {
  const { year, months } = yearGroup;

  // Calculate running index for alternating positions across months.
  let runningIndex = 0;

  return (
    <section className={styles.yearSection} aria-label={`${year} releases`}>
      <div className={styles.yearMarker}>
        <span className={styles.yearText}>{year}</span>
      </div>
      {months.map((monthGroup) => {
        const startIndex = runningIndex;
        runningIndex += monthGroup.items.length;
        return (
          <TimelineMonth
            key={`${year}-${monthGroup.month}`}
            monthGroup={monthGroup}
            startIndex={startIndex}
          />
        );
      })}
    </section>
  );
}
