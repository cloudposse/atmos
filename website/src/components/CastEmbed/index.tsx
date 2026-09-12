import React from 'react';

import CastPlayer from '@site/src/components/CastPlayer';
import CastProDownload from '@site/src/components/CastProDownload';
import CastShareLink from '@site/src/components/CastShareLink';
import { siteCastPath } from '@site/src/components/CastProArtifact/url.mjs';
import type { CastFormat } from '@site/src/components/CastProArtifact/useCastArtifact';

import styles from './styles.module.css';

type CastPlayerProps = React.ComponentProps<typeof CastPlayer>;

export interface CastEmbedProps extends CastPlayerProps {
  owner?: string;
  repo?: string;
  gitRef?: string;
  download?: boolean;
  share?: boolean;
  formats?: CastFormat[];
  ttlSeconds?: number;
  soundtrack?: string;
}

/**
 * `CastPlayer` plus the Atmos Pro "Download" and "Share" controls, wired to
 * the same repo path convention as the examples-gallery pages
 * (`FileBrowser/DirectoryPage.tsx`). Used for blog/changelog cast embeds so
 * readers can download a rendered GIF/MP4/SVG/WEBM or share the cast, not
 * just play it inline.
 */
export default function CastEmbed({
  owner,
  repo,
  gitRef = 'main',
  download = true,
  share = true,
  formats,
  ttlSeconds,
  soundtrack,
  ...playerProps
}: CastEmbedProps): JSX.Element {
  const path = siteCastPath(playerProps.src);
  const showActions = (download || share) && !playerProps.static;

  return (
    <div>
      <CastPlayer {...playerProps} />
      {showActions && (
        <div className={styles.castActions}>
          {share && (
            <CastShareLink owner={owner} repo={repo} gitRef={gitRef} path={path} />
          )}
          {download && (
            <CastProDownload
              owner={owner}
              repo={repo}
              gitRef={gitRef}
              path={path}
              formats={formats}
              ttlSeconds={ttlSeconds}
              soundtrack={soundtrack}
            />
          )}
        </div>
      )}
    </div>
  );
}
