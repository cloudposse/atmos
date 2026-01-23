/**
 * IndexPage - Landing page showing all example projects with tag filtering.
 */
import React, { useState } from 'react';
import Layout from '@theme/Layout';
import Link from '@docusaurus/Link';
import Markdown from 'react-markdown';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faFolder } from '@fortawesome/free-solid-svg-icons';
import type { ExamplesTree, FileBrowserOptions } from './types';
import styles from './styles.module.css';

/**
 * Markdown components for card descriptions.
 * Links are rendered as plain text to avoid nested <a> tags.
 */
const cardMarkdownComponents = {
  // Render links as plain text since card is already a link.
  a: ({ children }: { children?: React.ReactNode }) => <>{children}</>,
  // Remove paragraph wrappers for inline rendering.
  p: ({ children }: { children?: React.ReactNode }) => <>{children}</>,
};

interface IndexPageContentProps {
  treeData: ExamplesTree;
  optionsData: FileBrowserOptions;
  /** Hide the header (title/description) when embedded in another component. */
  hideHeader?: boolean;
}

/**
 * IndexPageContent - The actual content without Layout wrapper.
 * Use this when embedding in another component that already has Layout.
 */
export function IndexPageContent({ treeData, optionsData, hideHeader = false }: IndexPageContentProps): JSX.Element {
  const { examples, tags } = treeData;
  const { routeBasePath } = optionsData;
  const [activeTag, setActiveTag] = useState<string | null>(null);

  const filteredExamples = activeTag
    ? examples.filter((ex) => ex.tags.includes(activeTag))
    : examples;

  return (
    <div className={styles.indexPage}>
      {!hideHeader && (
        <header className={styles.indexHeader}>
          <h1 className={styles.indexTitle}>{optionsData.title}</h1>
          <p className={styles.indexDescription}>{optionsData.description}</p>
        </header>
      )}

      <div className={styles.filterBar}>
        <button
          type="button"
          className={`${styles.filterButton} ${activeTag === null ? styles.filterButtonActive : ''}`}
          onClick={() => setActiveTag(null)}
        >
          All
        </button>
        {tags.map((tag) => (
          <button
            key={tag}
            type="button"
            className={`${styles.filterButton} ${activeTag === tag ? styles.filterButtonActive : ''}`}
            onClick={() => setActiveTag(tag)}
          >
            {tag}
          </button>
        ))}
      </div>

      <div className={styles.examplesGrid}>
        {filteredExamples.map((example) => (
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
            <div className={styles.exampleCardDescription}>
              <Markdown components={cardMarkdownComponents}>
                {example.description || 'Explore this example project'}
              </Markdown>
            </div>
            <div className={styles.exampleCardFooter}>
              <div className={styles.tagList}>
                {example.tags.map((tag) => (
                  <span key={tag} className={styles.tagBadge}>{tag}</span>
                ))}
              </div>
            </div>
          </Link>
        ))}
      </div>
    </div>
  );
}

interface IndexPageProps {
  treeData: ExamplesTree;
  optionsData: FileBrowserOptions;
}

/**
 * IndexPage - Standalone page with Layout wrapper.
 * Use IndexPageContent for embedding without Layout.
 */
export default function IndexPage({ treeData, optionsData }: IndexPageProps): JSX.Element {
  const { title, description } = optionsData;

  return (
    <Layout title={title} description={description}>
      <IndexPageContent treeData={treeData} optionsData={optionsData} />
    </Layout>
  );
}
