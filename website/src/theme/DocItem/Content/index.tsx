import React from 'react';
import Head from '@docusaurus/Head';
import OriginalDocItemContent from '@theme-original/DocItem/Content';
import type DocItemContentType from '@theme/DocItem/Content';
import type { WrapperProps } from '@docusaurus/types';
import { useDoc } from '@docusaurus/plugin-content-docs/client';

import CopyMarkdownButton from '@site/src/components/CopyMarkdownButton';

type Props = WrapperProps<typeof DocItemContentType>;

/**
 * Wraps the default DocItem/Content to:
 *  - announce the raw Markdown alternate via <link rel="alternate" type="text/markdown">
 *  - render a Copy/View Markdown control above the page body
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
      {mdHref && <CopyMarkdownButton href={mdHref} />}
      <OriginalDocItemContent {...props} />
    </>
  );
}
