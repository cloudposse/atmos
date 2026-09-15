import React from 'react';
import ProductUpdatesNav, { type ProductUpdatesNavProps } from '../ProductUpdatesNav';
import styles from './styles.module.css';

export default function ProductUpdatesHeader({ activeView }: ProductUpdatesNavProps): JSX.Element {
  return (
    <header className={styles.header}>
      <h1 className={styles.title}>Atmos Updates</h1>
      <p className={styles.description}>What's new and what's next for Atmos.</p>
      <ProductUpdatesNav activeView={activeView} />
    </header>
  );
}
