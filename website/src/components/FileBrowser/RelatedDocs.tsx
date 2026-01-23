/**
 * RelatedDocs - Displays links to related documentation pages.
 */
import React from 'react';
import Link from '@docusaurus/Link';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faBook, faArrowRight } from '@fortawesome/free-solid-svg-icons';
import type { DocLink } from './types';
import styles from './styles.module.css';

interface RelatedDocsProps {
  docs: DocLink[];
}

export default function RelatedDocs({ docs }: RelatedDocsProps): JSX.Element | null {
  if (!docs || docs.length === 0) {
    return null;
  }

  return (
    <div className={styles.relatedDocs}>
      <h3 className={styles.relatedDocsTitle}>
        <FontAwesomeIcon icon={faBook} className={styles.relatedDocsIcon} />
        Related Documentation
      </h3>
      <ul className={styles.relatedDocsList}>
        {docs.map((doc) => (
          <li key={doc.url} className={styles.relatedDocsItem}>
            <Link to={doc.url} className={styles.relatedDocsLink}>
              {doc.label}
              <FontAwesomeIcon icon={faArrowRight} className={styles.relatedDocsArrow} />
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
