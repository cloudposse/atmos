import React, { useEffect, useRef } from 'react';
import Link from '@docusaurus/Link';
import { usePluginData } from '@docusaurus/useGlobalData';
import BrowserOnly from '@docusaurus/BrowserOnly';
import styles from './styles.module.css';

interface UnreleasedDoc {
  path: string;
  title: string;
  description: string | null;
}

interface DocReleaseData {
  releaseMap: Record<string, string>;
  unreleasedDocs: UnreleasedDoc[];
  buildDate: string;
}

/**
 * Fires confetti animation on first page load.
 * Uses canvas-confetti library for the celebration effect.
 */
function ConfettiCelebration(): JSX.Element | null {
  const hasRun = useRef(false);

  useEffect(() => {
    // Only fire once per page load.
    if (hasRun.current) return;
    hasRun.current = true;

    // Dynamically import canvas-confetti to avoid SSR issues.
    import('canvas-confetti').then((confettiModule) => {
      const confetti = confettiModule.default;

      // Fire confetti from both sides.
      const duration = 3000;
      const end = Date.now() + duration;

      const frame = () => {
        // Left side.
        confetti({
          particleCount: 3,
          angle: 60,
          spread: 55,
          origin: { x: 0, y: 0.6 },
          colors: ['#22c55e', '#3b82f6', '#f59e0b', '#ec4899'],
        });

        // Right side.
        confetti({
          particleCount: 3,
          angle: 120,
          spread: 55,
          origin: { x: 1, y: 0.6 },
          colors: ['#22c55e', '#3b82f6', '#f59e0b', '#ec4899'],
        });

        if (Date.now() < end) {
          requestAnimationFrame(frame);
        }
      };

      frame();
    });
  }, []);

  return null;
}

/**
 * Component shown when all documentation is released.
 */
function AllReleasedMessage({ buildDate }: { buildDate: string | null }): JSX.Element {
  return (
    <div className={styles.allReleasedContainer}>
      <BrowserOnly>{() => <ConfettiCelebration />}</BrowserOnly>
      <div className={styles.allReleasedIcon}>🎉</div>
      <h3 className={styles.allReleasedTitle}>All caught up!</h3>
      <p className={styles.allReleasedText}>
        All documentation is up to date with the latest release. There are no unreleased changes pending.
      </p>
      {buildDate && (
        <p className={styles.buildDateSmall}>
          Last checked on {buildDate}
        </p>
      )}
      <div className={styles.allReleasedLinks}>
        <Link to="/changelog" className={styles.allReleasedLink}>
          View Changelog
        </Link>
        <Link to="https://github.com/cloudposse/atmos/releases" className={styles.allReleasedLink}>
          GitHub Releases
        </Link>
      </div>
    </div>
  );
}

export default function UnreleasedDocsList(): JSX.Element {
  // Get the release data from our plugin's global data.
  // usePluginData returns undefined if the plugin isn't loaded.
  const data = usePluginData('doc-release-data') as DocReleaseData | undefined;

  if (!data) {
    return (
      <p className={styles.noFeatures}>
        Unable to load unreleased documentation data.
      </p>
    );
  }

  const buildDate = data?.buildDate
    ? new Date(data.buildDate).toLocaleDateString('en-US', {
        year: 'numeric',
        month: 'long',
        day: 'numeric',
      })
    : null;

  if (!data?.unreleasedDocs || data.unreleasedDocs.length === 0) {
    return <AllReleasedMessage buildDate={buildDate} />;
  }

  return (
    <div className={styles.container}>
      {buildDate && (
        <p className={styles.buildDate}>
          This list was generated on <strong>{buildDate}</strong>.
        </p>
      )}
      <ul className={styles.docList}>
        {data.unreleasedDocs.map((doc, index) => (
          <li key={index} className={styles.docItem}>
            <Link to={doc.path} className={styles.docLink}>
              {doc.title}
            </Link>
            {doc.description && (
              <p className={styles.description}>{doc.description}</p>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}
