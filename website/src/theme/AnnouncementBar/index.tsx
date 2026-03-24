/**
 * Swizzled AnnouncementBar component.
 * Replaces the default single-announcement bar with a multi-announcement
 * queue driven by website/src/data/announcements.js.
 * Dismissing one announcement reveals the next in the queue.
 */
import React from 'react';
import clsx from 'clsx';
import {useMultiAnnouncement} from './useMultiAnnouncement';
import styles from './styles.module.css';

export default function AnnouncementBar(): JSX.Element | null {
  const {current, dismiss, isActive} = useMultiAnnouncement();

  if (!isActive || !current) {
    return null;
  }

  const {
    backgroundColor = 'var(--announcement-bar-background)',
    textColor = 'var(--announcement-bar-text-color)',
    isCloseable = true,
    content,
  } = current;

  return (
    <div
      className={clsx('theme-announcement-bar', styles.announcementBar)}
      style={{backgroundColor, color: textColor}}
      role="banner">
      {isCloseable && <div className={styles.placeholder} />}
      <div className={styles.contentWrapper}>
        <div
          className={styles.content}
          // Same approach as Docusaurus default AnnouncementBar.
          dangerouslySetInnerHTML={{__html: content}}
        />
      </div>
      {isCloseable && (
        <button
          type="button"
          className={clsx('clean-btn close', styles.closeButton)}
          onClick={dismiss}
          aria-label="Close">
          <span aria-hidden="true">&times;</span>
        </button>
      )}
    </div>
  );
}
