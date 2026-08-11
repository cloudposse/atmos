import React from 'react';
import OriginalDocSidebarItem from '@theme-original/DocSidebarItem';
import type DocSidebarItemType from '@theme/DocSidebarItem';
import type { WrapperProps } from '@docusaurus/types';
import { isExperimentalRoute } from '@site/src/data/experimentalRoutes';
import ExperimentalDot from '@site/src/components/ExperimentalDot';

type Props = WrapperProps<typeof DocSidebarItemType>;

/**
 * Wraps the default sidebar item to mark experimental features with a discrete
 * yellow dot trailing the label. Experimental status is derived from the roadmap
 * (the single source of truth), so no per-doc frontmatter is required.
 *
 * The dot is injected into the item label (rather than via CSS) so it can carry a
 * fast, styled React tooltip that reads "experimental" on hover. Injecting via the
 * label works for both `link` items and doc-linked categories without ejecting the
 * upstream components.
 */
export default function DocSidebarItemWrapper(props: Props): JSX.Element {
  const item = props.item as { href?: string; label?: React.ReactNode };
  // `href` is the permalink for `link` items and for categories linked to a doc.
  const href = item?.href;

  if (isExperimentalRoute(href) && item.label != null) {
    const label = (
      <>
        {item.label}
        <ExperimentalDot />
      </>
    );
    return (
      <OriginalDocSidebarItem {...props} item={{ ...props.item, label } as Props['item']} />
    );
  }

  return <OriginalDocSidebarItem {...props} />;
}
