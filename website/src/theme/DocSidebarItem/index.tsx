import React from "react";
import OriginalDocSidebarItem from "@theme-original/DocSidebarItem";
import type DocSidebarItemType from "@theme/DocSidebarItem";
import type { WrapperProps } from "@docusaurus/types";
import { experimentalRoutes } from "@site/src/data/experimentalRoutes";
import { getFeatureStatus } from "@site/src/data/featureStatus.mjs";
import RouteStatusDot from "@site/src/components/FeatureStatusDot/RouteStatusDot";

import { useNavigationContext } from "@site/src/components/SidebarNavigator/context";
import {
  activeEntryPath,
  entryId,
} from "@site/src/components/SidebarNavigator/state.mjs";
import Category from "@site/src/components/SidebarNavigator/Category";
import { sidebarActivePath } from "@site/src/components/SidebarNavigator/canonical.mjs";

type Props = WrapperProps<typeof DocSidebarItemType>;

/** Decorate both doc links and linked categories without changing their text labels. */
export default function DocSidebarItemWrapper(props: Props): JSX.Element {
  const navigation = useNavigationContext();
  const activePath = navigation
    ? activeEntryPath(props.item, props.activePath, navigation.entry)
    : sidebarActivePath(props.item, props.activePath);
  const item = props.item as { href?: string; label?: React.ReactNode };
  // `href` is the permalink for `link` items and for categories linked to a doc.
  const href = item?.href;

  const decorated =
    getFeatureStatus(href, experimentalRoutes) && item.label != null
      ? ({
          ...props.item,
          label: (
            <>
              {item.label}
              <RouteStatusDot href={href} />
            </>
          ),
        } as Props["item"])
      : props.item;
  if (navigation && decorated.type === "category") {
    return <Category {...props} item={decorated} activePath={activePath} />;
  }
  return (
    <OriginalDocSidebarItem
      {...props}
      item={decorated}
      activePath={activePath}
      data-navigation-id={entryId(props.item)}
    />
  );
}
