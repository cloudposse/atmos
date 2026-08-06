/**
 * CopyMarkdownButton - Copies a directory's full content, nested files
 * included, to the clipboard as one Markdown document. Lets someone grab a
 * skill's complete context without installing it.
 */
import React, { useEffect, useRef, useState } from 'react';
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
  /** Renders as an icon-only control (no visible label) for tight spaces like a card corner. */
  iconOnly?: boolean;
  className?: string;
}

export default function CopyMarkdownButton({
  directory,
  title,
  description,
  label = 'Copy as Markdown',
  iconOnly = false,
  className,
}: CopyMarkdownButtonProps): JSX.Element {
  const [copied, setCopied] = useState(false);
  const resetTimeoutRef = useRef<ReturnType<typeof setTimeout>>();

  useEffect(() => () => clearTimeout(resetTimeoutRef.current), []);

  const handleCopy = async (event: React.MouseEvent) => {
    // Cards this button sits on may be clickable themselves - never let the
    // click fall through to a parent link/card navigation.
    event.preventDefault();
    event.stopPropagation();

    const heading = title ? `# ${title}\n\n${description ? `${description}\n\n` : ''}` : '';
    const markdown = heading + collectMarkdownContext(directory);

    try {
      await navigator.clipboard.writeText(markdown);
      setCopied(true);
      clearTimeout(resetTimeoutRef.current);
      resetTimeoutRef.current = setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard API unavailable (e.g. insecure context) - button just won't confirm.
    }
  };

  const buttonClassName = [
    iconOnly ? styles.copyMarkdownIconButton : styles.copyMarkdownButton,
    className,
  ].filter(Boolean).join(' ');

  return (
    <button
      type="button"
      className={buttonClassName}
      onClick={handleCopy}
      aria-label={copied ? 'Copied!' : label}
      title={iconOnly ? (copied ? 'Copied!' : label) : undefined}
    >
      <FontAwesomeIcon icon={copied ? faCheck : faCopy} className={styles.copyMarkdownIcon} />
      {!iconOnly && <span>{copied ? 'Copied!' : label}</span>}
    </button>
  );
}
