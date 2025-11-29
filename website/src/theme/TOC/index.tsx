/**
 * Swizzled TOC component to display release version badge at the top.
 */
import React from 'react';
import clsx from 'clsx';
import TOCItems from '@theme/TOCItems';
import type { Props } from '@theme/TOC';
import styles from './styles.module.css';

const LINK_CLASS_NAME = 'table-of-contents__link toc-highlight';
const LINK_ACTIVE_CLASS_NAME = 'table-of-contents__link--active';

// Try to import the blog hook at module scope. If the blog plugin isn't
// available, we use a no-op hook.
let useBlogPostRaw: (() => { frontMatter?: Record<string, unknown> }) | null =
  null;
try {
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const blogClient = require('@docusaurus/plugin-content-blog/client');
  useBlogPostRaw = blogClient.useBlogPost;
} catch {
  // Blog plugin not available.
}

// Safe wrapper hook that handles both missing plugin and missing provider.
// This is always called unconditionally in the component to satisfy Rules of Hooks.
function useBlogPostSafe(): { frontMatter?: Record<string, unknown> } | null {
  // If the blog plugin isn't available, return null.
  if (!useBlogPostRaw) {
    return null;
  }

  // Call the hook - it may throw if BlogPostProvider is missing (non-blog pages).
  try {
    return useBlogPostRaw();
  } catch {
    // No BlogPostProvider in the React tree (e.g., docs pages).
    return null;
  }
}

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

export default function TOC({ className, ...props }: Props): JSX.Element {
  // Always call the safe hook unconditionally to satisfy Rules of Hooks.
  // Returns null on non-blog pages or if the blog plugin is unavailable.
  const blogPost = useBlogPostSafe();
  const release = blogPost?.frontMatter?.release as string | undefined;

  return (
    <div className={clsx(styles.tableOfContents, 'thin-scrollbar', className)}>
      {release && (
        <div className={styles.releaseContainer}>
          <ReleaseBadge release={release} />
        </div>
      )}
      <TOCItems
        {...props}
        linkClassName={LINK_CLASS_NAME}
        linkActiveClassName={LINK_ACTIVE_CLASS_NAME}
      />
    </div>
  );
}
