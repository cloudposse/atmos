import React from 'react';
import useBrokenLinks from '@docusaurus/useBrokenLinks';
import { useColorMode } from '@docusaurus/theme-common';
import styles from './styles.module.css';

/** Subscribe to the Atmos list using the shared Cloud Posse signup form. */
export default function AtmosNewsletter(): JSX.Element {
  useBrokenLinks().collectAnchor('subscribe');
  const { colorMode } = useColorMode();
  const src = `https://cloudposse.com/embed/newsletter?list=atmos_newsletter&theme=${colorMode}&bg=transparent&utm_source=atmos.tools&utm_medium=embed&utm_campaign=atmos-newsletter`;

  return (
    <section id="subscribe" aria-label="Subscribe to Atmos updates" className={styles.newsletter}>
      <iframe
        src={src}
        title="Subscribe to Atmos updates"
        className={styles.frame}
        loading="lazy"
        sandbox="allow-same-origin allow-scripts allow-forms"
      />
      <p className={styles.feeds}>
        Prefer a feed reader? <a href="/changelog/rss.xml">RSS</a>
        {' · '}<a href="/changelog/atom.xml">Atom</a>
      </p>
    </section>
  );
}
