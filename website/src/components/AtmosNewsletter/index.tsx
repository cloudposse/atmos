import React, { useEffect, useRef, useState } from 'react';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import { useColorMode } from '@docusaurus/theme-common';
import styles from './styles.module.css';

/** Subscribe to the Atmos list using the shared Cloud Posse signup form. */
export default function AtmosNewsletter(): JSX.Element {
  const { colorMode } = useColorMode();
  const { siteConfig } = useDocusaurusContext();
  const configuredOrigin = siteConfig.customFields?.newsletterEmbedOrigin;
  const embedOrigin =
    typeof configuredOrigin === 'string'
      ? configuredOrigin.replace(/\/$/, '')
      : 'https://cloudposse.com';
  const frame = useRef<HTMLIFrameElement>(null);
  const [height, setHeight] = useState<number>();
  const [readySrc, setReadySrc] = useState<string>();
  useEffect(() => {
    const resize = (event: MessageEvent) => {
      const element = frame.current;
      if (
        element &&
        event.source === element.contentWindow &&
        event.origin === new URL(element.src).origin &&
        event.data?.type === 'cloudposse:newsletter:resize' &&
        typeof event.data.height === 'number' &&
        Number.isFinite(event.data.height) &&
        event.data.height >= 40 &&
        event.data.height <= 600
      ) {
        setHeight(Math.ceil(event.data.height));
        setReadySrc(element.src);
      }
    };
    window.addEventListener('message', resize);
    return () => window.removeEventListener('message', resize);
  }, []);
  const src = `${embedOrigin}/embed/newsletter?list=atmos_newsletter&variant=compact&theme=${colorMode}&bg=transparent&utm_source=atmos.tools&utm_medium=embed&utm_campaign=atmos-newsletter`;

  const ready = readySrc === src;

  return (
    <section
      aria-label="Subscribe to Atmos updates"
      className={styles.newsletter}
    >
      <h2>Keep up with Atmos</h2>
      <p className={styles.description}>
        A weekly roundup of new features, improvements, and fixes.
      </p>
      <div className={styles.formContainer}>
        {!ready && (
          <div className={styles.loading} role="status">
            <span className={styles.spinner} aria-hidden="true" />
            <span>Loading signup form…</span>
          </div>
        )}
        <iframe
          ref={frame}
          style={{ height, visibility: ready ? 'visible' : 'hidden' }}
          aria-busy={!ready}
          src={src}
          title="Subscribe to Atmos updates"
          className={styles.frame}
          loading="lazy"
          sandbox="allow-same-origin allow-scripts allow-forms"
        />
      </div>
      <p className={styles.feeds}>
        Prefer a feed reader? <a href="/changelog/rss.xml">RSS</a>
        {' · '}
        <a href="/changelog/atom.xml">Atom</a>
      </p>
    </section>
  );
}
