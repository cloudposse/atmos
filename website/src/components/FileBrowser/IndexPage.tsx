/**
 * IndexPage - Landing page showing all example projects with category filtering.
 */
import React, { useState } from 'react';
import Layout from '@theme/Layout';
import Link from '@docusaurus/Link';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faFolder } from '@fortawesome/free-solid-svg-icons';
import type { ExamplesTree, FileBrowserOptions } from './types';
import styles from './styles.module.css';

interface IndexPageProps {
  treeData: ExamplesTree;
  optionsData: FileBrowserOptions;
}

export default function IndexPage({ treeData, optionsData }: IndexPageProps): JSX.Element {
  const { examples, categories } = treeData;
  const { routeBasePath, title, description } = optionsData;
  const [activeCategory, setActiveCategory] = useState<string | null>(null);

  const filteredExamples = activeCategory
    ? examples.filter((ex) => ex.category === activeCategory)
    : examples;

  return (
    <Layout title={title} description={description}>
      <div className={styles.indexPage}>
        <header className={styles.indexHeader}>
          <h1 className={styles.indexTitle}>{title}</h1>
          <p className={styles.indexDescription}>{description}</p>
        </header>

        <div className={styles.filterBar}>
          <button
            type="button"
            className={`${styles.filterButton} ${activeCategory === null ? styles.filterButtonActive : ''}`}
            onClick={() => setActiveCategory(null)}
          >
            All
          </button>
          {categories.map((category) => (
            <button
              key={category}
              type="button"
              className={`${styles.filterButton} ${activeCategory === category ? styles.filterButtonActive : ''}`}
              onClick={() => setActiveCategory(category)}
            >
              {category}
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
              <p className={styles.exampleCardDescription}>
                {example.description || 'Explore this example project'}
              </p>
              <div className={styles.exampleCardFooter}>
                <span className={styles.categoryBadge}>{example.category}</span>
              </div>
            </Link>
          ))}
        </div>
      </div>
    </Layout>
  );
}
