/**
 * Utility functions for the ChangelogTimeline component.
 * Adapted from website/src/theme/BlogSidebar/Content/index.tsx
 */

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

export interface BlogPostItem {
  content: {
    metadata: BlogPostMetadata;
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
    case 'contributors':
      return 'tagContributors';
    default:
      return 'tagDefault';
  }
}
