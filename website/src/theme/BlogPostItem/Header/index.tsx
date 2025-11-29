/**
 * Swizzled BlogPostItem/Header to display release version badge.
 *
 * This component wraps the original header and adds a release badge
 * that links to the GitHub release page for the version specified
 * in the blog post's frontmatter.
 */
import React from 'react';
import OriginalHeader from '@theme-original/BlogPostItem/Header';
import styles from './styles.module.css';

// Safe hook to get blog post metadata - returns null if not in blog post context.
function useBlogPostSafe() {
  try {
    // Dynamic import to avoid SSR issues when context is not available.
    const { useBlogPost } = require('@docusaurus/theme-common/internal');
    return useBlogPost();
  } catch {
    return null;
  }
}

function ReleaseBadge({ release }: { release: string }): JSX.Element {
  if (release === 'unreleased') {
    return (
      <span className={`${styles.releaseBadge} ${styles.unreleased}`}>
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

export default function Header(props: React.ComponentProps<typeof OriginalHeader>): JSX.Element {
  const blogPost = useBlogPostSafe();
  const release = blogPost?.metadata?.frontMatter?.release as string | undefined;

  return (
    <div className={styles.headerWrapper}>
      <OriginalHeader {...props} />
      {release && (
        <div className={styles.releaseContainer}>
          <ReleaseBadge release={release} />
        </div>
      )}
    </div>
  );
}
