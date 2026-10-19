import React, { useEffect, useRef, useState } from 'react';
import { FiCheck, FiChevronDown, FiCode, FiLink, FiShare2 } from 'react-icons/fi';

import { buildEmbedUrl } from '../CastProArtifact/url.mjs';

import styles from './styles.module.css';

export interface CastShareLinkProps {
  owner?: string;
  repo?: string;
  // Branch, tag, or full commit SHA. Named `gitRef` (not `ref`) because `ref`
  // is a reserved JSX attribute on components and would otherwise never reach
  // props.
  gitRef: string;
  path: string;
  className?: string;
}

type CopyState = 'idle' | 'copied' | 'error';

/**
 * Split button offering ways to share a cast demo: the default action copies
 * a link to the current page, and the chevron menu also offers the Atmos Pro
 * hosted-player embed URL (built via CastProArtifact's buildEmbedUrl), for
 * embedding the demo elsewhere. Modeled on CopyMarkdownButton.
 */
export default function CastShareLink({ owner, repo, gitRef, path, className }: CastShareLinkProps): JSX.Element {
  const [menuOpen, setMenuOpen] = useState(false);
  const [copyState, setCopyState] = useState<CopyState>('idle');
  const resetTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const rootRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    return () => {
      if (resetTimeoutRef.current) clearTimeout(resetTimeoutRef.current);
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

  async function copy(text: string) {
    try {
      if (typeof navigator === 'undefined' || !navigator.clipboard) {
        throw new Error('clipboard unavailable');
      }
      await navigator.clipboard.writeText(text);
      setCopyState('copied');
    } catch {
      setCopyState('error');
    }
    if (resetTimeoutRef.current) clearTimeout(resetTimeoutRef.current);
    resetTimeoutRef.current = setTimeout(() => setCopyState('idle'), 2000);
  }

  function handleCopyPageLink() {
    setMenuOpen(false);
    void copy(typeof window !== 'undefined' ? window.location.href : '');
  }

  function handleCopyEmbedLink() {
    setMenuOpen(false);
    void copy(buildEmbedUrl({ owner, repo, ref: gitRef, path }));
  }

  const label = copyState === 'copied' ? 'Copied!' : copyState === 'error' ? 'Copy failed' : 'Share';
  const PrimaryIcon = copyState === 'copied' ? FiCheck : FiShare2;

  return (
    <div className={[styles.container, className].filter(Boolean).join(' ')} ref={rootRef}>
      <div className={styles.group} role="group" aria-label="Share this demo">
        <button
          type="button"
          className={styles.primary}
          onClick={handleCopyPageLink}
          title="Copy a link to this demo"
          aria-live="polite"
        >
          <PrimaryIcon className={styles.icon} aria-hidden="true" />
          <span>{label}</span>
        </button>
        <button
          type="button"
          className={styles.caret}
          onClick={() => setMenuOpen((open) => !open)}
          aria-expanded={menuOpen}
          aria-label="More share options"
          title="More share options"
        >
          <FiChevronDown className={styles.icon} aria-hidden="true" />
        </button>
      </div>

      {menuOpen && (
        <div className={styles.menu}>
          <button type="button" className={styles.menuItem} onClick={handleCopyPageLink}>
            <FiLink className={styles.icon} aria-hidden="true" />
            <span>Copy page link</span>
          </button>
          <button type="button" className={styles.menuItem} onClick={handleCopyEmbedLink}>
            <FiCode className={styles.icon} aria-hidden="true" />
            <span>Copy embed link</span>
          </button>
        </div>
      )}
    </div>
  );
}
