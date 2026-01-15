/**
 * FilePage - File content view with syntax highlighting.
 */
import React from 'react';
import Layout from '@theme/Layout';
import BreadcrumbNav from './BreadcrumbNav';
import FileTree from './FileTree';
import FileViewer from './FileViewer';
import { findExampleByName, getExampleNameFromPath } from './utils';
import type { ExamplesTree, FileBrowserOptions, FileNode } from './types';
import styles from './styles.module.css';

interface FilePageProps {
  treeData: ExamplesTree;
  optionsData: FileBrowserOptions;
  fileData: FileNode;
}

export default function FilePage({
  treeData,
  optionsData,
  fileData,
}: FilePageProps): JSX.Element {
  const { routeBasePath } = optionsData;
  const exampleName = getExampleNameFromPath(fileData.path);
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

  const pageTitle = `${fileData.name} - ${exampleName}`;

  return (
    <Layout title={pageTitle}>
      <div className={styles.pageLayout}>
        <FileTree
          example={example}
          routeBasePath={routeBasePath}
          currentPath={fileData.path}
        />
        <main className={styles.mainContent}>
          <BreadcrumbNav path={fileData.path} routeBasePath={routeBasePath} />
          <FileViewer file={fileData} />
        </main>
      </div>
    </Layout>
  );
}
