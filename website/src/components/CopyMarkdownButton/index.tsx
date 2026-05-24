import React, { useEffect, useRef, useState } from 'react';
import { FiCheck, FiChevronDown, FiCopy, FiExternalLink } from 'react-icons/fi';

import styles from './styles.module.css';

interface CopyMarkdownButtonProps {
  href: string;
}

type CopyState = 'idle' | 'copied' | 'error';

/**
 * Split button rendered above each doc page. The default action (clicking the
 * label) copies the page's raw Markdown to the clipboard. The chevron opens a
 * small menu with alternate actions (view in new tab, future: open in
 * ChatGPT/Claude/etc).
 */
export default function CopyMarkdownButton({ href }: CopyMarkdownButtonProps): JSX.Element {
  const [copyState, setCopyState] = useState<CopyState>('idle');
  const [menuOpen, setMenuOpen] = useState(false);
  const copyTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const rootRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    return () => {
      if (copyTimeoutRef.current) clearTimeout(copyTimeoutRef.current);
    };
  }, []);

  // Close menu on outside click or Escape.
  useEffect(() => {
    if (!menuOpen) return;
    function onPointer(e: MouseEvent) {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setMenuOpen(false);
      }
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') setMenuOpen(false);
    }
    document.addEventListener('mousedown', onPointer);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onPointer);
      document.removeEventListener('keydown', onKey);
    };
  }, [menuOpen]);

  async function handleCopy() {
    try {
      const res = await fetch(href, { headers: { Accept: 'text/markdown,text/plain,*/*' } });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const md = await res.text();
      if (typeof navigator === 'undefined' || !navigator.clipboard) {
        throw new Error('clipboard unavailable');
      }
      await navigator.clipboard.writeText(md);
      setCopyState('copied');
    } catch {
      setCopyState('error');
    }
    if (copyTimeoutRef.current) clearTimeout(copyTimeoutRef.current);
    copyTimeoutRef.current = setTimeout(() => setCopyState('idle'), 2000);
  }

  function handlePrimary() {
    setMenuOpen(false);
    void handleCopy();
  }

  function handleMenuCopy(e: React.MouseEvent) {
    e.preventDefault();
    setMenuOpen(false);
    void handleCopy();
  }

  function handleMenuView() {
    setMenuOpen(false);
    // The <a> tag handles navigation; just close the menu.
  }

  const label = copyState === 'copied' ? 'Copied!' : copyState === 'error' ? 'Copy failed' : 'Copy Markdown';
  const PrimaryIcon = copyState === 'copied' ? FiCheck : FiCopy;

  return (
    <div className={styles.container} ref={rootRef}>
      <div className={styles.group} role="group" aria-label="Markdown source">
        <button
          type="button"
          className={styles.primary}
          onClick={handlePrimary}
          title="Copy this page as Markdown"
          aria-live="polite"
        >
          <PrimaryIcon className={styles.icon} aria-hidden="true" />
          <span>{label}</span>
        </button>
        <button
          type="button"
          className={styles.caret}
          onClick={() => setMenuOpen((v) => !v)}
          aria-haspopup="menu"
          aria-expanded={menuOpen}
          aria-label="More Markdown actions"
          title="More Markdown actions"
        >
          <FiChevronDown className={styles.icon} aria-hidden="true" />
        </button>
      </div>

      {menuOpen && (
        <div className={styles.menu} role="menu">
          <a
            href={href}
            className={styles.menuItem}
            onClick={handleMenuCopy}
            role="menuitem"
          >
            <FiCopy className={styles.icon} aria-hidden="true" />
            <span>Copy as Markdown</span>
          </a>
          <a
            href={href}
            target="_blank"
            rel="noopener noreferrer"
            className={styles.menuItem}
            onClick={handleMenuView}
            role="menuitem"
          >
            <FiExternalLink className={styles.icon} aria-hidden="true" />
            <span>View Markdown</span>
          </a>
        </div>
      )}
    </div>
  );
}
