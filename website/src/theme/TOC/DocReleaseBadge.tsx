/**
 * Component that displays the release badge on documentation pages.
 * Uses global data from the doc-release-data plugin to determine if
 * the current doc has unreleased changes.
 *
 * Only shows a badge for unreleased docs - released docs show no badge.
 */
import React from 'react';
import { useLocation } from '@docusaurus/router';
import { usePluginData } from '@docusaurus/useGlobalData';
import ReleaseBadge from './ReleaseBadge';
import styles from './styles.module.css';

interface DocReleaseData {
  releaseMap: Record<string, string>;
}

export default function DocReleaseBadge(): JSX.Element | null {
  const { pathname } = useLocation();

  // Get the release map from our plugin's global data.
  let releaseData: DocReleaseData | undefined;
  try {
    releaseData = usePluginData('doc-release-data') as DocReleaseData;
  } catch {
    // Plugin data not available (e.g., during development without the plugin).
    return null;
  }

  if (!releaseData?.releaseMap) {
    return null;
  }

  // Look up the release for this doc path.
  // Try both with and without trailing slash.
  const release =
    releaseData.releaseMap[pathname] ||
    releaseData.releaseMap[pathname.replace(/\/$/, '')] ||
    releaseData.releaseMap[`${pathname}/`];

  // Only show badge for unreleased docs.
  // Released docs show no badge to keep the UI clean.
  if (!release || release !== 'unreleased') {
    return null;
  }

  return (
    <div className={styles.releaseContainer}>
      <ReleaseBadge release={release} unreleasedLink="/changelog" />
    </div>
  );
}
