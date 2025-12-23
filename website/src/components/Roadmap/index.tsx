import React from 'react';
import { MotionConfig } from 'framer-motion';
import RoadmapHero from './RoadmapHero';
import QuarterTimeline from './QuarterTimeline';
import InitiativeCard from './InitiativeCard';
import { roadmapConfig } from '@site/src/data/roadmap';
import styles from './styles.module.css';

export default function Roadmap(): JSX.Element {
  // Sort initiatives by progress (highest first).
  const sortedInitiatives = [...roadmapConfig.initiatives].sort(
    (a, b) => b.progress - a.progress
  );

  return (
    <MotionConfig reducedMotion="user">
      <div className={styles.roadmap}>
        <RoadmapHero vision={roadmapConfig.vision} />

        <section className={styles.timelineSection}>
          <QuarterTimeline quarters={roadmapConfig.quarters} initiatives={roadmapConfig.initiatives} />
        </section>

        <section className={styles.section}>
          <h2 className={styles.sectionTitle}>Initiatives</h2>
          <p className={styles.sectionDescription}>
            Click on any initiative to see detailed milestones and progress.
          </p>
          <div className={styles.initiativesGrid}>
            {sortedInitiatives.map((initiative, index) => (
              <InitiativeCard
                key={initiative.id}
                initiative={initiative}
                index={index}
              />
            ))}
          </div>
        </section>
      </div>
    </MotionConfig>
  );
}
