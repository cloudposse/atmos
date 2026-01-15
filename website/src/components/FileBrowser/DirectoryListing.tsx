/**
 * DirectoryListing - GitHub-style directory file table.
 */
import React from 'react';
import Link from '@docusaurus/Link';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { getFileIcon, formatFileSize } from './utils';
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
    <table className={styles.directoryTable}>
      <thead>
        <tr>
          <th>Name</th>
          <th>Size</th>
        </tr>
      </thead>
      <tbody>
        {directory.children.map((item: TreeNode) => (
          <tr key={item.path}>
            <td>
              <Link
                to={`${routeBasePath}/${item.path}`}
                className={styles.fileLink}
              >
                <FontAwesomeIcon
                  icon={getFileIcon(item)}
                  className={styles.fileLinkIcon}
                />
                <span>{item.name}</span>
              </Link>
            </td>
            <td className={styles.sizeCell}>
              {item.type === 'file' ? formatFileSize(item.size) : '-'}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
