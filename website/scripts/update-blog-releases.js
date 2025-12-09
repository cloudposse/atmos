#!/usr/bin/env node
/**
 * Script to compute and add release versions to blog post frontmatter.
 *
 * For each blog post, this script:
 * 1. Gets the commit SHA that introduced the file
 * 2. Finds the earliest stable release tag containing that commit
 * 3. Adds `release: vX.Y.Z` to the frontmatter
 *
 * Usage: node website/scripts/update-blog-releases.js
 */

const { execSync } = require('child_process');
const fs = require('fs');
const path = require('path');
const matter = require('gray-matter');

const blogDir = path.join(__dirname, '..', 'blog');

// Check if blog directory exists
if (!fs.existsSync(blogDir)) {
  console.error(`Blog directory not found: ${blogDir}`);
  process.exit(1);
}

const files = fs.readdirSync(blogDir).filter(f => f.match(/\.(md|mdx)$/));

console.log(`Found ${files.length} blog posts to process...\n`);

let updated = 0;
let skipped = 0;
let errors = 0;

for (const file of files) {
  const filePath = path.join(blogDir, file);

  try {
    const content = fs.readFileSync(filePath, 'utf8');
    const { data: frontmatter, content: body } = matter(content);

    // Skip if release already set
    if (frontmatter.release) {
      console.log(`✓ ${file}: Already has release: ${frontmatter.release}`);
      skipped++;
      continue;
    }

    // Get commit SHA that introduced this file (the first commit that added it)
    let sha;
    try {
      sha = execSync(`git log --follow --diff-filter=A --format="%H" -- "${filePath}"`, {
        encoding: 'utf8',
        stdio: ['pipe', 'pipe', 'pipe']
      }).trim().split('\n')[0];
    } catch (e) {
      sha = null;
    }

    if (!sha) {
      console.log(`⚠ ${file}: No commit found, setting to "unreleased"`);
      frontmatter.release = 'unreleased';
    } else {
      // Find all tags containing this commit, sorted by version
      let tags;
      try {
        tags = execSync(`git tag --contains ${sha} --sort=version:refname`, {
          encoding: 'utf8',
          stdio: ['pipe', 'pipe', 'pipe']
        }).trim().split('\n').filter(Boolean);
      } catch (e) {
        tags = [];
      }

      // Find the first stable release (matches vX.Y.Z exactly, no -rc, -test, etc.)
      const stableRelease = tags.find(t => /^v\d+\.\d+\.\d+$/.test(t));

      if (stableRelease) {
        console.log(`✓ ${file}: ${stableRelease}`);
        frontmatter.release = stableRelease;
      } else {
        console.log(`⚠ ${file}: No stable release contains commit ${sha.slice(0, 7)}, setting to "unreleased"`);
        frontmatter.release = 'unreleased';
      }
    }

    // Write back with updated frontmatter
    const updated_content = matter.stringify(body, frontmatter);
    fs.writeFileSync(filePath, updated_content);
    updated++;

  } catch (err) {
    console.error(`✗ ${file}: Error - ${err.message}`);
    errors++;
  }
}

console.log(`\n--- Summary ---`);
console.log(`Updated: ${updated}`);
console.log(`Skipped (already had release): ${skipped}`);
console.log(`Errors: ${errors}`);
