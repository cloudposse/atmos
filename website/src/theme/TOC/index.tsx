/**
 * Swizzled TOC component to display release version badges at the top.
 *
 * This component is used on both doc pages and blog pages:
 * - Blog pages: Displays release version badge from frontmatter
 * - Doc pages: Displays "Unreleased" badge if the doc has changes not in a release
 *
 * The challenge is that useBlogPost() throws when called outside of
 * BlogPostProvider (which only wraps individual blog post pages).
 *
 * We solve this by using BrowserOnly to defer the blog-specific rendering
 * to the client side only. During SSG, we render just the basic TOC.
 * On the client, we dynamically import and render the appropriate badge component.
 */
import React from 'react';
import clsx from 'clsx';
import { useLocation } from '@docusaurus/router';
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
 * Paths that are NOT documentation pages.
 * Used to determine when to show doc release badges.
 */
const NON_DOC_PATHS = [
  BLOG_ROUTE_BASE, // Blog/changelog pages
  '/search', // Search page
];

/**
 * Check if the current path is a blog post page.
 * Blog posts are paths under BLOG_ROUTE_BASE that are not listing/archive pages.
 */
function isBlogPostPath(pathname: string): boolean {
  // Must start with blog base path.
  if (!pathname.startsWith(BLOG_ROUTE_BASE)) {
    return false;
  }

  // Exclude the blog index page itself.
  if (pathname === BLOG_ROUTE_BASE || pathname === `${BLOG_ROUTE_BASE}/`) {
    return false;
  }

  // Exclude non-post paths (pagination, tags, archive).
  const pathAfterBase = pathname.slice(BLOG_ROUTE_BASE.length);
  for (const nonPostPath of BLOG_NON_POST_PATHS) {
    if (pathAfterBase.startsWith(nonPostPath)) {
      return false;
    }
  }

  return true;
}

/**
 * Check if the current path is a documentation page.
 * Doc pages are any page that isn't the home page, blog, or other special pages.
 */
function isDocPath(pathname: string): boolean {
  // Home page is not a doc page.
  if (pathname === '/' || pathname === '') {
    return false;
  }

  // Check if path starts with any non-doc path.
  for (const nonDocPath of NON_DOC_PATHS) {
    if (pathname.startsWith(nonDocPath)) {
      return false;
    }
  }

  return true;
}

/**
 * Error boundary to catch errors from badge components.
 * Returns null on error to gracefully degrade without breaking the TOC.
 */
class BadgeErrorBoundary extends React.Component<
  { children: React.ReactNode },
  { hasError: boolean }
> {
  constructor(props: { children: React.ReactNode }) {
    super(props);
    this.state = { hasError: false };
  }

  static getDerivedStateFromError(): { hasError: boolean } {
    return { hasError: true };
  }

  render(): React.ReactNode {
    if (this.state.hasError) {
      return null;
    }
    return this.props.children;
  }
}

/**
 * Lazy-loaded badge components for code splitting.
 */
const LazyBlogReleaseBadge = React.lazy(() => import('./BlogReleaseBadge'));
const LazyDocReleaseBadge = React.lazy(() => import('./DocReleaseBadge'));

/**
 * Client-side component wrapper that renders the blog release badge.
 * This is rendered inside BrowserOnly to avoid SSG issues.
 */
function ClientSideBlogReleaseBadge({
  pathname,
}: {
  pathname: string;
}): JSX.Element | null {
  if (!isBlogPostPath(pathname)) {
    return null;
  }

  return (
    <BadgeErrorBoundary>
      <React.Suspense fallback={null}>
        <LazyBlogReleaseBadge />
      </React.Suspense>
    </BadgeErrorBoundary>
  );
}

/**
 * Client-side component wrapper that renders the doc release badge.
 * This is rendered inside BrowserOnly to avoid SSG issues.
 */
function ClientSideDocReleaseBadge({
  pathname,
}: {
  pathname: string;
}): JSX.Element | null {
  if (!isDocPath(pathname)) {
    return null;
  }

  return (
    <BadgeErrorBoundary>
      <React.Suspense fallback={null}>
        <LazyDocReleaseBadge />
      </React.Suspense>
    </BadgeErrorBoundary>
  );
}

/**
 * Client-side component that determines which badge to show based on the path.
 */
function ClientSideReleaseBadge({
  pathname,
}: {
  pathname: string;
}): JSX.Element | null {
  // Blog posts get blog badge.
  if (isBlogPostPath(pathname)) {
    return <ClientSideBlogReleaseBadge pathname={pathname} />;
  }

  // Doc pages get doc badge.
  if (isDocPath(pathname)) {
    return <ClientSideDocReleaseBadge pathname={pathname} />;
  }

  return null;
}

export default function TOC({ className, ...props }: Props): JSX.Element {
  // useLocation re-renders the component on client-side navigation.
  const { pathname } = useLocation();

  return (
    <div className={clsx(styles.tableOfContents, 'thin-scrollbar', className)}>
      <BrowserOnly>
        {() => <ClientSideReleaseBadge pathname={pathname} />}
      </BrowserOnly>
      <TOCItems
        {...props}
        linkClassName={LINK_CLASS_NAME}
        linkActiveClassName={LINK_ACTIVE_CLASS_NAME}
      />
    </div>
  );
}
