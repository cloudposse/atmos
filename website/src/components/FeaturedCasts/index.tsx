import React from 'react';
import Link from '@docusaurus/Link';
import CastPlayer from '@site/src/components/CastPlayer';
import styles from './styles.module.css';

export type FeaturedCast = {
  /** Directory name under examples/, used to build the /examples/<name> link. */
  example: string;
  /** Card heading (falls back to the example directory name). */
  title: string;
  /** Short description shown under the cast preview. */
  description: string;
  /** Path to the committed .cast recording, served from website/static/casts/. */
  castFile: string;
  /** Title shown in the CastPlayer chrome bar. */
  castTitle: string;
};

type Props = {
  casts: FeaturedCast[];
};

/**
 * A small, hand-picked list of example casts, mirroring the "Featured"
 * section on /examples so a docs page can showcase a taste of Atmos without
 * pulling in the full file-browser plugin.
 */
export default function FeaturedCasts({ casts }: Props): JSX.Element {
  return (
    <div className={styles.list}>
      {casts.map((cast) => (
        <Link
          key={cast.example}
          to={`/examples/${cast.example}`}
          className={styles.row}
          aria-label={`Open the ${cast.title} example`}
        >
          <div className={styles.rowCast}>
            <CastPlayer
              src={cast.castFile}
              title={cast.castTitle}
              chrome
              thumbnail
              controls={false}
              scrubber={false}
              showCommand={false}
            />
          </div>
          <div className={styles.rowBody}>
            <h3 className={styles.rowTitle}>{cast.title}</h3>
            <p className={styles.rowDescription}>{cast.description}</p>
          </div>
          <span className={styles.rowArrow} aria-hidden="true">
            →
          </span>
        </Link>
      ))}
    </div>
  );
}
