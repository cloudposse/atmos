import React, { useEffect, useMemo, useRef, useState } from 'react';
import Link from '@docusaurus/Link';
import { useLocation } from '@docusaurus/router';
import useGlobalData from '@docusaurus/useGlobalData';
import CodeBlock from '@theme/CodeBlock';
import Mermaid from '@theme/Mermaid';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { RiCloseLine, RiExternalLinkLine, RiSideBarLine } from 'react-icons/ri';

import {
  findExampleByName,
  findNodeByPath,
  formatFileSize,
  getFileIcon,
  isBinaryFile,
  isMarkdownFile,
} from '@site/src/components/FileBrowser/utils';
import type {
  DirectoryNode,
  ExampleProject,
  FileBrowserOptions,
  FileNode,
  TreeNode,
} from '@site/src/components/FileBrowser/types';
import { getQuickStartExampleConfig } from './routeMap';
import styles from './styles.module.css';

interface QuickStartExampleDrawerProps {
  pageTitle?: string;
}

interface GlobalDataFileBrowser {
  examples: ExampleProject[];
  options: FileBrowserOptions;
}

const DEFAULT_DRAWER_WIDTH = 720;
const MIN_DRAWER_WIDTH = 420;
const MAX_DRAWER_WIDTH = 1000;
const DRAWER_VIEWPORT_MARGIN = 80;

function getMaxDrawerWidth(): number {
  if (typeof window === 'undefined') {
    return MAX_DRAWER_WIDTH;
  }
  return Math.max(
    MIN_DRAWER_WIDTH,
    Math.min(MAX_DRAWER_WIDTH, window.innerWidth - DRAWER_VIEWPORT_MARGIN)
  );
}

function clampDrawerWidth(width: number): number {
  return Math.min(Math.max(width, MIN_DRAWER_WIDTH), getMaxDrawerWidth());
}

function stripExamplePrefix(path: string, exampleName: string): string {
  return path.startsWith(`${exampleName}/`) ? path.slice(exampleName.length + 1) : path;
}

function findFirstFile(node: DirectoryNode): FileNode | null {
  for (const child of node.children) {
    if (child.type === 'file') {
      return child;
    }
    const found = findFirstFile(child);
    if (found) {
      return found;
    }
  }
  return null;
}

function findPreviewFile(example: ExampleProject, selectedPath: string): FileNode | null {
  const selectedNode = findNodeByPath(example.root, selectedPath);
  if (selectedNode?.type === 'file') {
    return selectedNode;
  }
  return example.root.readme || findFirstFile(example.root);
}

function isAncestorPath(nodePath: string, selectedPath: string): boolean {
  return selectedPath === nodePath || selectedPath.startsWith(`${nodePath}/`);
}

function FileTreeNode({
  node,
  selectedPath,
  onSelect,
  depth = 0,
}: {
  node: TreeNode;
  selectedPath: string;
  onSelect: (path: string) => void;
  depth?: number;
}): JSX.Element {
  const shouldOpen = node.type === 'directory' && isAncestorPath(node.path, selectedPath);
  const [isOpen, setIsOpen] = useState(shouldOpen);

  useEffect(() => {
    if (shouldOpen) {
      setIsOpen(true);
    }
  }, [shouldOpen, selectedPath]);

  const icon = getFileIcon(node, isOpen);

  if (node.type === 'file') {
    const isSelected = selectedPath === node.path;
    return (
      <li className={styles.treeItem}>
        <button
          type="button"
          className={`${styles.treeButton} ${isSelected ? styles.treeButtonActive : ''}`}
          style={{ paddingLeft: `${0.75 + depth * 0.85}rem` }}
          onClick={() => onSelect(node.path)}
          aria-current={isSelected ? 'true' : undefined}
          title={node.path}
        >
          <FontAwesomeIcon icon={icon} className={styles.treeIcon} />
          <span className={styles.treeName}>{node.name}</span>
        </button>
      </li>
    );
  }

  return (
    <li className={styles.treeItem}>
      <button
        type="button"
        className={styles.treeButton}
        style={{ paddingLeft: `${0.75 + depth * 0.85}rem` }}
        onClick={() => setIsOpen((open) => !open)}
        aria-expanded={isOpen}
        title={node.path}
      >
        <span className={`${styles.treeChevron} ${isOpen ? styles.treeChevronOpen : ''}`}>›</span>
        <FontAwesomeIcon icon={icon} className={styles.treeIcon} />
        <span className={styles.treeName}>{node.name}</span>
      </button>
      {isOpen && (
        <ul className={styles.treeChildren}>
          {node.children.map((child) => (
            <FileTreeNode
              key={child.path}
              node={child}
              selectedPath={selectedPath}
              onSelect={onSelect}
              depth={depth + 1}
            />
          ))}
        </ul>
      )}
    </li>
  );
}

