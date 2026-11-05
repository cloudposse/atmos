import React from "react";
import FeatureStatusDot from "./index";
import { experimentalRoutes } from "@site/src/data/experimentalRoutes";
import { getFeatureStatus } from "@site/src/data/featureStatus.mjs";

/** Use the same lifecycle metadata in sidebar links, headings, and search results. */
export default function RouteStatusDot({
  href,
}: {
  href?: string;
}): JSX.Element | null {
  const status = getFeatureStatus(href, experimentalRoutes);
  return status ? <FeatureStatusDot status={status} /> : null;
}
