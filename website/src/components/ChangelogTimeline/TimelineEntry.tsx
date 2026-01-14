import React from 'react';
import Link from '@docusaurus/Link';
import clsx from 'clsx';
import type { BlogPostItem } from './utils';
import { getTagColorClass } from './utils';
import styles from './styles.module.css';

interface TimelineEntryProps {
  item: BlogPostItem;
  position: 'left' | 'right';
}

export default function TimelineEntry({
  item,
  position,
}: TimelineEntryProps): JSX.Element {
  // Docusaurus provides metadata and frontMatter as siblings on content.
  const { metadata } = item.content;
  const { title, permalink, date, tags = [], description } = metadata;

  const formattedDate = new Date(date).toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
  });

  return (
    <article
      className={clsx(styles.entry, styles[`entry${position.charAt(0).toUpperCase() + position.slice(1)}`])}
      aria-label={title}
    >
      <div className={styles.entryNode} aria-hidden="true" />
      <div className={styles.entryConnector} aria-hidden="true" />
      <div className={styles.entryCard}>
        <div className={styles.entryHeader}>
          <time dateTime={date} className={styles.entryDate}>
            {formattedDate}
          </time>
          {tags.length > 0 && (
            <div className={styles.entryTags}>
              {tags.slice(0, 3).map((tag) => (
                <span
                  key={tag.label}
                  className={clsx(styles.tag, styles[getTagColorClass(tag.label)])}
                >
                  {tag.label}
                </span>
              ))}
            </div>
          )}
        </div>
        <Link to={permalink} className={styles.entryTitle}>
          <h3>{title}</h3>
        </Link>
        {description && (
          <p className={styles.entryExcerpt}>{description}</p>
        )}
      </div>
    </article>
  );
}
