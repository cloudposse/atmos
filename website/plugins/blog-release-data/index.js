/**
 * Plugin to extract release data from blog posts and make it available globally.
 * This allows components like the blog sidebar to group posts by release version.
 */
const fs = require('fs');
const path = require('path');
const matter = require('gray-matter');

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

        if (frontmatter.release) {
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

            releaseMap[slugPermalink] = frontmatter.release;
            releaseMap[`${slugPermalink}/`] = frontmatter.release;
            releaseMap[datePermalink] = frontmatter.release;
            releaseMap[`${datePermalink}/`] = frontmatter.release;
          } else if (frontmatter.slug) {
            // Non-date-prefixed file with explicit slug (e.g., welcome.md)
            const slugPermalink = `/changelog/${frontmatter.slug}`;
            releaseMap[slugPermalink] = frontmatter.release;
            releaseMap[`${slugPermalink}/`] = frontmatter.release;
          }
        }
      }

      return { releaseMap };
    },

    async contentLoaded({ content, actions }) {
      const { setGlobalData } = actions;
      setGlobalData(content);
    },
  };
};
