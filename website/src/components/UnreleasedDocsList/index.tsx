import React from 'react';
import Link from '@docusaurus/Link';
import { usePluginData } from '@docusaurus/useGlobalData';
import styles from './styles.module.css';

interface UnreleasedDoc {
  path: string;
  title: string;
  description: string | null;
}

interface DocReleaseData {
  releaseMap: Record<string, string>;
  unreleasedDocs: UnreleasedDoc[];
  buildDate: string;
}

export default function UnreleasedDocsList(): JSX.Element {
  let data: DocReleaseData | undefined;
  try {
    data = usePluginData('doc-release-data') as DocReleaseData;
  } catch {
    return (
      <p className={styles.noFeatures}>
        Unable to load unreleased documentation data.
      </p>
    );
  }

  if (!data?.unreleasedDocs || data.unreleasedDocs.length === 0) {
    return (
      <p className={styles.noFeatures}>
        No documentation pages are currently marked as unreleased.
      </p>
    );
  }

  const buildDate = data.buildDate
    ? new Date(data.buildDate).toLocaleDateString('en-US', {
        year: 'numeric',
        month: 'long',
        day: 'numeric',
      })
    : null;

  return (
    <div className={styles.container}>
      {buildDate && (
        <p className={styles.buildDate}>
          This list was generated on <strong>{buildDate}</strong>.
        </p>
      )}
      <ul className={styles.docList}>
        {data.unreleasedDocs.map((doc, index) => (
          <li key={index} className={styles.docItem}>
            <Link to={doc.path} className={styles.docLink}>
              {doc.title}
            </Link>
            {doc.description && (
              <p className={styles.description}>{doc.description}</p>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}
