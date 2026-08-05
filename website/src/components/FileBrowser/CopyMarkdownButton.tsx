/**
 * CopyMarkdownButton - Copies a directory's full content, nested files
 * included, to the clipboard as one Markdown document. Lets someone grab a
 * skill's complete context without installing it.
 */
import React, { useState } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faCopy, faCheck } from '@fortawesome/free-solid-svg-icons';
import { collectMarkdownContext } from './utils';
import type { DirectoryNode } from './types';
import styles from './styles.module.css';

interface CopyMarkdownButtonProps {
  directory: DirectoryNode;
  title?: string;
  description?: string;
  label?: string;
}

export default function CopyMarkdownButton({
  directory,
  title,
  description,
  label = 'Copy as Markdown',
}: CopyMarkdownButtonProps): JSX.Element {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    const heading = title ? `# ${title}\n\n${description ? `${description}\n\n` : ''}` : '';
    const markdown = heading + collectMarkdownContext(directory);

    try {
      await navigator.clipboard.writeText(markdown);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard API unavailable (e.g. insecure context) - button just won't confirm.
    }
  };

  return (
    <button type="button" className={styles.copyMarkdownButton} onClick={handleCopy}>
      <FontAwesomeIcon icon={copied ? faCheck : faCopy} className={styles.copyMarkdownIcon} />
      <span>{copied ? 'Copied!' : label}</span>
    </button>
  );
}
