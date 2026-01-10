import React, { useRef, useState, useEffect } from 'react';
import { Stream, StreamPlayerApi } from '@cloudflare/stream-react';

interface CloudflareStreamProps {
  src: string;
  controls?: boolean;
  autoplay?: boolean;
  muted?: boolean;
  loop?: boolean;
  className?: string;
  poster?: string;
  width?: string;
  height?: string;
  onEnded?: () => void;
}

// Build the src URL with HLS optimization parameters.
// These improve video startup quality by hinting at available bandwidth.
function buildOptimizedSrc(videoId: string): string {
  // clientBandwidthHint tells the player to assume good bandwidth and start with higher quality.
  // Value is in Kbps - 5000 = 5Mbps which should select 1080p on good connections.
  return `${videoId}?clientBandwidthHint=5000`;
}

export function CloudflareStream({
  src,
  controls = true,
  autoplay = false,
  muted = true,
  loop = false,
  className = '',
  poster,
  width,
  height,
  onEnded,
}: CloudflareStreamProps) {
  const streamRef = useRef<StreamPlayerApi | undefined>(undefined);
  const [hasInteracted, setHasInteracted] = useState(false);

  // Build optimized src with HLS parameters.
  const optimizedSrc = buildOptimizedSrc(src);

  // Update loop behavior based on interaction state.
  useEffect(() => {
    if (streamRef.current) {
      // Loop only if no interaction has occurred and the video is muted.
      const shouldLoop = loop || (!hasInteracted && muted);
      streamRef.current.loop = shouldLoop;
    }
  }, [hasInteracted, muted, loop]);

  const handleInteraction = () => {
    if (!hasInteracted) {
      setHasInteracted(true);
      if (streamRef.current) {
        streamRef.current.loop = loop;
      }
    }
  };

  // If no video ID provided, show placeholder.
  if (!src) {
    return (
      <div
        className={className}
        style={{
          width: width || '100%',
          aspectRatio: '16 / 9',
          backgroundColor: '#1a1a2e',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: '#666',
          fontSize: '0.875rem',
        }}
      >
        Video coming soon
      </div>
    );
  }

  return (
    <div className={className}>
      <Stream
        src={optimizedSrc}
        controls={controls}
        autoplay={autoplay}
        muted={muted}
        loop={loop || (!hasInteracted && muted)}
        poster={poster}
        width={width}
        height={height}
        streamRef={streamRef}
        onPlay={handleInteraction}
        onPause={handleInteraction}
        onSeeked={handleInteraction}
        onEnded={onEnded}
        responsive
      />
    </div>
  );
}
