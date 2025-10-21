const fs = require('fs');
const path = require('path');
const matter = require('gray-matter');

/**
 * Glossary Tooltips Plugin for Docusaurus.
 * Provides hover tooltips for glossary terms throughout documentation.
 *
 * This is a custom implementation inspired by @grnet/docusaurus-terminology
 * (https://github.com/grnet/docusaurus-terminology) but written from scratch
 * to avoid security vulnerabilities in the original package's dependencies.
 *
 * Original plugin: BSD-2-Clause License, Copyright (c) National Infrastructures
 * for Research and Technology (GRNET).
 */
module.exports = function (context, options) {
  const {
    termsDir = './docs/glossary/',
    docsDir = './docs/',
    glossaryFilepath = './docs/glossary/index.mdx',
  } = options;

  return {
    name: 'glossary-tooltips-plugin',

    async loadContent() {
      // Scan glossary directory for term files.
      const termsPath = path.resolve(context.siteDir, termsDir);
      const glossaryData = {};

      if (!fs.existsSync(termsPath)) {
        console.warn(`[native-glossary] Terms directory not found: ${termsPath}`);
        return glossaryData;
      }

      const files = fs.readdirSync(termsPath);

      for (const file of files) {
        if (file === 'index.mdx' || file === 'index.md') {
          continue; // Skip the glossary index page.
        }

        if (file.endsWith('.md') || file.endsWith('.mdx')) {
          const filePath = path.join(termsPath, file);
          const content = fs.readFileSync(filePath, 'utf-8');
          const { data, content: mdContent } = matter(content);

          // Validate required frontmatter fields.
          const filename = path.basename(file, path.extname(file));
          const termId = data.id || filename;
          const termTitle = data.title;

          // Skip files without required title field.
          if (!termTitle) {
            console.warn(
              `[glossary-tooltips] Skipping ${file}: missing required 'title' field in frontmatter`
            );
            continue;
          }

          // Use provided slug or generate from ID.
          const slug = data.slug || `/terms/${termId}`;

          const entry = {
            metadata: {
              id: termId,
              title: termTitle,
              hoverText: data.hoverText || '',
              slug: slug,
              disambiguation: data.disambiguation || {},
            },
            content: mdContent.trim(),
          };

          // Register entry under both slug and id for flexible lookups.
          glossaryData[slug] = entry;
          glossaryData[termId] = entry;
        }
      }

      return glossaryData;
    },

    async contentLoaded({ content, actions }) {
      const { createData } = actions;

      // Generate glossary.json for build output.
      await createData('glossary.json', JSON.stringify(content, null, 2));

      // Also write to static directory for runtime access.
      const staticPath = path.resolve(context.siteDir, 'static');
      if (!fs.existsSync(staticPath)) {
        fs.mkdirSync(staticPath, { recursive: true });
      }
      try {
        fs.writeFileSync(
          path.join(staticPath, 'glossary.json'),
          JSON.stringify(content, null, 2)
        );
      } catch (err) {
        console.error('[glossary-tooltips] Failed to write glossary.json:', err);
        throw err;
      }
    },

    configureWebpack(config, isServer, utils) {
      // Find the MDX rule.
      let rule = config.module.rules.find((rule) => {
        return rule.test && rule.test.toString().includes('mdx');
      });

      if (!rule) {
        const testMdFilename = 'test.md';
        const testMdxFilename = 'test.mdx';
        rule = config.module.rules.find((rule) => {
          if (!rule.test) return false;
          const ruleRegExp = new RegExp(rule.test);
          return ruleRegExp.test(testMdFilename) && ruleRegExp.test(testMdxFilename);
        });
      }

      if (rule && rule.use) {
        // Add our custom loader to transform term links.
        rule.use.push({
          loader: path.resolve(__dirname, 'webpack-loader.js'),
          options: {
            termsDir: termsDir.replace(/^\.\//, ''),
            baseUrl: context.baseUrl,
          },
        });
      }

      return {
        mergeStrategy: { 'module.rules': 'prepend' },
      };
    },

    getPathsToWatch() {
      return [path.resolve(context.siteDir, termsDir)];
    },
  };
};
