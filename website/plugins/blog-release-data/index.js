/**
 * Plugin to extract release data from blog posts and make it available globally.
 * This allows components like the blog sidebar to group posts by release version.
 *
 * Release detection order:
 * 1. Frontmatter `release` field (highest precedence - for manual overrides)
 * 2. Git-based detection: finds the first stable release tag containing the commit
 *    that introduced the blog post file
 * 3. Falls back to 'unreleased' if neither method succeeds
 */
const { execSync } = require('child_process');
const fs = require('fs');
const path = require('path');
const matter = require('gray-matter');

/**
 * Gets the commit SHA that introduced a file (the first commit that added it).
 * @param {string} filePath - Absolute path to the file.
 * @returns {string|null} - The commit SHA or null if not found.
 */
function getFileOriginCommit(filePath) {
  try {
    const sha = execSync(
      `git log --follow --diff-filter=A --format="%H" -- "${filePath}"`,
      {
        encoding: 'utf8',
        stdio: ['pipe', 'pipe', 'pipe'],
      }
    ).trim().split('\n')[0];
    return sha || null;
  } catch (e) {
    console.warn(`[blog-release-data] Failed to get origin commit for ${filePath}: ${e.message}`);
    return null;
  }
}

/**
 * Finds the first stable release tag containing a given commit.
 * Stable releases match the pattern vX.Y.Z (no -rc, -test, etc.).
 * @param {string} sha - The commit SHA to check.
 * @returns {string|null} - The release tag (e.g., "v1.202.0") or null if not found.
 */
function getFirstStableRelease(sha) {
  try {
    const tags = execSync(
      `git tag --contains ${sha} --sort=version:refname`,
      {
        encoding: 'utf8',
        stdio: ['pipe', 'pipe', 'pipe'],
      }
    ).trim().split('\n').filter(Boolean);

    // Find the first stable release (matches vX.Y.Z exactly, no suffixes).
    const stableRelease = tags.find((t) => /^v\d+\.\d+\.\d+$/.test(t)) || null;
    return stableRelease;
  } catch (e) {
    console.warn(`[blog-release-data] Failed to find tags for commit ${sha}: ${e.message}`);
    return null;
  }
}

/**
 * Determines the release version for a blog post.
 * @param {string} filePath - Absolute path to the blog post file.
 * @param {object} frontmatter - Parsed frontmatter from the blog post.
 * @returns {string} - The release version or 'unreleased'.
 */
function determineRelease(filePath, frontmatter) {
  // 1. Frontmatter takes precedence (allows manual overrides).
  if (frontmatter.release) {
    return frontmatter.release;
  }

  // 2. Try git-based detection.
  const sha = getFileOriginCommit(filePath);
  if (sha) {
    const release = getFirstStableRelease(sha);
    if (release) {
      return release;
    }
  }

  // 3. Fall back to unreleased.
  return 'unreleased';
}

/**
 * Adds release to the releaseMap for all permalink variations.
 * @param {object} releaseMap - The map to populate.
 * @param {string} file - The filename.
 * @param {object} frontmatter - Parsed frontmatter.
 * @param {string} release - The determined release version.
 */
function addToReleaseMap(releaseMap, file, frontmatter, release) {
  // The blog is configured with routeBasePath: 'changelog'.
  // Handle multiple filename formats and permalink styles.

  // Try date-prefixed filename: YYYY-MM-DD-slug-name.mdx
  const dateMatch = file.match(/^(\d{4})-(\d{2})-(\d{2})-(.+)\.(mdx?|md)$/);
  if (dateMatch) {
    const [, year, month, day, slugPart] = dateMatch;
    const slug = frontmatter.slug || slugPart;

    // Store multiple permalink formats to handle both slug-only and date-based paths.
    const slugPermalink = `/changelog/${slug}`;
    const datePermalink = `/changelog/${year}/${month}/${day}/${slugPart}`;

    releaseMap[slugPermalink] = release;
    releaseMap[`${slugPermalink}/`] = release;
    releaseMap[datePermalink] = release;
    releaseMap[`${datePermalink}/`] = release;
  } else if (frontmatter.slug) {
    // Non-date-prefixed file with explicit slug (e.g., welcome.md).
    const slugPermalink = `/changelog/${frontmatter.slug}`;
    releaseMap[slugPermalink] = release;
    releaseMap[`${slugPermalink}/`] = release;
  }
}

module.exports = function blogReleaseDataPlugin(context, options) {
  return {
    name: 'blog-release-data',

    async loadContent() {
      const blogDir = path.join(context.siteDir, 'blog');
      const releaseMap = {};

      // Read all .mdx and .md files in the blog directory.
      const files = fs.readdirSync(blogDir).filter(
        (file) => file.endsWith('.mdx') || file.endsWith('.md')
      );

      for (const file of files) {
        const filePath = path.join(blogDir, file);
        const content = fs.readFileSync(filePath, 'utf-8');
        const { data: frontmatter } = matter(content);

        const release = determineRelease(filePath, frontmatter);
        addToReleaseMap(releaseMap, file, frontmatter, release);
      }

      // Log summary for debugging.
      const entries = Object.entries(releaseMap);
      const released = entries.filter(([, v]) => v !== 'unreleased').length;
      const unreleased = entries.filter(([, v]) => v === 'unreleased').length;
      console.log(`[blog-release-data] Processed ${files.length} blog posts: ${released} released, ${unreleased} unreleased`);

      return { releaseMap };
    },

    async contentLoaded({ content, actions }) {
      const { setGlobalData } = actions;
      setGlobalData(content);
    },
  };
};
