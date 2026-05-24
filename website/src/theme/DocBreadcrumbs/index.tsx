import React from 'react';
import OriginalDocBreadcrumbs from '@theme-original/DocBreadcrumbs';
import { useDoc } from '@docusaurus/plugin-content-docs/client';

import CopyMarkdownButton from '@site/src/components/CopyMarkdownButton';

import styles from './styles.module.css';

/**
 * Derive `<permalink>.md` from the current doc's metadata, normalizing the
 * root and any trailing slash:
 *   /                      → /index.md
 *   /cli/                  → /cli.md
 *   /cli/commands/version  → /cli/commands/version.md
 */
function deriveMarkdownHref(permalink: string): string {
  if (!permalink) return '';
  // Linear trailing-slash trim (CodeQL-safe; avoids /\/+$/).
  let end = permalink.length;
  while (end > 0 && permalink.charCodeAt(end - 1) === 47) end -= 1; // '/'
  const normalized = permalink.slice(0, end);
  return normalized ? normalized + '.md' : '/index.md';
}

/**
 * Wraps the default DocBreadcrumbs so the "Copy Markdown" split button shares
 * the breadcrumb row, floated right.
 *
 * Why the try/catch around useDoc(): Docusaurus renders DocBreadcrumbs in two
 * contexts — real doc pages (DocProvider in scope) and auto-generated category
 * index pages (NO DocProvider). `useDoc()` throws a ReactContextError in the
 * latter. React Error Boundaries don't catch errors during static SSG, so this
 * is the only viable shape. The Rules of Hooks concern (hook order changing
 * between renders) doesn't apply here: for a given render path the
 * try/catch outcome is deterministic — the presence of DocProvider is
 * fixed by Docusaurus, not by component-internal state. No
 * eslint-plugin-react-hooks is configured in this repo.
 */
export default function DocBreadcrumbsWrapper(props: Record<string, unknown>): JSX.Element {
  let mdHref = '';
  try {
    // eslint-disable-next-line react-hooks/rules-of-hooks
    const { metadata } = useDoc();
    mdHref = deriveMarkdownHref(metadata?.permalink ?? '');
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
