import React, { useState } from 'react';
import { MotionConfig } from 'framer-motion';
import Link from '@docusaurus/Link';
import { RiLightbulbLine, RiExternalLinkLine, RiExpandDiagonalLine } from 'react-icons/ri';
import RoadmapHero from './RoadmapHero';
import QuarterTimeline from './QuarterTimeline';
import FeaturedSection from './FeaturedSection';
import RoadmapStats from './RoadmapStats';
import InitiativeCard from './InitiativeCard';
import { roadmapConfig } from '@site/src/data/roadmap';
import styles from './styles.module.css';

// Priority order for initiatives (lower = higher priority).
const initiativePriority: Record<string, number> = {
  quality: 1,
  docs: 2,
  dx: 3,
  'ci-cd': 4,
};

export default function Roadmap(): JSX.Element {
  const [expandAllMilestones, setExpandAllMilestones] = useState(false);

  // Sort initiatives by priority (explicit priorities first, then by progress).
  const sortedInitiatives = [...roadmapConfig.initiatives].sort((a, b) => {
    const priorityA = initiativePriority[a.id] ?? 100;
    const priorityB = initiativePriority[b.id] ?? 100;
    if (priorityA !== priorityB) return priorityA - priorityB;
    // Fall back to progress for non-prioritized initiatives.
    return b.progress - a.progress;
  });

  return (
    <MotionConfig reducedMotion="user">
      <div className={styles.roadmap}>
        <RoadmapHero vision={roadmapConfig.vision} />

        <section className={styles.timelineSection}>
          <QuarterTimeline quarters={roadmapConfig.quarters} initiatives={roadmapConfig.initiatives} />
        </section>

        {roadmapConfig.featured && roadmapConfig.featured.length > 0 && (
          <FeaturedSection items={roadmapConfig.featured} />
        )}

        <RoadmapStats initiatives={roadmapConfig.initiatives} />

        <section className={styles.section}>
          <h2 className={styles.sectionTitle}>Major Initiatives</h2>
          <p className={styles.sectionDescription}>
            Click on any initiative to see detailed milestones and progress.
            {' '}
            <Link
              to="https://github.com/cloudposse/atmos/issues"
              className={styles.viewFeaturesLink}
            >
              <RiExternalLinkLine />
              <span>View Issues</span>
            </Link>
            <Link
              to="https://github.com/cloudposse/atmos/issues/new?template=feature_request.yml"
              className={styles.featureRequestLink}
            >
              <RiLightbulbLine />
              <span>Request a Feature</span>
            </Link>
            <button
              type="button"
              className={styles.expandAllButton}
              onClick={() => setExpandAllMilestones(!expandAllMilestones)}
              title={expandAllMilestones ? 'Collapse milestone sections' : 'Expand all milestone sections'}
            >
              <RiExpandDiagonalLine />
              <span>{expandAllMilestones ? 'Collapse All' : 'Expand All'}</span>
            </button>
          </p>
          <div className={styles.initiativesGrid}>
            {sortedInitiatives.map((initiative, index) => (
              <InitiativeCard
                key={initiative.id}
                initiative={initiative}
                index={index}
                expandAllMilestones={expandAllMilestones}
              />
            ))}
          </div>
        </section>
      </div>
    </MotionConfig>
  );
}
