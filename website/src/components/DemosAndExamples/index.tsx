/**
 * DemosAndExamples - Combined page with tabs for Demos and Examples.
 * Reduces top navigation by combining two pages into one with tab switching.
 */
import React, { useState, useEffect } from 'react';
import useGlobalData from '@docusaurus/useGlobalData';
import DemoGallery from '../DemoGallery';
import { IndexPageContent } from '../FileBrowser/IndexPage';
import type { ExamplesTree, FileBrowserOptions } from '../FileBrowser/types';
import styles from './styles.module.css';

type TabType = 'demos' | 'examples';

interface GlobalDataFileBrowser {
  examples: ExamplesTree['examples'];
  options: FileBrowserOptions;
}

export default function DemosAndExamples(): JSX.Element {
  const [activeTab, setActiveTab] = useState<TabType>('demos');

  // Get FileBrowser global data for the Examples tab.
  const globalData = useGlobalData();
  const fileBrowserData = globalData['file-browser']?.['examples'] as GlobalDataFileBrowser | undefined;

  // Read URL hash on mount to handle deep linking.
  useEffect(() => {
    const hash = window.location.hash;
    if (hash === '#examples') {
      setActiveTab('examples');
    } else if (hash === '#demos' || !hash) {
      setActiveTab('demos');
    }
    // If hash is something else (like a video ID), stay on demos tab.
  }, []);

  // Update URL hash when tab changes (but preserve video hashes).
  const handleTabChange = (tab: TabType) => {
    setActiveTab(tab);
    if (tab === 'examples') {
      window.history.replaceState({}, '', '/demos#examples');
    } else {
      // For demos tab, just go to /demos (video deep links will add their own hash).
      window.history.replaceState({}, '', '/demos');
    }
  };

  // Build tree data structure for IndexPage.
  const treeData: ExamplesTree | null = fileBrowserData ? {
    examples: fileBrowserData.examples,
    tags: [...new Set(fileBrowserData.examples.flatMap(ex => ex.tags))],
    generatedAt: new Date().toISOString(),
    totalFiles: fileBrowserData.examples.reduce((sum, ex) => sum + ex.root.fileCount, 0),
    totalExamples: fileBrowserData.examples.length,
  } : null;

  return (
    <div className={styles.container}>
      {/* Header */}
      <div className={styles.header}>
        <h1 className={styles.title}>Demos & Examples</h1>
        <p className={styles.description}>
          Watch Atmos in action with terminal demos, or explore complete example projects.
        </p>
      </div>

      {/* Tab Bar */}
      <div className={styles.tabBar}>
        <button
          type="button"
          className={`${styles.tab} ${activeTab === 'demos' ? styles.tabActive : ''}`}
          onClick={() => handleTabChange('demos')}
        >
          Demos
        </button>
        <button
          type="button"
          className={`${styles.tab} ${activeTab === 'examples' ? styles.tabActive : ''}`}
          onClick={() => handleTabChange('examples')}
        >
          Examples
        </button>
      </div>

      {/* Tab Content */}
      <div className={styles.tabContent}>
        {activeTab === 'demos' && (
          <DemoGallery />
        )}
        {activeTab === 'examples' && treeData && fileBrowserData && (
          <IndexPageContent
            treeData={treeData}
            optionsData={fileBrowserData.options}
            hideHeader
          />
        )}
        {activeTab === 'examples' && !fileBrowserData && (
          <div className={styles.emptyState}>
            <p>No examples available.</p>
          </div>
        )}
      </div>
    </div>
  );
}
