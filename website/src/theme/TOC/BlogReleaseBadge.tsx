/**
 * Component that displays the release badge on blog post pages.
 * This component MUST only be rendered inside BlogPostProvider context.
 */
import React from 'react';
import { useBlogPost } from '@docusaurus/plugin-content-blog/client';
import useGlobalData from '@docusaurus/useGlobalData';
import ReleaseBadge from './ReleaseBadge';
import styles from './styles.module.css';

export default function BlogReleaseBadge(): JSX.Element | null {
  // This hook will throw if not inside BlogPostProvider.
  // The parent component (TOC) ensures this is only rendered on blog post pages
  // where BlogPostProvider is available.
  const blogPost = useBlogPost();
  const globalData = useGlobalData();
  const releaseMap = (
    globalData['blog-release-data']?.default as { releaseMap?: Record<string, string> } | undefined
  )?.releaseMap;

  // Same precedence as the blog-release-data plugin: a frontmatter override wins,
  // then the release detected from git tags. Most posts carry no `release`
  // frontmatter, so reading only the frontmatter hides every badge. A post missing
  // from the map has an unknown release, not an unreleased one, so show no badge.
  const release =
    (blogPost?.frontMatter?.release as string | undefined) ||
    releaseMap?.[blogPost?.metadata?.permalink];

  if (!release) {
    return null;
  }

  return (
    <div className={styles.releaseContainer}>
      <ReleaseBadge release={release} />
    </div>
  );
}
