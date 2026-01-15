/**
 * IndexPage - Landing page showing all example projects.
 */
import React from 'react';
import Layout from '@theme/Layout';
import Link from '@docusaurus/Link';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faFolder, faFile, faCube } from '@fortawesome/free-solid-svg-icons';
import type { ExamplesTree, FileBrowserOptions } from './types';
import styles from './styles.module.css';

interface IndexPageProps {
  treeData: ExamplesTree;
  optionsData: FileBrowserOptions;
}

export default function IndexPage({ treeData, optionsData }: IndexPageProps): JSX.Element {
  const { examples, totalFiles, totalExamples } = treeData;
  const { routeBasePath, title, description } = optionsData;

  return (
    <Layout title={title} description={description}>
      <div className={styles.indexPage}>
        <header className={styles.indexHeader}>
          <h1 className={styles.indexTitle}>{title}</h1>
          <p className={styles.indexDescription}>{description}</p>
          <div className={styles.indexStats}>
            <span className={styles.indexStatItem}>
              <FontAwesomeIcon icon={faCube} />
              {totalExamples} examples
            </span>
            <span className={styles.indexStatItem}>
              <FontAwesomeIcon icon={faFile} />
              {totalFiles} files
            </span>
          </div>
        </header>

        <div className={styles.examplesGrid}>
          {examples.map((example) => (
            <Link
              key={example.name}
              to={`${routeBasePath}/${example.name}`}
              className={styles.exampleCard}
            >
              <div className={styles.exampleCardHeader}>
                <div className={styles.exampleCardIcon}>
                  <FontAwesomeIcon icon={faFolder} />
                </div>
                <h2 className={styles.exampleCardTitle}>{example.name}</h2>
              </div>
              <p className={styles.exampleCardDescription}>
                {example.description || 'Explore this example project'}
              </p>
              <div className={styles.exampleCardFooter}>
                <span>{example.root.fileCount} files</span>
                <div className={styles.exampleCardBadges}>
                  {example.hasAtmosYaml && (
                    <span className={styles.exampleCardBadge}>atmos.yaml</span>
                  )}
                  {example.hasReadme && (
                    <span className={styles.exampleCardBadge}>README</span>
                  )}
                </div>
              </div>
            </Link>
          ))}
        </div>
      </div>
    </Layout>
  );
}
