import React from 'react';
import Link from '@docusaurus/Link';
import ProductUpdatesNav, { type ProductUpdatesNavProps } from '../ProductUpdatesNav';
import styles from './styles.module.css';

/** Render the shared heading and navigation so both update views retain the same layout. */
export default function ProductUpdatesHeader({ activeView }: ProductUpdatesNavProps): JSX.Element {
  return (
    <header className={styles.header}>
      <h1 className={styles.title}>Atmos Updates</h1>
      <p className={styles.description}>What's new and what's next for Atmos.</p>
      <nav className={styles.subscriptions} aria-label="Subscribe to Atmos updates">
        <Link to={activeView === 'changelog' ? '#subscribe' : '/changelog#subscribe'}>Subscribe</Link>
        <a href="/changelog/rss.xml">RSS</a>
        <a href="/changelog/atom.xml">Atom</a>
      </nav>
      <ProductUpdatesNav activeView={activeView} />
    </header>
  );
}
