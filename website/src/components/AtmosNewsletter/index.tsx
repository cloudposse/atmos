import React, { useEffect, useRef, useState } from 'react';
import { useColorMode } from '@docusaurus/theme-common';
import styles from './styles.module.css';

/** Subscribe to the Atmos list using the shared Cloud Posse signup form. */
export default function AtmosNewsletter(): JSX.Element {
  const { colorMode } = useColorMode();
  const frame = useRef<HTMLIFrameElement>(null);
  const [height, setHeight] = useState<number>();
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
      )
        setHeight(Math.ceil(event.data.height));
    };
    window.addEventListener('message', resize);
    return () => window.removeEventListener('message', resize);
  }, []);
  const src = `https://cloudposse.com/embed/newsletter?list=atmos_newsletter&variant=compact&theme=${colorMode}&bg=transparent&utm_source=atmos.tools&utm_medium=embed&utm_campaign=atmos-newsletter`;

  return (
    <section
      aria-label="Subscribe to Atmos updates"
      className={styles.newsletter}
    >
      <h2>Keep up with Atmos</h2>
      <p className={styles.description}>
        A weekly digest of what’s new in Atmos.
      </p>
      <iframe
        ref={frame}
        style={height === undefined ? undefined : { height }}
        src={src}
        title="Subscribe to Atmos updates"
        className={styles.frame}
        loading="lazy"
        sandbox="allow-same-origin allow-scripts allow-forms"
      />
      <p className={styles.feeds}>
        Prefer a feed reader? <a href="/changelog/rss.xml">RSS</a>
        {' · '}
        <a href="/changelog/atom.xml">Atom</a>
      </p>
    </section>
  );
}
