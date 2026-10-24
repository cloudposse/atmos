import React, { useEffect, useState } from 'react';
import './index.css';

/**
 * Watermark renders a fixed-position clickable Cloud Posse logo
 * in the bottom-right corner of the page.
 *
 * To reduce clutter over the hero, it stays hidden until the visitor scrolls,
 * then fades in. Theme switching is handled via CSS using [data-theme] selectors.
 */
export default function Watermark(): JSX.Element {
  const [scrolled, setScrolled] = useState(false);

  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 80);
    onScroll();
    window.addEventListener('scroll', onScroll, { passive: true });
    return () => window.removeEventListener('scroll', onScroll);
  }, []);

  return (
    <a
      href="https://cloudposse.com"
      target="_blank"
      rel="noopener noreferrer"
      className={`cloudposse-logo${scrolled ? ' cloudposse-logo--visible' : ''}`}
      aria-label="Cloud Posse - Visit cloudposse.com"
      title="Cloud Posse"
    >
      <img
        src="/img/cloudposse-light.svg"
        alt="Cloud Posse"
        loading="lazy"
        className="cloudposse-logo__light"
      />
      <img
        src="/img/cloudposse-opaque.svg"
        alt="Cloud Posse"
        loading="lazy"
        className="cloudposse-logo__dark"
      />
    </a>
  );
}
