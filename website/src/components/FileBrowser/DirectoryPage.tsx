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
import GistDisclaimer from '@site/src/components/GistDisclaimer';
import CastPlayer from '@site/src/components/CastPlayer';
import CastProDownload from '@site/src/components/CastProDownload';
import CastShareLink from '@site/src/components/CastShareLink';
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

  const sectionName = optionsData.title || 'Examples';
  const pageTitle = dirData.path === exampleName
    ? `${exampleName} - ${sectionName}`
    : `${dirData.name} - ${exampleName}`;
  const isExampleRoot = dirData.path === exampleName;
  const showCast = isExampleRoot && !!example.cast?.file;

  // The cast source lives under website/static/**, which is committed to the
  // repo at that same literal path (unlike other example files, which live
  // under optionsData.githubPath) — so the Atmos Pro rendering service's
  // repo path is derived here rather than reusing githubPath.
  const [proOwner, proRepo] = (optionsData.githubRepo || '').split('/');
  const proSource = showCast
    ? {
        owner: proOwner,
        repo: proRepo,
        gitRef: optionsData.githubBranch || 'main',
        path: `website/static${example.cast!.file}`,
      }
    : null;

  return (
    <Layout title={pageTitle}>
      <div className={styles.pageLayout}>
        <FileTree
          example={example}
          routeBasePath={routeBasePath}
          currentPath={dirData.path}
        />
        <main className={styles.mainContent}>
          <BreadcrumbNav path={dirData.path} routeBasePath={routeBasePath} rootLabel={sectionName.toLowerCase()} />

          {optionsData.disclaimer && (
            <GistDisclaimer text={optionsData.disclaimer} />
          )}

          {showCast && (
            <div className={styles.castSection}>
              <CastPlayer
                src={example.cast!.file!}
                title={example.cast!.title || example.name}
                chrome
                controls
                scrubber
              />
              {proSource && (
                <div className={styles.castActions}>
                  <CastShareLink
                    owner={proSource.owner}
                    repo={proSource.repo}
                    gitRef={proSource.gitRef}
                    path={proSource.path}
                  />
                  <CastProDownload
                    owner={proSource.owner}
                    repo={proSource.repo}
                    gitRef={proSource.gitRef}
                    path={proSource.path}
                  />
                </div>
              )}
            </div>
          )}

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
