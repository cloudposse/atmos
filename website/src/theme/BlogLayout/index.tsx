/**
 * Custom BlogLayout that uses flexbox instead of grid for better
 * collapsible sidebar support.
 */
import React, { type ReactNode } from 'react';
import clsx from 'clsx';
import Layout from '@theme/Layout';
import BlogSidebar from '@theme/BlogSidebar';

import type { Props } from '@theme/BlogLayout';
import styles from './styles.module.css';

export default function BlogLayout(props: Props): ReactNode {
  const { sidebar, toc, children, ...layoutProps } = props;
  const hasSidebar = sidebar && sidebar.items.length > 0;

  return (
    <Layout {...layoutProps}>
      <div className="container margin-vert--lg">
        <div className={styles.blogLayoutRow}>
          {hasSidebar && <BlogSidebar sidebar={sidebar} />}
          <main className={styles.blogMain}>
            {children}
          </main>
          {toc && <div className={styles.blogToc}>{toc}</div>}
        </div>
      </div>
    </Layout>
  );
}
