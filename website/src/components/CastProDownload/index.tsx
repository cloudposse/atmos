import React, { useEffect, useRef, useState } from 'react';
import { FiAlertCircle, FiChevronDown, FiDownload, FiLoader } from 'react-icons/fi';

import { CAST_FORMATS } from '../CastProArtifact/url.mjs';
import { CastFormat, useCastArtifact } from '../CastProArtifact/useCastArtifact';

import styles from './styles.module.css';

export interface CastProDownloadProps {
  owner?: string;
  repo?: string;
  // Branch, tag, or full commit SHA. Named `gitRef` (not `ref`) because `ref`
  // is a reserved JSX attribute on components and would otherwise never reach
  // props.
  gitRef: string;
  path: string;
  formats?: CastFormat[];
  ttlSeconds?: number;
  soundtrack?: string;
  label?: string;
  className?: string;
}

interface FormatMenuItemProps {
  owner?: string;
  repo?: string;
  gitRef: string;
  path: string;
  format: CastFormat;
  ttlSeconds?: number;
  soundtrack?: string;
}

function FormatMenuItem({
  owner,
  repo,
  gitRef,
  path,
  format,
  ttlSeconds,
  soundtrack,
}: FormatMenuItemProps): JSX.Element {
  const { status, errorMessage, start } = useCastArtifact({
    owner,
    repo,
    ref: gitRef,
    path,
    format,
    ttlSeconds,
    soundtrack,
  });
  const busy = status === 'checking' || status === 'rendering';

  return (
    <div className={styles.menuItemGroup}>
      <button
        type="button"
        className={styles.menuItem}
        onClick={start}
        disabled={busy}
        title={`Download ${format.toUpperCase()}`}
      >
        {busy ? (
          <FiLoader className={`${styles.icon} ${styles.spin}`} aria-hidden="true" />
        ) : status === 'error' ? (
          <FiAlertCircle className={styles.icon} aria-hidden="true" />
        ) : (
          <FiDownload className={styles.icon} aria-hidden="true" />
        )}
        <span>{format.toUpperCase()}</span>
        {status === 'rendering' && <span className={styles.hint}>Rendering…</span>}
      </button>
      {status === 'error' && errorMessage && (
        <p className={styles.errorText} role="alert">
          {errorMessage}
        </p>
      )}
    </div>
  );
}

/**
 * "Download ▾" split button offering rendered GIF/MP4/SVG/WEBM artifacts of a
 * .cast file from the Atmos Pro cast-rendering service
 * (https://atmos-pro.com/casts/{owner}/{repo}/{ref}/{path}.cast.{format}).
 * Handles the render service's three response shapes: an already-rendered
 * artifact (triggers a native browser download), a still-rendering one
 * (polls on Retry-After, capped at ~60s), and a hard error (surfaces the
 * JSON error message inline).
 */
export default function CastProDownload({
  owner,
  repo,
  gitRef,
  path,
  formats = CAST_FORMATS as CastFormat[],
  ttlSeconds,
  soundtrack,
  label = 'Download',
  className,
}: CastProDownloadProps): JSX.Element {
  const [menuOpen, setMenuOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement | null>(null);

  // Close menu on outside click or Escape (same convention as CopyMarkdownButton).
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

  return (
    <div className={[styles.container, className].filter(Boolean).join(' ')} ref={rootRef}>
      <button
        type="button"
        className={styles.trigger}
        onClick={() => setMenuOpen((open) => !open)}
        aria-expanded={menuOpen}
        aria-label={`${label} cast`}
      >
        <FiDownload className={styles.icon} aria-hidden="true" />
        <span>{label}</span>
        <FiChevronDown className={styles.icon} aria-hidden="true" />
      </button>

      {menuOpen && (
        <div className={styles.menu}>
          {formats.map((format) => (
            <FormatMenuItem
              key={format}
              owner={owner}
              repo={repo}
              gitRef={gitRef}
              path={path}
              format={format}
              ttlSeconds={ttlSeconds}
              soundtrack={soundtrack}
            />
          ))}
        </div>
      )}
    </div>
  );
}
