import React from 'react';
import styles from './styles.module.css';

interface RoadmapHeroProps {
  vision: string;
}

export default function RoadmapHero({ vision }: RoadmapHeroProps): JSX.Element {
  return <p className={styles.heroVision}>{vision}</p>;
}
