/**
 * Swizzled TOC component to display release version badge at the top.
 *
 * This component is used on both doc pages and blog pages. On blog pages,
 * it displays a release version badge from the blog post's frontmatter.
 * On non-blog pages, it renders the standard TOC without the badge.
 *
 * The challenge is that useBlogPost() throws when called outside of
 * BlogPostProvider (which only wraps individual blog post pages).
 *
 * We solve this by using BrowserOnly to defer the blog-specific rendering
 * to the client side only. During SSG, we render just the basic TOC.
 * On the client, we dynamically import and render the blog badge component.
 */
import React from 'react';
import clsx from 'clsx';
import BrowserOnly from '@docusaurus/BrowserOnly';
import TOCItems from '@theme/TOCItems';
import type { Props } from '@theme/TOC';
import styles from './styles.module.css';

const LINK_CLASS_NAME = 'table-of-contents__link toc-highlight';
const LINK_ACTIVE_CLASS_NAME = 'table-of-contents__link--active';

/**
 * Blog route base path - must match the `routeBasePath` in docusaurus.config.js
 * under the blog plugin configuration.
 *
 * IMPORTANT: If the blog route changes in docusaurus.config.js, update this constant.
 * See: docusaurus.config.js -> presets -> classic -> blog -> routeBasePath
 */
const BLOG_ROUTE_BASE = '/changelog';

/**
 * Paths under the blog route that are NOT individual blog posts.
 * These are listing pages, tag pages, and archive pages.
 */
const BLOG_NON_POST_PATHS = ['/page/', '/tags/', '/archive'];

/**
 * Check if the current path is a blog post page.
 * Blog posts are paths under BLOG_ROUTE_BASE that are not listing/archive pages.
 */
function isBlogPostPath(pathname: string): boolean {
  // Must start with blog base path
  if (!pathname.startsWith(BLOG_ROUTE_BASE)) {
    return false;
  }

  // Exclude the blog index page itself
  if (pathname === BLOG_ROUTE_BASE || pathname === `${BLOG_ROUTE_BASE}/`) {
    return false;
  }

  // Exclude non-post paths (pagination, tags, archive)
  const pathAfterBase = pathname.slice(BLOG_ROUTE_BASE.length);
  for (const nonPostPath of BLOG_NON_POST_PATHS) {
    if (pathAfterBase.startsWith(nonPostPath)) {
      return false;
    }
  }

  return true;
}

/**
 * Client-side component wrapper that renders the blog release badge.
 * This is rendered inside BrowserOnly to avoid SSG issues.
 */
function ClientSideBlogReleaseBadge(): JSX.Element | null {
  // Check if we're on a blog post page
  if (!isBlogPostPath(window.location.pathname)) {
    return null;
  }

  // Dynamically import and render the blog release badge component.
  // We use require here to avoid static analysis pulling in the blog client
  // dependencies during SSG bundling.
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const BlogReleaseBadge = require('./BlogReleaseBadge').default;
    return <BlogReleaseBadge />;
  } catch (error) {
    // If there's an error (e.g., not in BlogPostProvider context), silently fail
    console.warn('Failed to render BlogReleaseBadge:', error);
    return null;
  }
}

export default function TOC({ className, ...props }: Props): JSX.Element {
  return (
    <div className={clsx(styles.tableOfContents, 'thin-scrollbar', className)}>
      <BrowserOnly>
        {() => <ClientSideBlogReleaseBadge />}
      </BrowserOnly>
      <TOCItems
        {...props}
        linkClassName={LINK_CLASS_NAME}
        linkActiveClassName={LINK_ACTIVE_CLASS_NAME}
      />
    </div>
  );
}
