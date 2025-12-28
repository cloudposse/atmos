import React from 'react';
import { motion } from 'framer-motion';
import styles from './styles.module.css';

interface RoadmapHeroProps {
  vision: string;
}

export default function RoadmapHero({
  vision,
}: RoadmapHeroProps): JSX.Element {
  return (
    <motion.div
      className={styles.hero}
      initial={{ opacity: 0, y: 20 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.6, ease: 'easeOut' }}
    >
      <h1 className={styles.heroTitle}>Roadmap</h1>
      <p className={styles.heroVision}>{vision}</p>
    </motion.div>
  );
}
