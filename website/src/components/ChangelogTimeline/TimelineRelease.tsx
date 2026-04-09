import React from 'react';
import TimelineEntry from './TimelineEntry';
import type { ReleaseGroup } from './utils';
import styles from './styles.module.css';

interface TimelineReleaseProps {
  releaseGroup: ReleaseGroup;
  startIndex: number;
}

export default function TimelineRelease({
  releaseGroup,
  startIndex,
}: TimelineReleaseProps): JSX.Element {
  const { release, items } = releaseGroup;
  const isUnreleased = release === 'unreleased';
  const displayRelease = isUnreleased ? 'Unreleased' : release;

  return (
    <section className={styles.yearSection} aria-label={`${displayRelease} releases`}>
      <div className={styles.yearMarker}>
        {isUnreleased ? (
          <span className={styles.yearText}>{displayRelease}</span>
        ) : (
          <a
            href={`https://github.com/cloudposse/atmos/releases/tag/${release}`}
            className={styles.yearText}
            target="_blank"
            rel="noopener noreferrer"
            style={{ textDecoration: 'none', color: 'inherit' }}
          >
            {displayRelease}
          </a>
        )}
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
    </section>
  );
}
