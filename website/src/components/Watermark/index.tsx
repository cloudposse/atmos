import React from 'react';
import './index.css';

/**
 * Watermark renders a fixed-position clickable Cloud Posse logo
 * in the bottom-right corner of the page.
 *
 * Theme switching is handled via CSS using [data-theme] selectors.
 */
export default function Watermark(): JSX.Element {
  return (
    <a
      href="https://cloudposse.com"
      target="_blank"
      rel="noopener noreferrer"
      className="cloudposse-logo"
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
