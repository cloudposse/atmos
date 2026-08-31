/**
 * IndexPage - Landing page showing all example projects with tag filtering.
 */
import React, { useState } from 'react';
import Layout from '@theme/Layout';
import Link from '@docusaurus/Link';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faFolder, faGraduationCap } from '@fortawesome/free-solid-svg-icons';
import CastPlayer from '@site/src/components/CastPlayer';
import CopyMarkdownButton from './CopyMarkdownButton';
import type { ExamplesTree, FileBrowserOptions } from './types';
import styles from './styles.module.css';

/**
 * Card icons selectable via the `cardIcon` plugin option (see FileBrowserOptions).
 * 'folder' (examples/gists) is the default; add new entries here as new
 * file-browser instances need a different visual identity.
 */
const ICON_MAP = {
  folder: faFolder,
  'graduation-cap': faGraduationCap,
};

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
  const { routeBasePath, title, description, searchable, cardIcon, cardCtaLabel, titleAsCode, enableCopyMarkdown } = optionsData;
  const cardIconDefinition = ICON_MAP[cardIcon] || faFolder;
  const cardCta = cardCtaLabel || 'Open';
  const [activeTag, setActiveTag] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState('');

  const tagFilteredExamples = activeTag
    ? examples.filter((ex) => ex.tags.includes(activeTag))
    : examples;

  const query = searchQuery.trim().toLowerCase();
  const filteredExamples = query
    ? tagFilteredExamples.filter((ex) => {
      const haystack = [ex.name, ex.title, ex.description, ...ex.tags].join(' ').toLowerCase();
      return haystack.includes(query);
    })
    : tagFilteredExamples;

  // Group the "All" view into visible sections by each example's primary
  // (first) tag, in the site's tag order; anything untagged lands in "More".
  // When searching, sections are built from the search results so empty
  // sections drop out instead of showing a heading with nothing under it.
  const sections = [
    ...tags.map((tag) => ({
      tag,
      examples: filteredExamples.filter((ex) => (ex.tags[0] ?? 'More') === tag),
    })),
    { tag: 'More', examples: filteredExamples.filter((ex) => ex.tags.length === 0) },
  ].filter((section) => section.examples.length > 0);

  // Render a single example card. All cards use the friendly English title
  // (README front matter `title:`), falling back to the directory name.
  const renderCard = (example: ExamplesTree['examples'][number], displayName: string) => (
    <article
      key={example.name}
      className={styles.exampleCard}
    >
      {enableCopyMarkdown && (
        <CopyMarkdownButton
          directory={example.root}
          title={example.title || example.name}
          description={example.description}
          iconOnly
          className={styles.exampleCardCopyButton}
        />
      )}
      <Link to={`${routeBasePath}/${example.name}`} className={styles.exampleCardLink}>
        <div className={styles.exampleCardHeader}>
          <div className={styles.exampleCardIcon}>
            <FontAwesomeIcon icon={cardIconDefinition} />
          </div>
          <h2 className={styles.exampleCardTitle}>
            {titleAsCode ? <code>/{example.name}</code> : displayName}
          </h2>
        </div>
      </Link>
      {example.cast?.file && (
        <Link
          to={`${routeBasePath}/${example.name}`}
          className={styles.exampleCardCastLink}
          aria-label={`Open the ${displayName} example`}
        >
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
        </Link>
      )}
      <div className={styles.exampleCardDescription}>
        <Markdown components={cardMarkdownComponents} remarkPlugins={[remarkGfm]}>
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
          {cardCta}
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

        {searchable && (
          <div className={styles.searchContainer}>
            <input
              type="text"
              className={styles.searchInput}
              placeholder={`Search ${title.toLowerCase()} by name, description, or category...`}
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
            />
            <div className={styles.searchResults}>
              Showing {filteredExamples.length} of {examples.length}
            </div>
          </div>
        )}

        {activeTag === null && !query && featured.length > 0 && (
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
                {sectionExamples.map((example) => renderCard(example, example.title || example.name))}
              </div>
            </section>
          ))
        ) : (
          <div className={styles.examplesGrid}>
            {filteredExamples.map((example) => renderCard(example, example.title || example.name))}
          </div>
        )}
      </div>
    </Layout>
  );
}
