import React from "react";
import OriginalDocSidebarItems from "@theme-original/DocSidebarItems";
import type { Props } from "@theme/DocSidebarItems";
import SidebarNavigator from "@site/src/components/SidebarNavigator";

/** Both the desktop and mobile menus enter here at level 1. */
export default function DocSidebarItems(props: Props): JSX.Element {
  if (props.level !== 1) return <OriginalDocSidebarItems {...props} />;
  const sidebarName = String(
    props.items.find((item) => item.type === "category")?.customProps
      ?.navigationSidebar || "",
  );
  return (
    <SidebarNavigator key={sidebarName} {...props} sidebarName={sidebarName} />
  );
}
