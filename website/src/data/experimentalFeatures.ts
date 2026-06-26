import roadmapConfig from '@site/src/data/roadmap';

/**
 * A single experimental feature extracted from the roadmap, plus the initiative
 * it belongs to. This is the shared shape consumed by both the experimental
 * features list page and the sidebar badge wiring.
 */
export interface ExperimentalFeature {
  label: string;
  docs?: string;
  changelog?: string;
  prd?: string;
  description?: string;
}

export interface FeatureGroup {
  id: string;
  title: string;
  features: ExperimentalFeature[];
}

interface FeaturedItem {
  experimental?: boolean;
  title: string;
  docs?: string;
  changelog?: string;
  prd?: string;
  description?: string;
}

interface Milestone {
  label: string;
  experimental?: boolean;
  docs?: string;
  changelog?: string;
  prd?: string;
  description?: string;
}

interface Initiative {
  id: string;
  title: string;
  milestones?: Milestone[];
}

/**
 * Walks the roadmap data and collects all features flagged `experimental: true`,
 * grouped by initiative (with a leading "Featured" group). Labels are de-duplicated
 * so a feature that appears in both `featured` and an initiative is only listed once.
 *
 * This is the single source of truth for "what is experimental" on the website, so
 * the experimental features page and the sidebar badge cannot drift apart.
 */
export function getGroupedExperimentalFeatures(): FeatureGroup[] {
  const groups: FeatureGroup[] = [];
  const seenLabels = new Set<string>();

  // First, collect featured experimental items into a "Featured" group.
  if (roadmapConfig.featured) {
    const featuredFeatures: ExperimentalFeature[] = [];
    (roadmapConfig.featured as FeaturedItem[]).forEach((item) => {
      if (item.experimental) {
        featuredFeatures.push({
          label: item.title,
          docs: item.docs,
          changelog: item.changelog,
          prd: item.prd,
          description: item.description,
        });
        seenLabels.add(item.title);
      }
    });
    if (featuredFeatures.length > 0) {
      groups.push({
        id: 'featured',
        title: 'Featured',
        features: featuredFeatures,
      });
    }
  }

  // Then, group initiative milestones by their parent initiative.
  if (roadmapConfig.initiatives) {
    (roadmapConfig.initiatives as Initiative[]).forEach((initiative) => {
      if (initiative.milestones) {
        const initiativeFeatures: ExperimentalFeature[] = [];
        initiative.milestones.forEach((milestone) => {
          if (milestone.experimental && !seenLabels.has(milestone.label)) {
            initiativeFeatures.push({
              label: milestone.label,
              docs: milestone.docs,
              changelog: milestone.changelog,
              prd: milestone.prd,
              description: milestone.description,
            });
            seenLabels.add(milestone.label);
          }
        });
        if (initiativeFeatures.length > 0) {
          groups.push({
            id: initiative.id,
            title: initiative.title,
            features: initiativeFeatures,
          });
        }
      }
    });
  }

  return groups;
}
