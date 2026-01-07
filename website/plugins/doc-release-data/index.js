/**
 * Plugin to extract release data from documentation files and make it available globally.
 * This allows components like the TOC to display "Unreleased" badges on docs with
 * changes not yet included in a release.
 *
 * Release detection:
 * 1. Gets the last-modified commit for each doc file
 * 2. Checks if that commit is contained in any stable release tag
 * 3. If not in any release, marks as 'unreleased'
 *
 * Key difference from blog-release-data: This plugin uses the LAST modified commit
 * (does this doc have unreleased changes?) rather than first-added commit
 * (when was this content introduced?).
 */
const { execSync } = require('child_process');
const fs = require('fs');
const path = require('path');

/**
 * Gets the commit SHA that last modified a file.
 * @param {string} filePath - Absolute path to the file.
 * @returns {string|null} - The commit SHA or null if not found.
 */
function getLastModifiedCommit(filePath) {
  try {
    const sha = execSync(`git log -1 --format="%H" -- "${filePath}"`, {
      encoding: 'utf8',
      stdio: ['pipe', 'pipe', 'pipe'],
    }).trim();
    return sha || null;
  } catch (e) {
    console.warn(
      `[doc-release-data] Failed to get last modified commit for ${filePath}: ${e.message}`
    );
    return null;
  }
}

/**
 * Finds the first stable release tag containing a given commit.
 * Stable releases match the pattern vX.Y.Z (no -rc, -test, -alpha, -beta, etc.).
 * @param {string} sha - The commit SHA to check.
 * @returns {string|null} - The release tag (e.g., "v1.202.0") or null if not found.
 */
function getFirstStableRelease(sha) {
  try {
    const tags = execSync(`git tag --contains ${sha} --sort=version:refname`, {
      encoding: 'utf8',
      stdio: ['pipe', 'pipe', 'pipe'],
    })
      .trim()
      .split('\n')
      .filter(Boolean);

    // Find the first stable release (matches vX.Y.Z exactly, no suffixes).
    const stableRelease = tags.find((t) => /^v\d+\.\d+\.\d+$/.test(t)) || null;
    return stableRelease;
  } catch (e) {
    console.warn(
      `[doc-release-data] Failed to find tags for commit ${sha}: ${e.message}`
    );
    return null;
  }
}

/**
 * Determines the release version for a documentation file.
 * @param {string} filePath - Absolute path to the doc file.
 * @returns {string} - The release version or 'unreleased'.
 */
function determineRelease(filePath) {
  const sha = getLastModifiedCommit(filePath);
  if (sha) {
    const release = getFirstStableRelease(sha);
    if (release) {
      return release;
    }
  }

  // Fall back to unreleased.
  return 'unreleased';
}

/**
 * Converts a doc file path to its URL path.
 * Handles Docusaurus doc routing conventions.
 *
 * @param {string} filePath - Absolute path to the doc file.
 * @param {string} docsDir - Absolute path to the docs directory.
 * @returns {string} - The URL path (e.g., "/cli/commands/describe").
 */
function getDocUrlPath(filePath, docsDir) {
  // Get relative path from docs dir.
  let relativePath = path.relative(docsDir, filePath);

  // Remove file extension (.mdx or .md).
  relativePath = relativePath.replace(/\.(mdx?|md)$/, '');

  // Handle index files - they become the directory path.
  if (relativePath.endsWith('/index') || relativePath === 'index') {
    relativePath = relativePath.replace(/\/?index$/, '');
  }

  // Ensure leading slash.
  const urlPath = '/' + relativePath;

  return urlPath;
}

/**
 * Recursively finds all .mdx and .md files in a directory.
 * @param {string} dir - Directory to search.
 * @returns {string[]} - Array of absolute file paths.
 */
function findDocFiles(dir) {
  const files = [];

  function walk(currentDir) {
    const entries = fs.readdirSync(currentDir, { withFileTypes: true });
    for (const entry of entries) {
      const fullPath = path.join(currentDir, entry.name);
      if (entry.isDirectory()) {
        walk(fullPath);
      } else if (entry.isFile() && /\.(mdx?|md)$/.test(entry.name)) {
        files.push(fullPath);
      }
    }
  }

  walk(dir);
  return files;
}

module.exports = function docReleaseDataPlugin(context, options) {
  return {
    name: 'doc-release-data',

    async loadContent() {
      const docsDir = path.join(context.siteDir, 'docs');
      const releaseMap = {};

      // Check if docs directory exists.
      if (!fs.existsSync(docsDir)) {
        console.warn('[doc-release-data] docs directory not found');
        return { releaseMap };
      }

      // Find all doc files recursively.
      const files = findDocFiles(docsDir);

      for (const filePath of files) {
        const release = determineRelease(filePath);
        const urlPath = getDocUrlPath(filePath, docsDir);

        // Store both with and without trailing slash for lookup flexibility.
        releaseMap[urlPath] = release;
        if (urlPath !== '/') {
          releaseMap[`${urlPath}/`] = release;
        }
      }

      // Log summary for debugging.
      const entries = Object.entries(releaseMap);
      // Count unique paths (without trailing slash duplicates).
      const uniquePaths = new Set(
        entries.map(([k]) => k.replace(/\/$/, '') || '/')
      );
      const released = [...uniquePaths].filter(
        (p) => releaseMap[p] !== 'unreleased'
      ).length;
      const unreleased = uniquePaths.size - released;
      console.log(
        `[doc-release-data] Processed ${files.length} doc files: ${released} released, ${unreleased} unreleased`
      );

      return { releaseMap };
    },

    async contentLoaded({ content, actions }) {
      const { setGlobalData } = actions;
      setGlobalData(content);
    },
  };
};
