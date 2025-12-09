/**
 * Swizzled BlogPostItem/Header - passes through to original.
 * Release badge is now displayed in BlogLayout TOC area instead.
 */
import React from 'react';
import OriginalHeader from '@theme-original/BlogPostItem/Header';

export default function Header(props: React.ComponentProps<typeof OriginalHeader>): JSX.Element {
  return <OriginalHeader {...props} />;
}
