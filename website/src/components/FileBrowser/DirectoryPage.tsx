/**
 * DirectoryPage - Directory listing view with file tree sidebar.
 */
import React from 'react';
import Layout from '@theme/Layout';
import BreadcrumbNav from './BreadcrumbNav';
import FileTree from './FileTree';
import DirectoryListing from './DirectoryListing';
import FileViewer from './FileViewer';
import RelatedDocs from './RelatedDocs';
import { findExampleByName, getExampleNameFromPath } from './utils';
import type { ExamplesTree, FileBrowserOptions, DirectoryNode } from './types';
import styles from './styles.module.css';

interface DirectoryPageProps {
  treeData: ExamplesTree;
  optionsData: FileBrowserOptions;
  dirData: DirectoryNode;
}

export default function DirectoryPage({
  treeData,
  optionsData,
  dirData,
}: DirectoryPageProps): JSX.Element {
  const { routeBasePath } = optionsData;
  const exampleName = getExampleNameFromPath(dirData.path);
  const example = findExampleByName(treeData.examples, exampleName);

  if (!example) {
    return (
      <Layout title="Not Found">
        <div className={styles.emptyState}>
          <h1 className={styles.emptyStateTitle}>Example not found</h1>
          <p className={styles.emptyStateDescription}>
            The requested example could not be found.
          </p>
        </div>
      </Layout>
    );
  }

  const pageTitle = dirData.path === exampleName
    ? `${exampleName} - Examples`
    : `${dirData.name} - ${exampleName}`;

  return (
    <Layout title={pageTitle}>
      <div className={styles.pageLayout}>
        <FileTree
          example={example}
          routeBasePath={routeBasePath}
          currentPath={dirData.path}
        />
        <main className={styles.mainContent}>
          <BreadcrumbNav path={dirData.path} routeBasePath={routeBasePath} />

          {/* Show README if present */}
          {dirData.readme && (
            <div className={styles.readmeSection}>
              <FileViewer file={dirData.readme} />
            </div>
          )}

          <DirectoryListing directory={dirData} routeBasePath={routeBasePath} />

          {/* Show related documentation */}
          {example.docs && example.docs.length > 0 && (
            <RelatedDocs docs={example.docs} />
          )}
        </main>
      </div>
    </Layout>
  );
}
