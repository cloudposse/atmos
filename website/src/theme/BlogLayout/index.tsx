/**
 * Swizzled BlogLayout that adjusts column widths based on sidebar collapsed state.
 */
import React, { type ReactNode } from 'react';
import clsx from 'clsx';
import Layout from '@theme/Layout';
import BlogSidebar from '@theme/BlogSidebar';
import type { Props } from '@theme/BlogLayout';
import { SidebarProvider, useSidebarCollapsed } from '../BlogSidebar/context';

import styles from './styles.module.css';

function BlogLayoutContent({
  sidebar,
  toc,
  children,
  ...layoutProps
}: Props): ReactNode {
  const hasSidebar = sidebar && sidebar.items.length > 0;
  const { isCollapsed } = useSidebarCollapsed();

  // When sidebar is collapsed, give main content more space
  // Expanded: sidebar (3) + main (7) + toc (2) = 12
  // Collapsed: sidebar takes fixed 48px, main uses flex-grow to fill remaining space
  const mainColClass = clsx('col', styles.mainContent, {
    'col--7': hasSidebar && !isCollapsed,
    [styles.mainExpanded]: hasSidebar && isCollapsed,
    'col--9 col--offset-1': !hasSidebar,
  });

  return (
    <Layout {...layoutProps}>
      <div className="container margin-vert--lg">
        <div className="row">
          <BlogSidebar sidebar={sidebar} />
          <main className={mainColClass}>
            {children}
          </main>
          {toc && <div className={clsx('col col--2', styles.tocWrapper)}>{toc}</div>}
        </div>
      </div>
    </Layout>
  );
}

export default function BlogLayout(props: Props): ReactNode {
  return (
    <SidebarProvider>
      <BlogLayoutContent {...props} />
    </SidebarProvider>
  );
}
