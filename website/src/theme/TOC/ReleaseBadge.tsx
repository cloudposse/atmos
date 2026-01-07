/**
 * Shared ReleaseBadge component used by both BlogReleaseBadge and DocReleaseBadge.
 * Displays a release version badge or an "Unreleased" indicator.
 */
import React from 'react';
import Link from '@docusaurus/Link';
import clsx from 'clsx';
import styles from './styles.module.css';

interface ReleaseBadgeProps {
  release: string;
  /** Link destination for unreleased badge. Defaults to /unreleased */
  unreleasedLink?: string;
}

export default function ReleaseBadge({
  release,
  unreleasedLink = '/unreleased',
}: ReleaseBadgeProps): JSX.Element {
  if (release === 'unreleased') {
    return (
      <Link
        to={unreleasedLink}
        className={clsx(styles.releaseBadge, styles.unreleased)}
        title="This content has changes not yet included in a release"
      >
        Unreleased
      </Link>
    );
  }

  return (
    <a
      href={`https://github.com/cloudposse/atmos/releases/tag/${release}`}
      className={styles.releaseBadge}
      target="_blank"
      rel="noopener noreferrer"
    >
      {release}
    </a>
  );
}
