/**
 * IndexPage - Landing page showing all example projects with tag filtering.
 */
import React, { useState } from 'react';
import Layout from '@theme/Layout';
import Link from '@docusaurus/Link';
import Markdown from 'react-markdown';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faFolder } from '@fortawesome/free-solid-svg-icons';
import CastPlayer from '@site/src/components/CastPlayer';
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

interface IndexPageProps {
  treeData: ExamplesTree;
  optionsData: FileBrowserOptions;
}

export default function IndexPage({ treeData, optionsData }: IndexPageProps): JSX.Element {
  const { examples, featured = [], tags } = treeData;
  const { routeBasePath, title, description } = optionsData;
  const [activeTag, setActiveTag] = useState<string | null>(null);

  const filteredExamples = activeTag
    ? examples.filter((ex) => ex.tags.includes(activeTag))
    : examples;

  // Group the "All" view into visible sections by each example's primary
  // (first) tag, in the site's tag order; anything untagged lands in "More".
  const sections = [
    ...tags.map((tag) => ({
      tag,
      examples: examples.filter((ex) => (ex.tags[0] ?? 'More') === tag),
    })),
    { tag: 'More', examples: examples.filter((ex) => ex.tags.length === 0) },
  ].filter((section) => section.examples.length > 0);

  // Render a single example card. Featured cards use the friendly title; the full grid keeps
  // the directory name so it stays scannable alongside the URL path.
  const renderCard = (example: ExamplesTree['examples'][number], displayName: string) => (
    <article
      key={example.name}
      className={styles.exampleCard}
    >
      <Link to={`${routeBasePath}/${example.name}`} className={styles.exampleCardLink}>
        <div className={styles.exampleCardHeader}>
          <div className={styles.exampleCardIcon}>
            <FontAwesomeIcon icon={faFolder} />
          </div>
          <h2 className={styles.exampleCardTitle}>{displayName}</h2>
        </div>
      </Link>
      {example.cast?.file && (
        <div className={styles.exampleCardCast}>
          <CastPlayer
            src={example.cast.file}
            title={example.cast.title || displayName}
            chrome
            thumbnail
            controls={false}
            scrubber={false}
            showCommand={false}
          />
        </div>
      )}
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
        <Link to={`${routeBasePath}/${example.name}`} className={styles.exampleCardCta}>
          Open
        </Link>
      </div>
    </article>
  );

  return (
    <Layout title={title} description={description}>
      <div className={styles.indexPage}>
        <header className={styles.indexHeader}>
          <h1 className={styles.indexTitle}>{title}</h1>
          <p className={styles.indexDescription}>{description}</p>
        </header>

        {activeTag === null && featured.length > 0 && (
          <section className={styles.featuredSection}>
            <h2 className={styles.featuredHeading}>Featured</h2>
            <div className={styles.examplesGrid}>
              {featured.map((example) => renderCard(example, example.title || example.name))}
            </div>
          </section>
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

        {activeTag === null ? (
          sections.map(({ tag, examples: sectionExamples }) => (
            <section key={tag} className={styles.tagSection}>
              <h2 className={styles.tagSectionHeading}>{tag}</h2>
              <div className={styles.examplesGrid}>
                {sectionExamples.map((example) => renderCard(example, example.name))}
              </div>
            </section>
          ))
        ) : (
          <div className={styles.examplesGrid}>
            {filteredExamples.map((example) => renderCard(example, example.name))}
          </div>
        )}
      </div>
    </Layout>
  );
}
