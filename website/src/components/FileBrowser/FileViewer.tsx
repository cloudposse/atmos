/**
 * FileViewer - Displays file content with syntax highlighting.
 */
import React from 'react';
import CodeBlock from '@theme/CodeBlock';
import Markdown from 'react-markdown';
import SourceLink from './SourceLink';
import { formatFileSize, isBinaryFile, isMarkdownFile } from './utils';
import type { FileNode } from './types';
import styles from './styles.module.css';

interface FileViewerProps {
  file: FileNode;
}

export default function FileViewer({ file }: FileViewerProps): JSX.Element {
  const showGithubLink = !!file.githubUrl;

  // Handle binary files.
  if (isBinaryFile(file)) {
    return (
      <div className={styles.fileViewer}>
        <div className={styles.fileHeader}>
          <div className={styles.fileInfo}>
            <span className={styles.fileName}>{file.name}</span>
            <span className={styles.fileSize}>{formatFileSize(file.size)}</span>
          </div>
          {showGithubLink && <SourceLink url={file.githubUrl!} />}
        </div>
        <div className={styles.binaryFileMessage}>
          <p>Binary file - preview not available</p>
          {showGithubLink && (
            <p>
              <SourceLink url={file.githubUrl!} label="View on GitHub" />
            </p>
          )}
        </div>
      </div>
    );
  }

  // Handle missing content.
  if (!file.content) {
    return (
      <div className={styles.fileViewer}>
        <div className={styles.fileHeader}>
          <div className={styles.fileInfo}>
            <span className={styles.fileName}>{file.name}</span>
            <span className={styles.fileSize}>{formatFileSize(file.size)}</span>
          </div>
          {showGithubLink && <SourceLink url={file.githubUrl!} />}
        </div>
        <div className={styles.binaryFileMessage}>
          <p>File too large to preview ({formatFileSize(file.size)})</p>
          {showGithubLink && (
            <p>
              <SourceLink url={file.githubUrl!} label="View on GitHub" />
            </p>
          )}
        </div>
      </div>
    );
  }

  // Render markdown files as formatted content.
  if (isMarkdownFile(file)) {
    return (
      <div className={styles.fileViewer}>
        <div className={styles.fileHeader}>
          <div className={styles.fileInfo}>
            <span className={styles.fileName}>{file.name}</span>
            <span className={styles.fileSize}>{formatFileSize(file.size)}</span>
          </div>
          {showGithubLink && <SourceLink url={file.githubUrl!} />}
        </div>
        <div className={styles.markdownContent}>
          <Markdown
            components={{
              // Render code blocks with syntax highlighting.
              code({ className, children, ...props }) {
                const match = /language-(\w+)/.exec(className || '');
                const isInline = !match;
                return isInline ? (
                  <code className={className} {...props}>
                    {children}
                  </code>
                ) : (
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
      </div>
    );
  }

  // Render code files with syntax highlighting.
  return (
    <div className={styles.fileViewer}>
      <div className={styles.fileHeader}>
        <div className={styles.fileInfo}>
          <span className={styles.fileName}>{file.name}</span>
          <span className={styles.fileSize}>{formatFileSize(file.size)}</span>
        </div>
        {showGithubLink && <SourceLink url={file.githubUrl!} />}
      </div>
      <div className={styles.fileContent}>
        <CodeBlock language={file.language} showLineNumbers>
          {file.content}
        </CodeBlock>
      </div>
    </div>
  );
}
