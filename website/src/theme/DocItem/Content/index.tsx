import React from 'react';
import Head from '@docusaurus/Head';
import OriginalDocItemContent from '@theme-original/DocItem/Content';
import type DocItemContentType from '@theme/DocItem/Content';
import type { WrapperProps } from '@docusaurus/types';
import { useDoc } from '@docusaurus/plugin-content-docs/client';

type Props = WrapperProps<typeof DocItemContentType>;

/**
 * Wraps the default DocItem/Content to inject a Markdown alternate link
 * (<link rel="alternate" type="text/markdown">) into <head>. The companion
 * "Copy Markdown" UI lives in the DocBreadcrumbs swizzle so it shares the
 * breadcrumb row instead of consuming a full row above the page body.
 *
 * The raw .md file is emitted at build time by the local
 * `docusaurus-plugin-llms-txt` plugin (option: generatePerPageMarkdown).
 */
export default function ContentWrapper(props: Props): JSX.Element {
  const { metadata } = useDoc();
  const permalink = metadata?.permalink ?? '';
  const mdHref = permalink ? permalink.replace(/\/$/, '') + '.md' : '';

  return (
    <>
      {mdHref && (
        <Head>
          <link rel="alternate" type="text/markdown" href={mdHref} />
        </Head>
      )}
      <OriginalDocItemContent {...props} />
    </>
  );
}
