/**
 * FileTree - Sidebar file tree navigation component.
 */
import React from 'react';
import Link from '@docusaurus/Link';
import FileTreeNode from './FileTreeNode';
import type { ExampleProject } from './types';
import styles from './styles.module.css';

interface FileTreeProps {
  example: ExampleProject;
  routeBasePath: string;
  currentPath: string;
}

export default function FileTree({
  example,
  routeBasePath,
  currentPath,
}: FileTreeProps): JSX.Element {
  return (
    <aside className={styles.sidebar}>
      <div className={styles.sidebarHeader}>
        <Link
          to={`${routeBasePath}/${example.name}`}
          className={styles.sidebarExampleLink}
        >
          {example.name}
        </Link>
      </div>
      <ul className={styles.fileTree}>
        {example.root.children.map((child) => (
          <FileTreeNode
            key={child.path}
            node={child}
            routeBasePath={routeBasePath}
            currentPath={currentPath}
            defaultOpen={currentPath.startsWith(child.path)}
          />
        ))}
      </ul>
    </aside>
  );
}
