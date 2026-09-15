import React from 'react';
import Link from '@docusaurus/Link';
import styles from './styles.module.css';

export interface ProductUpdatesNavProps {
  activeView: 'changelog' | 'roadmap';
}

/** Link the update views and identify the current route for assistive technology. */
export default function ProductUpdatesNav({
  activeView,
}: ProductUpdatesNavProps): JSX.Element {
  return (
    <nav className={styles.navigation} aria-label="Product updates">
      <div className={styles.switch}>
        <Link
          to="/changelog"
          className={styles.link}
          aria-current={activeView === 'changelog' ? 'page' : undefined}
        >
          Changelog
        </Link>
        <Link
          to="/roadmap"
          className={styles.link}
          aria-current={activeView === 'roadmap' ? 'page' : undefined}
        >
          Roadmap
        </Link>
      </div>
    </nav>
  );
}
