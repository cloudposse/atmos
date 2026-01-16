/**
 * DirectoryListing - Simple file list without size column.
 */
import React from 'react';
import Link from '@docusaurus/Link';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { getFileIcon } from './utils';
import type { DirectoryNode, TreeNode } from './types';
import styles from './styles.module.css';

interface DirectoryListingProps {
  directory: DirectoryNode;
  routeBasePath: string;
}

export default function DirectoryListing({
  directory,
  routeBasePath,
}: DirectoryListingProps): JSX.Element {
  if (directory.children.length === 0) {
    return (
      <div className={styles.emptyState}>
        <div className={styles.emptyStateIcon}>
          <FontAwesomeIcon icon={getFileIcon(directory)} />
        </div>
        <h3 className={styles.emptyStateTitle}>Empty directory</h3>
        <p className={styles.emptyStateDescription}>
          This directory doesn&apos;t contain any files.
        </p>
      </div>
    );
  }

  return (
    <ul className={styles.fileList}>
      {directory.children.map((item: TreeNode) => (
        <li key={item.path} className={styles.fileListItem}>
          <Link
            to={`${routeBasePath}/${item.path}`}
            className={styles.fileListLink}
          >
            <FontAwesomeIcon
              icon={getFileIcon(item)}
              className={styles.fileListIcon}
            />
            <span className={styles.fileListName}>{item.name}</span>
          </Link>
        </li>
      ))}
    </ul>
  );
}
