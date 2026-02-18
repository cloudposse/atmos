/**
 * Utility functions for the ChangelogTimeline component.
 * Adapted from website/src/theme/BlogSidebar/Content/index.tsx
 */

export interface BlogPostFrontMatter {
  release?: string;
  [key: string]: unknown;
}

export interface BlogPostMetadata {
  permalink: string;
  title: string;
  date: string;
  description?: string;
  tags?: Array<{
    label: string;
    permalink: string;
  }>;
  authors?: Array<{
    name: string;
    imageURL?: string;
  }>;
}

// Docusaurus BlogPostContent structure: content has both metadata and frontMatter as siblings.
export interface BlogPostItem {
  content: {
    metadata: BlogPostMetadata;
    frontMatter: BlogPostFrontMatter;
  };
}

export interface MonthGroup {
  month: string;
  monthNum: number;
  items: BlogPostItem[];
}

export interface YearGroup {
  year: string;
  months: MonthGroup[];
}

export interface ReleaseGroup {
  release: string;
  items: BlogPostItem[];
}

/**
 * Parses a semver version string (e.g., "v1.200.0") into comparable parts.
 * Only matches exact vX.Y.Z or X.Y.Z formats. Prerelease suffixes (e.g.,
 * "v1.2.3-rc1") and other non-standard formats return null.
 */
function parseVersion(version: string): [number, number, number] | null {
  const match = version.match(/^v?(\d+)\.(\d+)\.(\d+)$/);
  if (!match) return null;
  return [parseInt(match[1], 10), parseInt(match[2], 10), parseInt(match[3], 10)];
}

/**
 * Compares two version strings for sorting (descending order).
 * "unreleased" comes first, then versions in descending semver order.
 */
export function compareVersionsDescending(a: string, b: string): number {
  // Unreleased always comes first.
  if (a === 'unreleased' && b === 'unreleased') return 0;
  if (a === 'unreleased') return -1;
  if (b === 'unreleased') return 1;

  const versionA = parseVersion(a);
  const versionB = parseVersion(b);

  // If either version is invalid, fall back to string comparison.
  if (!versionA && !versionB) return b.localeCompare(a);
  if (!versionA) return 1;
  if (!versionB) return -1;

  // Compare major, minor, patch in descending order.
  for (let i = 0; i < 3; i++) {
    if (versionB[i] !== versionA[i]) {
      return versionB[i] - versionA[i];
    }
  }
  return 0;
}

/**
 * Groups blog posts by release version, sorted with unreleased first then descending.
 * @param items - Blog post items to group.
 * @param globalReleaseMap - Optional release map from blog-release-data plugin (permalink → release).
 */
export function groupBlogPostsByRelease(
  items: BlogPostItem[],
  globalReleaseMap?: Record<string, string>
): ReleaseGroup[] {
  const releaseGroups = new Map<string, BlogPostItem[]>();

  items.forEach((item) => {
    // Priority: frontMatter > globalReleaseMap > 'unreleased'.
    let release = item.content.frontMatter?.release;
    if (!release && globalReleaseMap) {
      const permalink = item.content.metadata.permalink;
      release = globalReleaseMap[permalink] || globalReleaseMap[`${permalink}/`];
    }
    release = release || 'unreleased';

    if (!releaseGroups.has(release)) {
      releaseGroups.set(release, []);
    }
    releaseGroups.get(release)!.push(item);
  });

  // Sort releases: unreleased first, then by version descending.
  const sortedReleases = Array.from(releaseGroups.keys()).sort(compareVersionsDescending);

  return sortedReleases.map((release) => ({
    release,
    items: releaseGroups.get(release)!,
  }));
}

/**
 * Groups blog posts by year and month, sorted descending.
 */
