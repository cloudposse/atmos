import React, { useEffect, useRef, useState } from 'react';
import { useReducedMotion } from 'framer-motion';
import useBaseUrl from '@docusaurus/useBaseUrl';
import CastPlayer from '@site/src/components/CastPlayer';
import './styles.css';

// Landing recordings are committed with the site, just like the example casts.
export default function DemoCast({ title, slug, className }) {
  const src = useBaseUrl(`/casts/demo/landing/${slug}.cast`);
  const reduceMotion = useReducedMotion();
  const [userPaused, setUserPaused] = useState(null);
  const [isVisible, setIsVisible] = useState(false);
  const viewportRef = useRef(null);
  const paused = userPaused ?? reduceMotion ?? false;

  useEffect(() => {
    if (typeof IntersectionObserver === 'undefined') {
      setIsVisible(true);
      return undefined;
    }
    const observer = new IntersectionObserver(([entry]) => setIsVisible(entry.isIntersecting));
    observer.observe(viewportRef.current);
    return () => observer.disconnect();
  }, []);

  return (
    <div className={className}>
      <div className="terminal landing-demo-cast" ref={viewportRef}>
        <div className="window-bar">
          <div className="window-controls" aria-hidden="true">
            <div className="control-dot close-dot"></div>
            <div className="control-dot minimize-dot"></div>
            <div className="control-dot maximize-dot"></div>
          </div>
          <div className="window-title">{title}</div>
        </div>
        <div className="landing-demo-cast-viewport">
          <CastPlayer src={src} title={title} chrome={false} controls={false}
            showCommand={false} autoplay loop paused={!isVisible || paused}
            static={Boolean(reduceMotion && userPaused === null)}
            speed={1} maxScrollRate={1} className="landing-demo-cast-player" />
          <button type="button" className="landing-demo-cast-toggle"
            aria-label={`${paused ? 'Play' : 'Pause'} ${title}`}
            onClick={() => setUserPaused(!paused)}>
            {paused ? 'Play' : 'Pause'}
          </button>
        </div>
      </div>
    </div>
  );
}
