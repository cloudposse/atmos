import React from 'react';
import { useBlogPost } from '@docusaurus/plugin-content-blog/client';
import OriginalFooter from '@theme-original/BlogPostItem/Footer';
import AtmosNewsletter from '@site/src/components/AtmosNewsletter';

/** Show signup below each full changelog post, without repeating it in previews. */
export default function Footer(props: React.ComponentProps<typeof OriginalFooter>): JSX.Element {
  const { isBlogPostPage } = useBlogPost();
  return (
    <>
      <OriginalFooter {...props} />
      {isBlogPostPage && <AtmosNewsletter />}
    </>
  );
}
