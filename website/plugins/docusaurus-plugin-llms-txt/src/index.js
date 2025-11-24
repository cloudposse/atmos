/**
 * Custom Docusaurus plugin to generate llms.txt and llms-full.txt files.
 *
 * This plugin uses Docusaurus's resolved routes directly instead of reconstructing
 * URLs from file paths, ensuring correct URLs that respect:
 * - frontmatter slug overrides
 * - routeBasePath configurations
 * - numeric/date prefix handling
 *
 * Replaces docusaurus-plugin-llms v0.2.2 which has URL reconstruction bugs.
 */

const fs = require('fs').promises;
const path = require('path');
const matter = require('gray-matter');

/**
 * Extract title from frontmatter or markdown content.
 */
function extractTitle(frontmatter, content, filePath) {
  if (frontmatter.title) {
    return frontmatter.title;
  }

  if (frontmatter.sidebar_label) {
    return frontmatter.sidebar_label;
  }

  const headingMatch = content.match(/^#\s+(.+)$/m);
  if (headingMatch) {
    return headingMatch[1].trim();
  }

  return path.basename(filePath, path.extname(filePath));
}

/**
 * Extract description from frontmatter or content.
 */
function extractDescription(frontmatter, content) {
  if (frontmatter.description) {
    return frontmatter.description;
  }

  const paragraphs = content.split('\n\n');
  for (const para of paragraphs) {
    const trimmed = para.trim();
    if (trimmed && !trimmed.startsWith('#') && !trimmed.startsWith('import ')) {
      return trimmed.substring(0, 200);
    }
  }

  return '[Description not available]';
}

/**
 * Clean markdown content for llms-full.txt.
 */
function cleanMarkdownContent(content) {
  return content
    // Remove import statements
    .replace(/^import\s+.+$/gm, '')
    // Remove JSX/MDX component tags
    .replace(/<[A-Z][^>]*>/g, '')
    .replace(/<\/[A-Z][^>]*>/g, '')
    // Clean up extra blank lines
    .replace(/\n{3,}/g, '\n\n')
    .trim();
}

/**
 * Read and process a markdown file.
 */
async function processMarkdownFile(filePath, url, siteUrl) {
  try {
    const fileContent = await fs.readFile(filePath, 'utf-8');
    const { data: frontmatter, content } = matter(fileContent);

    // Skip draft files
    if (frontmatter.draft === true) {
      return null;
    }

    const title = extractTitle(frontmatter, content, filePath);
    const description = extractDescription(frontmatter, content);
    const fullUrl = new URL(url, siteUrl).toString();
    const cleanedContent = cleanMarkdownContent(content);

    return {
      title,
      description,
      url: fullUrl,
      content: cleanedContent,
    };
  } catch (error) {
    console.warn(`Error processing ${filePath}: ${error.message}`);
    return null;
  }
}

/**
 * Recursively extract all markdown file routes from Docusaurus route config.
 * Uses routesPaths array to get all resolved URLs.
 */
function extractContentRoutes(routesPaths, siteDir) {
  const contentRoutes = [];

  // Common doc file locations
  const searchPaths = [
    'docs',
    'blog',
  ];

  for (const routePath of routesPaths) {
    // Skip non-content routes
    if (routePath.includes('/tags/') ||
        routePath.includes('/page/') ||
        routePath === '/search' ||
        routePath === '404.html') {
      continue;
    }

    // Try to find the source file for this route
    for (const searchPath of searchPaths) {
      const possiblePaths = [];

      // For blog posts with date prefixes
      if (searchPath === 'blog') {
        // Try with .mdx and .md extensions
        const slug = routePath.replace('/changelog/', '');
        possiblePaths.push(`${searchPath}/${slug}.mdx`);
        possiblePaths.push(`${searchPath}/${slug}.md`);

        // Try with date prefixes (common blog pattern)
        const blogFiles = require('fs').readdirSync(path.join(siteDir, searchPath))
          .filter(f => f.endsWith('.mdx') || f.endsWith('.md'));

        for (const blogFile of blogFiles) {
          const fileSlug = blogFile.replace(/^\d{4}-\d{2}-\d{2}-/, '').replace(/\.mdx?$/, '');
          if (fileSlug === slug) {
            possiblePaths.push(`${searchPath}/${blogFile}`);
          }
        }
      } else {
        // For docs
        const slug = routePath.replace(/^\//, '');
        possiblePaths.push(`${searchPath}/${slug}.mdx`);
        possiblePaths.push(`${searchPath}/${slug}.md`);
        possiblePaths.push(`${searchPath}/${slug}/index.mdx`);
        possiblePaths.push(`${searchPath}/${slug}/index.md`);
      }

      // Check which file exists
      for (const possiblePath of possiblePaths) {
        const fullPath = path.join(siteDir, possiblePath);
        try {
          require('fs').accessSync(fullPath);
          contentRoutes.push({
            path: routePath,
            sourcePath: possiblePath,
          });
          break;
        } catch {
          // File doesn't exist, try next
        }
      }
    }
  }

  return contentRoutes;
}

/**
 * Generate llms.txt (table of contents format).
 */
async function generateLlmsTxt(documents, outputPath, siteConfig) {
  const header = `# ${siteConfig.title}

> ${siteConfig.tagline || 'Documentation'}

This file contains links to documentation sections following the llmstxt.org standard.

## Table of Contents

`;

  const entries = documents
    .map(doc => `- [${doc.title}](${doc.url}): ${doc.description}`)
    .join('\n');

  const content = header + entries + '\n';

  await fs.writeFile(outputPath, content, 'utf-8');
  console.log(`✓ Generated ${outputPath} (${documents.length} entries)`);
}

/**
 * Generate llms-full.txt (full content format).
 */
async function generateLlmsFullTxt(documents, outputPath, siteConfig) {
  const header = `# ${siteConfig.title}

> ${siteConfig.tagline || 'Documentation'}

This file contains all documentation content in a single document following the llmstxt.org standard.

`;

  const sections = documents
    .map(doc => `## ${doc.title}\n\n${doc.content}`)
    .join('\n\n---\n\n');

  const content = header + sections + '\n';

  await fs.writeFile(outputPath, content, 'utf-8');
  console.log(`✓ Generated ${outputPath} (${documents.length} sections)`);
}

/**
 * Docusaurus plugin implementation.
 */
module.exports = function docusaurusPluginLlmsTxt(context, options) {
  const {
    generateLlmsTxt: enableLlmsTxt = true,
    generateLlmsFullTxt: enableLlmsFullTxt = true,
    llmsTxtFilename = 'llms.txt',
    llmsFullTxtFilename = 'llms-full.txt',
  } = options;

  return {
    name: 'docusaurus-plugin-llms-txt',

    async postBuild(props) {
      console.log('Generating LLM-friendly documentation using resolved routes...');

      const { siteConfig, outDir, routesPaths } = props;
      const siteUrl = siteConfig.url + (
        siteConfig.baseUrl.endsWith('/')
          ? siteConfig.baseUrl.slice(0, -1)
          : siteConfig.baseUrl || ''
      );

      // Extract content routes using Docusaurus's resolved route paths
      const contentRoutes = extractContentRoutes(routesPaths, context.siteDir);
      console.log(`Found ${contentRoutes.length} content routes from Docusaurus`);

      // Process each route
      const documents = [];
      for (const route of contentRoutes) {
        const filePath = path.join(context.siteDir, route.sourcePath);

        const doc = await processMarkdownFile(filePath, route.path, siteUrl);
        if (doc) {
          documents.push(doc);
        }
      }

      console.log(`Processed ${documents.length} documents`);

      // Generate output files
      if (enableLlmsTxt) {
        const llmsTxtPath = path.join(outDir, llmsTxtFilename);
        await generateLlmsTxt(documents, llmsTxtPath, siteConfig);
      }

      if (enableLlmsFullTxt) {
        const llmsFullTxtPath = path.join(outDir, llmsFullTxtFilename);
        await generateLlmsFullTxt(documents, llmsFullTxtPath, siteConfig);
      }
    },
  };
};
