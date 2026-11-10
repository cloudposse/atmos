import React from 'react';
import { useColorMode } from '@docusaurus/theme-common';
import styles from './styles.module.css';

/** Subscribe to the Atmos list using the shared Cloud Posse signup form. */
export default function AtmosNewsletter(): JSX.Element {
  const { colorMode } = useColorMode();
  const src = `https://cloudposse.com/embed/newsletter?list=atmos_newsletter&variant=compact&theme=${colorMode}&bg=transparent&utm_source=atmos.tools&utm_medium=embed&utm_campaign=atmos-newsletter`;

  return (
    <section
      aria-label="Subscribe to Atmos updates"
      className={styles.newsletter}
    >
      <h2>Keep up with Atmos</h2>
      <p className={styles.description}>
        Five released updates at a time, delivered weekly. No updates, no email.
      </p>
      <iframe
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
