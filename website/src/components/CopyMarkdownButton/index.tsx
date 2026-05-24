import React, { useEffect, useRef, useState } from 'react';
import { FiCheck, FiCopy, FiExternalLink } from 'react-icons/fi';

import styles from './styles.module.css';

interface CopyMarkdownButtonProps {
  href: string;
}

/**
 * Small split control rendered above each doc page that lets readers grab the
 * raw Markdown for that page (the same file announced via <link rel=alternate
 * type="text/markdown">). "Copy" fetches the .md sibling and writes it to the
 * clipboard; "View" opens it in a new tab.
 */
export default function CopyMarkdownButton({ href }: CopyMarkdownButtonProps): JSX.Element {
  const [state, setState] = useState<'idle' | 'copied' | 'error'>('idle');
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
    };
  }, []);

  async function handleCopy() {
    try {
      const res = await fetch(href, { headers: { Accept: 'text/markdown,text/plain,*/*' } });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const md = await res.text();
      if (typeof navigator === 'undefined' || !navigator.clipboard) {
        throw new Error('clipboard unavailable');
      }
      await navigator.clipboard.writeText(md);
      setState('copied');
    } catch {
      setState('error');
    }
    if (timeoutRef.current) clearTimeout(timeoutRef.current);
    timeoutRef.current = setTimeout(() => setState('idle'), 2000);
  }

  const copyLabel = state === 'copied' ? 'Copied!' : state === 'error' ? 'Copy failed' : 'Copy as Markdown';
  const CopyIcon = state === 'copied' ? FiCheck : FiCopy;

  return (
    <div className={styles.container} role="group" aria-label="Markdown source">
      <button
        type="button"
        className={styles.button}
        onClick={handleCopy}
        title="Copy this page as Markdown"
        aria-live="polite"
      >
        <CopyIcon className={styles.icon} aria-hidden="true" />
        <span>{copyLabel}</span>
      </button>
      <a
        href={href}
        target="_blank"
        rel="noopener noreferrer"
        className={styles.button}
        title="View this page as Markdown"
      >
        <FiExternalLink className={styles.icon} aria-hidden="true" />
        <span>View Markdown</span>
      </a>
    </div>
  );
}
