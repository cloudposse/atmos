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

// Safe hook to get blog post metadata.
// biome-ignore lint/correctness/useHookAtTopLevel: safe conditional import for optional blog plugin
function useBlogPostSafe() {
  try {
    const { useBlogPost } = require('@docusaurus/plugin-content-blog/client');
    return useBlogPost();
  } catch {
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