function FilePreview({ file }: { file: FileNode }): JSX.Element {
  const showGithubLink = !!file.githubUrl;

  return (
    <div className={styles.preview}>
      <div className={styles.previewHeader}>
        <div className={styles.previewTitleGroup}>
          <span className={styles.previewName}>{file.name}</span>
          <span className={styles.previewMeta}>{formatFileSize(file.size)}</span>
        </div>
        {showGithubLink && (
          <Link
            to={file.githubUrl!}
            className={styles.sourceLink}
            target="_blank"
            rel="noopener noreferrer"
          >
            Source <RiExternalLinkLine />
          </Link>
        )}
      </div>
      <div className={styles.previewBody}>
        {isBinaryFile(file) ? (
          <p className={styles.emptyMessage}>Binary file preview is not available.</p>
        ) : file.content === '' ? (
          <p className={styles.emptyMessage}>File is empty.</p>
        ) : file.content == null ? (
          <p className={styles.emptyMessage}>
            File too large to preview ({formatFileSize(file.size)}).
          </p>
        ) : isMarkdownFile(file) ? (
          <div className={styles.markdownContent}>
            <Markdown
              remarkPlugins={[remarkGfm]}
              components={{
                code({ className, children, ...props }) {
                  const match = /language-(\w+)/.exec(className || '');
                  const isInline = !match;
                  if (isInline) {
                    return (
                      <code className={className} {...props}>
                        {children}
                      </code>
                    );
                  }
                  if (match[1] === 'mermaid') {
                    return <Mermaid value={String(children).replace(/\n$/, '')} />;
                  }
                  return (
                    <CodeBlock language={match[1]}>
                      {String(children).replace(/\n$/, '')}
                    </CodeBlock>
                  );
                },
              }}
            >
              {file.content}
            </Markdown>
          </div>
        ) : (
          <CodeBlock language={file.language} showLineNumbers>
            {file.content}
          </CodeBlock>
        )}
      </div>
    </div>
  );
}

