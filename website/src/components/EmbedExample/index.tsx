/**
 * EmbedExample - Embeds an example from the file browser into documentation pages.
 */
import React, { useState } from 'react';
import Link from '@docusaurus/Link';
import useGlobalData from '@docusaurus/useGlobalData';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import CodeBlock from '@theme/CodeBlock';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {
  faFolder,
  faFile,
  faChevronRight,
  faChevronDown,
  faArrowRight,
} from '@fortawesome/free-solid-svg-icons';
import type { ExampleProject, TreeNode, FileBrowserOptions } from '../FileBrowser/types';
import styles from './styles.module.css';

interface EmbedExampleProps {
  /** Name of the example (e.g., "demo-vendoring"). */
  example: string;
  /** Whether to show the README content. Default: true. */
  showReadme?: boolean;
  /** Whether to show the file listing. Default: true. */
  showFiles?: boolean;
  /** Override the display title. */
  title?: string;
  /** Maximum number of files to show. Default: 10. */
  maxFiles?: number;
}

interface GlobalDataFileBrowser {
  examples: ExampleProject[];
  options: FileBrowserOptions;
}

/**
 * Collects files from a tree node up to a limit.
 */
function collectFiles(node: TreeNode, limit: number, collected: TreeNode[] = []): TreeNode[] {
  if (collected.length >= limit) return collected;

  if (node.type === 'file') {
    collected.push(node);
  } else if (node.type === 'directory' && node.children) {
    for (const child of node.children) {
      if (collected.length >= limit) break;
      collectFiles(child, limit, collected);
    }
  }

  return collected;
}

/**
 * Strips frontmatter from README content.
 */
function stripFrontmatter(content: string): string {
  if (!content.startsWith('---')) return content;

  const endIndex = content.indexOf('---', 3);
  if (endIndex === -1) return content;

  return content.slice(endIndex + 3).trim();
}

/**
 * Markdown components for rendering README content.
 */
const markdownComponents = {
  code({ className, children, ...props }: { className?: string; children?: React.ReactNode }) {
    const match = /language-(\w+)/.exec(className || '');
    const isInline = !match;
    return isInline ? (
      <code className={className} {...props}>
        {children}
      </code>
    ) : (
      <CodeBlock language={match[1]}>{String(children).replace(/\n$/, '')}</CodeBlock>
    );
  },
};

export default function EmbedExample({
  example,
  showReadme = true,
  showFiles = true,
  title,
  maxFiles = 10,
}: EmbedExampleProps): JSX.Element | null {
  const [isReadmeExpanded, setIsReadmeExpanded] = useState(true);
  const [isFilesExpanded, setIsFilesExpanded] = useState(true);

  // Get global data from the file-browser plugin.
  const globalData = useGlobalData();
  const fileBrowserData = globalData['file-browser']?.['examples'] as GlobalDataFileBrowser | undefined;

  if (!fileBrowserData) {
    console.warn(`[EmbedExample] No file-browser data found in global data`);
    return null;
  }

  const { examples, options } = fileBrowserData;
  const exampleData = examples.find((ex) => ex.name === example);

  if (!exampleData) {
    console.warn(`[EmbedExample] Example "${example}" not found`);
    return null;
  }

  const displayTitle = title || exampleData.name;
  const readmeContent = exampleData.root.readme?.content;
  const files = collectFiles(exampleData.root, maxFiles);
  const exampleUrl = `${options.routeBasePath}/${exampleData.name}`;

  return (
    <div className={styles.embedExample}>
      {/* Header */}
      <div className={styles.header}>
        <Link to={exampleUrl} className={styles.headerLink}>
          <div className={styles.headerIcon}>
            <FontAwesomeIcon icon={faFolder} />
          </div>
          <span className={styles.headerTitle}>{displayTitle}</span>
        </Link>
        <Link to={exampleUrl} className={styles.viewFullLink}>
          View full example <FontAwesomeIcon icon={faArrowRight} />
        </Link>
      </div>

      {/* README Section */}
      {showReadme && readmeContent && (
        <div className={styles.section}>
          <button
            type="button"
            className={styles.sectionToggle}
            onClick={() => setIsReadmeExpanded(!isReadmeExpanded)}
          >
            <FontAwesomeIcon
              icon={isReadmeExpanded ? faChevronDown : faChevronRight}
              className={styles.toggleIcon}
            />
            <span>README</span>
          </button>
          {isReadmeExpanded && (
            <div className={styles.readmeContent}>
              <Markdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
                {stripFrontmatter(readmeContent)}
              </Markdown>
            </div>
          )}
        </div>
      )}

      {/* Files Section */}
      {showFiles && files.length > 0 && (
        <div className={styles.section}>
          <button
            type="button"
            className={styles.sectionToggle}
            onClick={() => setIsFilesExpanded(!isFilesExpanded)}
          >
            <FontAwesomeIcon
              icon={isFilesExpanded ? faChevronDown : faChevronRight}
              className={styles.toggleIcon}
            />
            <span>Files ({exampleData.root.fileCount})</span>
          </button>
          {isFilesExpanded && (
            <ul className={styles.fileList}>
              {files.map((file) => (
                <li key={file.path} className={styles.fileItem}>
                  <Link
                    to={`${options.routeBasePath}/${file.path}`}
                    className={styles.fileLink}
                  >
                    <FontAwesomeIcon
                      icon={file.type === 'directory' ? faFolder : faFile}
                      className={styles.fileIcon}
                    />
                    <span className={styles.fileName}>{file.path}</span>
                  </Link>
                </li>
              ))}
              {exampleData.root.fileCount > maxFiles && (
                <li className={styles.fileItem}>
                  <Link to={exampleUrl} className={styles.moreFilesLink}>
                    ... and {exampleData.root.fileCount - maxFiles} more files
                  </Link>
                </li>
              )}
            </ul>
          )}
        </div>
      )}

      {/* Footer */}
      <div className={styles.footer}>
        <Link to={exampleUrl} className={styles.footerLink}>
          Browse all files <FontAwesomeIcon icon={faArrowRight} />
        </Link>
      </div>
    </div>
  );
}
