/**
 * Component that displays the release badge on blog post pages.
 * This component MUST only be rendered inside BlogPostProvider context.
 */
import React from 'react';
import { useBlogPost } from '@docusaurus/plugin-content-blog/client';
import clsx from 'clsx';
import styles from './styles.module.css';

function ReleaseBadge({ release }: { release: string }): JSX.Element {
  if (release === 'unreleased') {
    return (
      <span className={clsx(styles.releaseBadge, styles.unreleased)}>
        Unreleased
      </span>
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

export default function BlogReleaseBadge(): JSX.Element | null {
  // This hook will throw if not inside BlogPostProvider.
  // The parent component (TOC) ensures this is only rendered on blog post pages
  // where BlogPostProvider is available.
  const blogPost = useBlogPost();
  const release = blogPost?.frontMatter?.release as string | undefined;

  if (!release) {
    return null;
  }

  return (
    <div className={styles.releaseContainer}>
      <ReleaseBadge release={release} />
    </div>
  );
}
