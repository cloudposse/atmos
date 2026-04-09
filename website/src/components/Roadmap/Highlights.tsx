import React from 'react';
import { motion } from 'framer-motion';
import * as Icons from 'react-icons/ri';
import styles from './styles.module.css';

interface Highlight {
  id: string;
  label: string;
  before: string;
  after: string;
  icon: string;
  description: string;
}

interface HighlightsProps {
  highlights: Highlight[];
}

export default function Highlights({
  highlights,
}: HighlightsProps): JSX.Element {
  return (
    <div className={styles.highlights}>
      <h2 className={styles.highlightsTitle}>2025 Highlights</h2>
      <div className={styles.highlightsGrid}>
        {highlights.map((highlight, index) => {
          const IconComponent = (Icons as Record<string, React.ComponentType<{ className?: string }>>)[
            highlight.icon
          ] || Icons.RiStarLine;

          return (
            <motion.div
              key={highlight.id}
              className={styles.highlightCard}
              initial={{ opacity: 0, y: 20 }}
              whileInView={{ opacity: 1, y: 0 }}
              viewport={{ once: true }}
              transition={{ duration: 0.5, delay: index * 0.1 }}
            >
              <div className={styles.highlightIcon}>
                <IconComponent />
              </div>
              <h3 className={styles.highlightLabel}>{highlight.label}</h3>
              <div className={styles.highlightComparison}>
                <span className={styles.highlightBefore}>{highlight.before}</span>
                <span className={styles.highlightArrow}>→</span>
                <span className={styles.highlightAfter}>{highlight.after}</span>
              </div>
              <p className={styles.highlightDescription}>{highlight.description}</p>
            </motion.div>
          );
        })}
      </div>
    </div>
  );
}