export function groupBlogPostsByYearMonth(items: BlogPostItem[]): YearGroup[] {
  interface MonthData {
    items: BlogPostItem[];
    monthNum: number;
  }

  const yearMonthMap = new Map<string, Map<string, MonthData>>();

  items.forEach((item) => {
    const date = new Date(item.content.metadata.date);

    // Validate date.
    if (isNaN(date.getTime())) {
      console.warn(`Invalid date for blog item: ${item.content.metadata.date}`);
      return;
    }

    const year = `${date.getFullYear()}`;
    const month = date.toLocaleString('default', { month: 'long' });
    const monthNum = date.getMonth();

    if (!yearMonthMap.has(year)) {
      yearMonthMap.set(year, new Map());
    }

    const monthMap = yearMonthMap.get(year)!;
    if (!monthMap.has(month)) {
      monthMap.set(month, { items: [], monthNum });
    }

    monthMap.get(month)!.items.push(item);
  });

  // Sort years descending.
  const sortedYears = Array.from(yearMonthMap.keys()).sort(
    (a, b) => parseInt(b) - parseInt(a)
  );

  return sortedYears.map((year) => {
    const monthMap = yearMonthMap.get(year)!;
    // Sort months descending within each year.
    const sortedMonths = Array.from(monthMap.entries()).sort(
      (a, b) => b[1].monthNum - a[1].monthNum
    );

    return {
      year,
      months: sortedMonths.map(([month, monthData]) => ({
        month,
        monthNum: monthData.monthNum,
        items: monthData.items,
      })),
    };
  });
}

/**
 * Extracts unique years from blog posts.
 */
export function extractYears(items: BlogPostItem[]): string[] {
  const years = new Set<string>();
  items.forEach((item) => {
    const date = new Date(item.content.metadata.date);
    if (!isNaN(date.getTime())) {
      years.add(`${date.getFullYear()}`);
    }
  });
  return Array.from(years).sort((a, b) => parseInt(b) - parseInt(a));
}

/**
 * Extracts unique tags from blog posts.
 */
export function extractTags(items: BlogPostItem[]): string[] {
  const tags = new Set<string>();
  items.forEach((item) => {
    item.content.metadata.tags?.forEach((tag) => {
      tags.add(tag.label);
    });
  });
  return Array.from(tags).sort();
}

/**
 * Filters blog posts by year and/or tag.
 */
export function filterBlogPosts(
  items: BlogPostItem[],
  selectedYear: string | null,
  selectedTag: string | null
): BlogPostItem[] {
  return items.filter((item) => {
    const metadata = item.content.metadata;

    // Filter by year.
    if (selectedYear) {
      const date = new Date(metadata.date);
      if (isNaN(date.getTime())) return false;
      if (`${date.getFullYear()}` !== selectedYear) return false;
    }

    // Filter by tag.
    if (selectedTag) {
      const hasTag = metadata.tags?.some((tag) => tag.label === selectedTag);
      if (!hasTag) return false;
    }

    return true;
  });
}

/**
 * Filters blog posts by tag only.
 */
export function filterBlogPostsByTag(
  items: BlogPostItem[],
  selectedTag: string | null
): BlogPostItem[] {
  if (!selectedTag) return items;

  return items.filter((item) => {
    const metadata = item.content.metadata;
    return metadata.tags?.some((tag) => tag.label === selectedTag);
  });
}

/**
 * Filters blog posts by multiple years and/or tags.
 * Years are OR'd (post matches if in any selected year).
 * Tags are OR'd (post matches if has any selected tag).
 * Years and tags are AND'd together.
 */
export function filterBlogPostsMulti(
  items: BlogPostItem[],
  selectedYears: string[],
  selectedTags: string[]
): BlogPostItem[] {
  return items.filter((item) => {
    const metadata = item.content.metadata;

    // Filter by years (OR logic - match any selected year).
    if (selectedYears.length > 0) {
      const date = new Date(metadata.date);
      if (isNaN(date.getTime())) return false;
      const itemYear = `${date.getFullYear()}`;
      if (!selectedYears.includes(itemYear)) return false;
    }

    // Filter by tags (OR logic - match any selected tag).
    if (selectedTags.length > 0) {
      const hasMatchingTag = metadata.tags?.some((tag) =>
        selectedTags.includes(tag.label)
      );
      if (!hasMatchingTag) return false;
    }

    return true;
  });
}

/**
 * Returns the CSS class for a tag based on its type.
 */
export function getTagColorClass(tagLabel: string): string {
  const label = tagLabel.toLowerCase();
  switch (label) {
    case 'feature':
      return 'tagFeature';
    case 'enhancement':
      return 'tagEnhancement';
    case 'bugfix':
    case 'bug fix':
      return 'tagBugfix';
    case 'dx':
      return 'tagDx';
    case 'breaking-change':
      return 'tagBreakingChange';
    case 'security':
      return 'tagSecurity';
    case 'documentation':
      return 'tagDocumentation';
    case 'core':
      return 'tagCore';
    case 'deprecation':
      return 'tagDeprecation';
    case 'experimental':
      return 'tagExperimental';
    default:
      return 'tagDefault';
  }
}
