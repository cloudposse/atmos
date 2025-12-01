/**
 * Custom BlogSidebar/Desktop that adds a collapsible sidebar.
 * Collapsed by default to give more screen real estate to content.
 */
import React from 'react';
import clsx from 'clsx';
import { translate } from '@docusaurus/Translate';
import {
  useVisibleBlogSidebarItems,
  BlogSidebarItemList,
} from '@docusaurus/plugin-content-blog/client';
import BlogSidebarContent from '@theme/BlogSidebar/Content';
import type { Props as BlogSidebarContentProps } from '@theme/BlogSidebar/Content';
import type { Props } from '@theme/BlogSidebar/Desktop';
import { useSidebarCollapsed } from '../context';

import styles from './styles.module.css';

const ListComponent: BlogSidebarContentProps['ListComponent'] = ({ items }) => {
  return (
    <BlogSidebarItemList
      items={items}
      ulClassName={clsx(styles.sidebarItemList, 'clean-list')}
      liClassName={styles.sidebarItem}
      linkClassName={styles.sidebarItemLink}
      linkActiveClassName={styles.sidebarItemLinkActive}
    />
  );
};

function BlogSidebarDesktop({ sidebar }: Props) {
  const items = useVisibleBlogSidebarItems(sidebar.items);
  const { isCollapsed, setIsCollapsed } = useSidebarCollapsed();

  return (
    <aside
      className={clsx(
        styles.sidebarContainer,
        isCollapsed ? styles.sidebarCollapsed : 'col col--3'
      )}
    >
      <button
        className={styles.toggleButton}
        onClick={() => setIsCollapsed(!isCollapsed)}
        aria-label={isCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
        title={isCollapsed ? 'Show recent changes' : 'Hide sidebar'}
      >
        <svg
          viewBox="0 0 24 24"
          width="20"
          height="20"
          className={clsx(styles.toggleIcon, isCollapsed && styles.toggleIconCollapsed)}
        >
          <path
            fill="currentColor"
            d="M15.41 7.41L14 6l-6 6 6 6 1.41-1.41L10.83 12z"
          />
        </svg>
      </button>
      <nav
        className={clsx(styles.sidebar, 'thin-scrollbar')}
        aria-label={translate({
          id: 'theme.blog.sidebar.navAriaLabel',
          message: 'Blog recent posts navigation',
          description: 'The ARIA label for recent posts in the blog sidebar',
        })}
      >
        <div className={clsx(styles.sidebarItemTitle, 'margin-bottom--md')}>
          {sidebar.title}
        </div>
        <BlogSidebarContent
          items={items}
          ListComponent={ListComponent}
          yearGroupHeadingClassName={styles.yearGroupHeading}
        />
      </nav>
    </aside>
  );
}

export default BlogSidebarDesktop;
