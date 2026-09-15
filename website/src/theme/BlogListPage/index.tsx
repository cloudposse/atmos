/**
 * Custom BlogListPage that replaces the default sidebar layout
 * with a full-width vertical timeline.
 */
import React, { type ReactNode } from 'react';
import clsx from 'clsx';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import {
  PageMetadata,
  HtmlClassNameProvider,
  ThemeClassNames,
} from '@docusaurus/theme-common';
import Layout from '@theme/Layout';
import SearchMetadata from '@theme/SearchMetadata';
import type { Props } from '@theme/BlogListPage';
import ChangelogTimeline from '@site/src/components/ChangelogTimeline';
import ProductUpdatesHeader from '@site/src/components/ProductUpdatesHeader';
import styles from './styles.module.css';

function BlogListPageMetadata({ metadata }: { metadata: Props['metadata'] }): ReactNode {
  const {
    siteConfig: { title: siteTitle },
  } = useDocusaurusContext();
  const { blogDescription, blogTitle, permalink } = metadata;
  const isBlogOnlyMode = permalink === '/';
  const title = isBlogOnlyMode ? siteTitle : blogTitle;

  return (
    <>
      <PageMetadata title={title} description={blogDescription} />
      <SearchMetadata tag="blog_posts_list" />
    </>
  );
}

function BlogListPageContent({ items }: Props): ReactNode {
  return (
    <Layout>
      <ProductUpdatesHeader activeView="changelog" />
      <div className={styles.changelogContainer}>
        <ChangelogTimeline items={items} />
      </div>
    </Layout>
  );
}

export default function BlogListPage(props: Props): ReactNode {
  return (
    <HtmlClassNameProvider
      className={clsx(
        ThemeClassNames.wrapper.blogPages,
        ThemeClassNames.page.blogListPage
      )}
    >
      <BlogListPageMetadata metadata={props.metadata} />
      <BlogListPageContent {...props} />
    </HtmlClassNameProvider>
  );
}
