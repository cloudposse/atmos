import React from 'react';
import OriginalDocBreadcrumbs from '@theme-original/DocBreadcrumbs';
import { useDoc } from '@docusaurus/plugin-content-docs/client';

import CopyMarkdownButton from '@site/src/components/CopyMarkdownButton';

import styles from './styles.module.css';

/**
 * Wraps the default DocBreadcrumbs so the "Copy Markdown" split button
 * shares the breadcrumb row, floated to the right. Falls back to the
 * unmodified breadcrumbs when used outside a DocProvider (e.g. category
 * landing pages without a permalink).
 */
export default function DocBreadcrumbsWrapper(props: Record<string, unknown>): JSX.Element {
  let mdHref = '';
  try {
    const { metadata } = useDoc();
    const permalink = metadata?.permalink ?? '';
    mdHref = permalink ? permalink.replace(/\/$/, '') + '.md' : '';
  } catch {
    mdHref = '';
  }

  return (
    <div className={styles.row}>
      <div className={styles.breadcrumbsCol}>
        <OriginalDocBreadcrumbs {...props} />
      </div>
      {mdHref && (
        <div className={styles.actionsCol}>
          <CopyMarkdownButton href={mdHref} />
        </div>
      )}
    </div>
  );
}
