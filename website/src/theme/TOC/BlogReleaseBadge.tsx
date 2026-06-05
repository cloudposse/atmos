/**
 * Component that displays the release badge on blog post pages.
 * This component MUST only be rendered inside BlogPostProvider context.
 */
import React from 'react';
import { useBlogPost } from '@docusaurus/plugin-content-blog/client';
import ReleaseBadge from './ReleaseBadge';
import styles from './styles.module.css';

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