export default function QuickStartExampleDrawer({
  pageTitle,
}: QuickStartExampleDrawerProps): JSX.Element | null {
  const { pathname, search, hash } = useLocation();
  const triggerRef = useRef<HTMLButtonElement>(null);
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const drawerRef = useRef<HTMLDivElement>(null);
  const previousActiveElement = useRef<HTMLElement | null>(null);
  const [isOpen, setIsOpen] = useState(false);
  const [selectedPath, setSelectedPath] = useState('');
  const [drawerWidth, setDrawerWidth] = useState(DEFAULT_DRAWER_WIDTH);

  const config = useMemo(() => getQuickStartExampleConfig(pathname), [pathname]);

  const globalData = useGlobalData();
  const fileBrowserData = globalData['file-browser']?.['examples'] as GlobalDataFileBrowser | undefined;
  const examples = fileBrowserData?.examples || [];
  const options = fileBrowserData?.options;
  const example = config ? findExampleByName(examples, config.exampleName) : undefined;

  useEffect(() => {
    if (config) {
      setSelectedPath(config.selectedPath);
    }
  }, [config?.selectedPath]);

  useEffect(() => {
    if (!config || !example) {
      return;
    }

    const handleOpenDrawer = (event: Event) => {
      const drawerEvent = event as CustomEvent<{ path?: string }>;
      const path = drawerEvent.detail?.path;
      if (path && findNodeByPath(example.root, path)) {
        setSelectedPath(path);
      }
      setIsOpen(true);
    };

    window.addEventListener('quick-start-example-drawer:open', handleOpenDrawer);
    return () => window.removeEventListener('quick-start-example-drawer:open', handleOpenDrawer);
  }, [config, example]);

  useEffect(() => {
    if (!config) {
      return;
    }

    const params = new URLSearchParams(search);
    if (hash === '#example' || params.get('example') === '1') {
      setIsOpen(true);
    }
  }, [config, hash, search]);

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && isOpen) {
        setIsOpen(false);
      }
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [isOpen]);

  useEffect(() => {
    const handleMouseDown = (event: MouseEvent) => {
      if (
        isOpen &&
        drawerRef.current &&
        !drawerRef.current.contains(event.target as Node)
      ) {
        setIsOpen(false);
      }
    };
    document.addEventListener('mousedown', handleMouseDown);
    return () => document.removeEventListener('mousedown', handleMouseDown);
  }, [isOpen]);

  useEffect(() => {
    if (isOpen) {
      previousActiveElement.current = document.activeElement as HTMLElement;
      document.body.style.overflow = 'hidden';
      setDrawerWidth((width) => clampDrawerWidth(width));
      window.setTimeout(() => closeButtonRef.current?.focus(), 100);
    } else {
      document.body.style.overflow = '';
      if (previousActiveElement.current) {
        previousActiveElement.current.focus();
        previousActiveElement.current = null;
      }
    }

    return () => {
      document.body.style.overflow = '';
    };
  }, [isOpen]);

  const handleResizePointerDown = (event: React.PointerEvent<HTMLButtonElement>) => {
    if (typeof window !== 'undefined' && window.innerWidth <= 640) {
      return;
    }

    event.preventDefault();
    const startX = event.clientX;
    const startWidth = drawerRef.current?.getBoundingClientRect().width || drawerWidth;
    const previousCursor = document.body.style.cursor;
    const previousUserSelect = document.body.style.userSelect;

    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';

    const handlePointerMove = (moveEvent: PointerEvent) => {
      setDrawerWidth(clampDrawerWidth(startWidth + startX - moveEvent.clientX));
    };

    const stopResize = () => {
      document.removeEventListener('pointermove', handlePointerMove);
      document.removeEventListener('pointerup', stopResize);
      document.body.style.cursor = previousCursor;
      document.body.style.userSelect = previousUserSelect;
    };

    document.addEventListener('pointermove', handlePointerMove);
    document.addEventListener('pointerup', stopResize);
  };

  const handleResizeKeyDown = (event: React.KeyboardEvent<HTMLButtonElement>) => {
    if (event.key === 'ArrowLeft') {
      event.preventDefault();
      setDrawerWidth((width) => clampDrawerWidth(width + 40));
    } else if (event.key === 'ArrowRight') {
      event.preventDefault();
      setDrawerWidth((width) => clampDrawerWidth(width - 40));
    } else if (event.key === 'Home') {
      event.preventDefault();
      setDrawerWidth(MIN_DRAWER_WIDTH);
    } else if (event.key === 'End') {
      event.preventDefault();
      setDrawerWidth(getMaxDrawerWidth());
    }
  };

  if (!config || !example || !options) {
    return null;
  }

  const currentFile = findPreviewFile(example, selectedPath);
  if (!currentFile) {
    return null;
  }

  const relatedFiles = config.relatedPaths
    .map((path) => findNodeByPath(example.root, path))
    .filter((node): node is FileNode => node?.type === 'file');
  const duplicateRelatedNames = new Set(
    relatedFiles
      .map((file) => file.name)
      .filter((name, index, names) => names.indexOf(name) !== index)
  );
  const drawerTitle = pageTitle || (config.exampleName === 'quick-start-simple' ? 'Simple Tutorial' : 'Advanced Tutorial');
  const exampleUrl = `${options.routeBasePath}/${example.name}`;

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        className={`${styles.trigger} ${isOpen ? styles.triggerHidden : ''}`}
        onClick={() => setIsOpen(true)}
        aria-expanded={isOpen}
        aria-controls="quick-start-example-drawer"
        aria-label="Open example drawer"
        title="Open example drawer"
      >
        <RiSideBarLine className={styles.triggerIcon} />
        <span>Example</span>
      </button>

      {isOpen && <div className={styles.backdrop} aria-hidden="true" />}

      <aside
        id="quick-start-example-drawer"
        ref={drawerRef}
        className={`${styles.drawer} ${isOpen ? styles.drawerOpen : ''}`}
        style={{ '--quick-start-drawer-width': `${drawerWidth}px` } as React.CSSProperties}
        role="dialog"
        aria-modal="true"
        aria-labelledby="quick-start-example-drawer-title"
        aria-hidden={!isOpen}
      >
        <button
          type="button"
          className={styles.resizeHandle}
          onPointerDown={handleResizePointerDown}
          onKeyDown={handleResizeKeyDown}
          aria-label="Resize example drawer"
          aria-orientation="vertical"
          role="separator"
          tabIndex={isOpen ? 0 : -1}
        >
          <span className={styles.resizeGrip} />
        </button>

        <header className={styles.header}>
          <div className={styles.headerText}>
            <span className={styles.kicker}>{example.name}</span>
            <h2 id="quick-start-example-drawer-title" className={styles.title}>
              {drawerTitle}
            </h2>
            <Link to={exampleUrl} className={styles.fullExampleLink}>
              View full example <RiExternalLinkLine />
            </Link>
          </div>
          <button
            ref={closeButtonRef}
            type="button"
            className={styles.closeButton}
            onClick={() => setIsOpen(false)}
            aria-label="Close example drawer"
          >
            <RiCloseLine />
          </button>
        </header>

        <div className={styles.content}>
          <section className={styles.navigator} aria-label={`${example.name} files`}>
            <div className={styles.pathLabel}>
              {stripExamplePrefix(currentFile.path, example.name)}
            </div>
            {relatedFiles.length > 1 && (
              <div className={styles.relatedFiles} aria-label="Related files">
                {relatedFiles.map((file) => (
                  <button
                    key={file.path}
                    type="button"
                    className={`${styles.relatedFile} ${file.path === currentFile.path ? styles.relatedFileActive : ''}`}
                    onClick={() => setSelectedPath(file.path)}
                    title={stripExamplePrefix(file.path, example.name)}
                  >
                    {duplicateRelatedNames.has(file.name)
                      ? stripExamplePrefix(file.path, example.name)
                      : file.name}
                  </button>
                ))}
              </div>
            )}
            <ul className={styles.tree}>
              {example.root.children.map((child) => (
                <FileTreeNode
                  key={child.path}
                  node={child}
                  selectedPath={currentFile.path}
                  onSelect={setSelectedPath}
                />
              ))}
            </ul>
          </section>

          <section className={styles.previewPanel} aria-label="Selected file preview">
            <FilePreview file={currentFile} />
          </section>
        </div>
      </aside>
    </>
  );
}
