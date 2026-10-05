import React from "react";
import OriginalDocSidebarItem from "@theme-original/DocSidebarItem";
import type DocSidebarItemType from "@theme/DocSidebarItem";
import type { WrapperProps } from "@docusaurus/types";
import { experimentalRoutes } from "@site/src/data/experimentalRoutes";
import { getFeatureStatus } from "@site/src/data/featureStatus.mjs";
import RouteStatusDot from "@site/src/components/FeatureStatusDot/RouteStatusDot";

import { sidebarActivePath } from "@site/src/components/SidebarNavigator/canonical.mjs";

type Props = WrapperProps<typeof DocSidebarItemType>;

/** Decorate both doc links and linked categories without changing their text labels. */
export default function DocSidebarItemWrapper(props: Props): JSX.Element {
  const activePath = sidebarActivePath(props.item, props.activePath);
  const item = props.item as { href?: string; label?: React.ReactNode };
  // `href` is the permalink for `link` items and for categories linked to a doc.
  const href = item?.href;

  if (getFeatureStatus(href, experimentalRoutes) && item.label != null) {
    const label = (
      <>
        {item.label}
        <RouteStatusDot href={href} />
      </>
    );
    return (
      <OriginalDocSidebarItem
        {...props}
        activePath={activePath}
        item={{ ...props.item, label } as Props["item"]}
      />
    );
  }

  return <OriginalDocSidebarItem {...props} activePath={activePath} />;
}
