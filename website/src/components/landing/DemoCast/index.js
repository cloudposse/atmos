import React, { useEffect, useRef, useState } from 'react';
import { useReducedMotion } from 'framer-motion';
import useBaseUrl from '@docusaurus/useBaseUrl';
import CastPlayer from '@site/src/components/CastPlayer';
import './styles.css';

// Landing recordings are committed with the site, just like the example casts.
export default function DemoCast({ title, slug, className }) {
  const src = useBaseUrl(`/casts/demo/landing/${slug}.cast`);
  const reduceMotion = useReducedMotion();
  const [isVisible, setIsVisible] = useState(false);
  const viewportRef = useRef(null);

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
          <CastPlayer src={src} title={title} chrome={false} controls scrubber
            showCommand={false} autoplay={!reduceMotion} loop paused={!isVisible}
            idleSkip={false} speed={1} maxScrollRate={1} className="landing-demo-cast-player" />
        </div>
      </div>
    </div>
  );
}
