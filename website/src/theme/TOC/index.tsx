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
// available, we fall back to a no-op hook that returns null.
let useBlogPost: () => { frontMatter?: Record<string, unknown> } | null;
try {
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const blogClient = require('@docusaurus/plugin-content-blog/client');
  useBlogPost = blogClient.useBlogPost;
} catch {
  // Blog plugin not available - provide a no-op hook.
  useBlogPost = () => null;
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
  // Hook is always called unconditionally - useBlogPost is either the real
  // hook or a no-op that returns null (set at module scope).
  const blogPost = useBlogPost();
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
