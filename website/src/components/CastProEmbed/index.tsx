import React from 'react';

import { buildEmbedUrl } from '../CastProArtifact/url.mjs';

export interface CastProEmbedProps {
  owner?: string;
  repo?: string;
  // Branch, tag, or full commit SHA. Named `gitRef` (not `ref`) because `ref`
  // is a reserved JSX attribute on components and would otherwise never reach
  // props.
  gitRef: string;
  path: string;
  ttlSeconds?: number;
  title: string;
  className?: string;
}

/**
 * Embeds the Atmos Pro cast-rendering service's hosted HTML player
 * (https://atmos-pro.com/casts/{owner}/{repo}/{ref}/{path}.cast, no format
 * suffix) in an <iframe>, mirroring the sandbox convention used by
 * CloudPosseOfficeHoursEmbed/CloudPosseSlackEmbed.
 */
export default function CastProEmbed({
  owner,
  repo,
  gitRef,
  path,
  ttlSeconds,
  title,
  className,
}: CastProEmbedProps): JSX.Element {
  const src = buildEmbedUrl({ owner, repo, ref: gitRef, path, ttlSeconds });

  return (
    <iframe
      src={src}
      title={title}
      className={className}
      style={{
        width: '100%',
        aspectRatio: '16 / 9',
        border: '0',
        borderRadius: '0.375rem',
      }}
      loading="lazy"
      sandbox="allow-same-origin allow-scripts allow-forms allow-popups"
    />
  );
}
