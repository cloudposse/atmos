/**
 * BreadcrumbNav - GitHub-style breadcrumb navigation.
 */
import React from 'react';
import Link from '@docusaurus/Link';
import styles from './styles.module.css';

interface BreadcrumbNavProps {
  path: string;
  routeBasePath: string;
  rootLabel?: string;
}

export default function BreadcrumbNav({
  path,
  routeBasePath,
  rootLabel = 'examples',
}: BreadcrumbNavProps): JSX.Element {
  const parts = path.split('/').filter(Boolean);

  return (
    <nav className={styles.breadcrumb}>
      <Link to={routeBasePath} className={styles.breadcrumbLink}>
        {rootLabel}
      </Link>
      {parts.map((part, index) => {
        const isLast = index === parts.length - 1;
        const partPath = `${routeBasePath}/${parts.slice(0, index + 1).join('/')}`;

        return (
          <React.Fragment key={partPath}>
            <span className={styles.breadcrumbSeparator}>/</span>
            {isLast ? (
              <span className={styles.breadcrumbCurrent}>{part}</span>
            ) : (
              <Link to={partPath} className={styles.breadcrumbLink}>
                {part}
              </Link>
            )}
          </React.Fragment>
        );
      })}
    </nav>
  );
}
