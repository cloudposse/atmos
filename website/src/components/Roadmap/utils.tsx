import React from 'react';
import styles from './styles.module.css';

/**
 * Renders basic inline markdown (backtick code spans).
 * Converts `code` to <code>code</code>.
 */
export function renderInlineMarkdown(text: string): React.ReactNode {
  const parts = text.split(/(`[^`]+`)/g);
  return parts.map((part, i) => {
    if (part.startsWith('`') && part.endsWith('`')) {
      return <code key={i} className={styles.inlineCode}>{part.slice(1, -1)}</code>;
    }
    return part;
  });
}
