/**
 * FileTreeNode - Recursive tree node component for sidebar navigation.
 */
import React, { useState } from 'react';
import Link from '@docusaurus/Link';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faChevronRight } from '@fortawesome/free-solid-svg-icons';
import { getFileIcon } from './utils';
import type { TreeNode, DirectoryNode } from './types';
import styles from './styles.module.css';

interface FileTreeNodeProps {
  node: TreeNode;
  routeBasePath: string;
  currentPath: string;
  depth?: number;
  defaultOpen?: boolean;
}

export default function FileTreeNode({
  node,
  routeBasePath,
  currentPath,
  depth = 0,
  defaultOpen = false,
}: FileTreeNodeProps): JSX.Element {
  // Determine if this directory should be open by default.
  const shouldBeOpen = defaultOpen || currentPath.startsWith(node.path);
  const [isOpen, setIsOpen] = useState(shouldBeOpen);

  const fullPath = `${routeBasePath}/${node.path}`;
  const isActive = currentPath === node.path;
  const icon = getFileIcon(node, isOpen);

  if (node.type === 'file') {
    return (
      <li className={styles.fileTreeItem}>
        <Link
          to={fullPath}
          className={`${styles.fileTreeLink} ${isActive ? styles.fileTreeLinkActive : ''}`}
        >
          <FontAwesomeIcon icon={icon} className={styles.fileTreeIcon} />
          <span className={styles.fileTreeName}>{node.name}</span>
        </Link>
      </li>
    );
  }

  const dirNode = node as DirectoryNode;

  return (
    <li className={styles.fileTreeItem}>
      <button
        type="button"
        className={styles.fileTreeToggle}
        onClick={() => setIsOpen(!isOpen)}
        aria-expanded={isOpen}
      >
        <FontAwesomeIcon
          icon={faChevronRight}
          className={`${styles.fileTreeChevron} ${isOpen ? styles.fileTreeChevronOpen : ''}`}
        />
        <FontAwesomeIcon icon={icon} className={styles.fileTreeIcon} />
        <span className={styles.fileTreeName}>{node.name}</span>
      </button>
      {isOpen && dirNode.children.length > 0 && (
        <ul className={styles.fileTreeChildren}>
          {dirNode.children.map((child) => (
            <FileTreeNode
              key={child.path}
              node={child}
              routeBasePath={routeBasePath}
              currentPath={currentPath}
              depth={depth + 1}
            />
          ))}
        </ul>
      )}
    </li>
  );
}
