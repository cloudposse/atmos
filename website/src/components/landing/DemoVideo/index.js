import React, { useEffect, useRef, useState } from 'react';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import useBaseUrl from '@docusaurus/useBaseUrl';
import './styles.css';

// DemoVideo renders an autoplaying, looping terminal recording (produced by VHS,
// see demo/landing/*.tape) inside the same window chrome as <Screengrab>, so the
// landing page's visual language is unchanged whether a section shows a static
// ANSI capture or a moving recording.
//
// The rendered binaries are NEVER committed to git. They are synced to S3/CDN and
// referenced from `customFields.demosBaseUrl` in production. When that field is
// empty (local dev), we fall back to the gitignored local copy under
// /img/demos/<slug>.{webm,mp4} so a maintainer can preview before publishing.
export default function DemoVideo({ title, slug, className }) {
  const { siteConfig } = useDocusaurusContext();
  const remoteBase = siteConfig.customFields?.demosBaseUrl || '';

  const [isVisible, setIsVisible] = useState(false);
  const [reduceMotion, setReduceMotion] = useState(false);
  const viewportRef = useRef(null);

  useEffect(() => {
    const media = window.matchMedia('(prefers-reduced-motion: reduce)');
    const sync = () => setReduceMotion(media.matches);
    sync();
    media.addEventListener?.('change', sync);
    return () => media.removeEventListener?.('change', sync);
  }, []);

  useEffect(() => {
    const node = viewportRef.current;
    if (!node) return undefined;

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setIsVisible(true);
          observer.disconnect();
        }
      },
      { rootMargin: '200px', threshold: 0.1 },
    );

    observer.observe(node);
    return () => observer.disconnect();
  }, []);

  // Local fallbacks are resolved through useBaseUrl so the site baseUrl prefix is
  // respected; the remote base (CDN) is used verbatim when configured.
  const localWebm = useBaseUrl(`/img/demos/${slug}.webm`);
  const localMp4 = useBaseUrl(`/img/demos/${slug}.mp4`);
  const localPoster = useBaseUrl(`/img/demos/${slug}.png`);

  const webm = remoteBase ? `${remoteBase}/${slug}.webm` : localWebm;
  const mp4 = remoteBase ? `${remoteBase}/${slug}.mp4` : localMp4;
  const poster = remoteBase ? `${remoteBase}/${slug}.png` : localPoster;

  return (
    <div className={className}>
      <div className="terminal">
        <div className="window-bar">
          <div className="window-controls">
            <div className="control-dot close-dot"></div>
            <div className="control-dot minimize-dot"></div>
            <div className="control-dot maximize-dot"></div>
          </div>
          <div className="window-title">{title}</div>
        </div>
        <div className="viewport viewport--video" ref={viewportRef}>
          {isVisible ? (
            <video
              className="demo-video"
              autoPlay={!reduceMotion}
              muted
              loop={!reduceMotion}
              controls={reduceMotion}
              playsInline
              preload="metadata"
              poster={poster}
              aria-label={title}
            >
              <source src={webm} type="video/webm" />
              <source src={mp4} type="video/mp4" />
            </video>
          ) : (
            <div className="demo-video demo-video--placeholder" aria-hidden="true" />
          )}
        </div>
      </div>
    </div>
  );
}
