/**
 * Custom Docusaurus plugin to generate LLM-friendly markdown artifacts:
 *
 *  - llms.txt              — table-of-contents index (llmstxt.org standard)
 *  - llms-full.txt         — entire docs corpus, normalized
 *  - <page>.md             — per-page raw markdown (when generatePerPageMarkdown: true)
 *
 * Routes come from Docusaurus's resolved `routesPaths` array, which already
 * respects frontmatter slug overrides, routeBasePath, and numeric/date prefixes.
 * MDX bodies are normalized via the AST-based mdx-normalize.mjs helper so that
 * custom components (Intro, Tabs, Terminal, dl/dt/dd, …) round-trip into
 * portable Markdown instead of being regex-stripped.
 */

const fs = require('fs').promises;
const path = require('path');
const matter = require('gray-matter');

// mdx-normalize.mjs is ESM (depends on unified/remark v11). Loaded lazily on
// first call via dynamic import so the CJS plugin entry stays compatible with
// Docusaurus's loader.
let _normalizeMdxToMarkdown = null;
async function getNormalizer() {
  if (!_normalizeMdxToMarkdown) {
    const mod = await import('./mdx-normalize.mjs');
    _normalizeMdxToMarkdown = mod.normalizeMdxToMarkdown;
  }
  return _normalizeMdxToMarkdown;
}

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
 * Read and process a markdown/MDX file.
 *
 * Returns the parsed frontmatter, the raw post-matter body, the normalized
 * Markdown body (MDX components rewritten to portable Markdown), and the
 * fully-qualified site URL for the page.
 */
async function processMarkdownFile(filePath, url, siteUrl) {
  try {
    const fileContent = await fs.readFile(filePath, 'utf-8');
    const { data: frontmatter, content } = matter(fileContent);

    // Skip draft files.
    if (frontmatter.draft === true) {
      return null;
    }

    const title = extractTitle(frontmatter, content, filePath);
    const description = extractDescription(frontmatter, content);
    const fullUrl = new URL(url, siteUrl).toString();

    const normalize = await getNormalizer();
    let normalizedContent;
    try {
      normalizedContent = (await normalize(content)).trim();
    } catch (err) {
      console.warn(`mdx-normalize failed for ${filePath}: ${err.message}. Falling back to raw body.`);
      normalizedContent = content.trim();
    }

    return {
      title,
      description,
      url: fullUrl,
      routePath: url,
      content: normalizedContent,
    };
  } catch (error) {
    console.warn(`Error processing ${filePath}: ${error.message}`);
    return null;
  }
}

/**
 * Compute the per-page .md output path for a route.
 *
 *   /                          → index.md
 *   /cli/commands/version      → cli/commands/version.md
 *   /cli/commands/version/     → cli/commands/version.md
 */
function perPageOutputPath(outDir, routePath) {
  // Character-walking trim avoids the polynomial-regex pattern CodeQL flags
  // on `/^\/+/` / `/\/+$/`; routePath comes from Docusaurus so it's already
  // bounded, but the linear loop is also marginally cheaper.
  let start = 0;
  let end = routePath.length;
  while (start < end && routePath.charCodeAt(start) === 47) start += 1; // '/'
  while (end > start && routePath.charCodeAt(end - 1) === 47) end -= 1;
  const rel = start === end ? 'index' : routePath.slice(start, end);
  return path.join(outDir, rel + '.md');
}

/**
 * Write per-page raw markdown files mirroring the docs route tree.
 *
 * Each file gets an H1 derived from the page's title (frontmatter →
 * sidebar_label → first heading) followed by the normalized body. Static
 * hosting layers serve these as text/markdown by extension.
 */
async function generatePerPageMarkdown(documents, outDir) {
  let written = 0;
  for (const doc of documents) {
    if (!doc.routePath) continue;
    const outPath = perPageOutputPath(outDir, doc.routePath);
    const body = `# ${doc.title}\n\n${doc.content}\n`;
    await fs.mkdir(path.dirname(outPath), { recursive: true });
    await fs.writeFile(outPath, body, 'utf-8');
    written += 1;
  }
  console.log(`✓ Wrote ${written} per-page .md files into ${outDir}`);
}

/**
 * Write a synthesized index.md for the site root when no doc owns the `/`
 * permalink (Atmos serves a React landing page there). Gives crawlers, LLMs,
 * and "what does this site contain?" probes a discoverable Markdown overview
 * pointing to llms.txt, llms-full.txt, and the main docs sections.
 */
async function generateRootIndexMarkdown(documents, outDir, siteConfig) {
  // Don't overwrite if a real doc claims the root.
  const rootAlreadyWritten = documents.some((d) => d.routePath === '/');
  if (rootAlreadyWritten) return;

  // Group top-level sections from the documents list.
  const sectionMap = new Map();
  for (const doc of documents) {
    if (!doc.routePath || doc.routePath === '/') continue;
    const segments = doc.routePath.split('/').filter(Boolean);
    if (segments.length === 0) continue;
    const section = segments[0];
    if (!sectionMap.has(section)) sectionMap.set(section, []);
    sectionMap.get(section).push(doc);
  }

  const sectionOrder = ['intro', 'quick-start', 'install', 'learn', 'cli', 'stacks', 'components', 'design-patterns', 'ai', 'ci', 'best-practices', 'cheatsheet', 'reference', 'faq', 'changelog'];
  const knownSections = sectionOrder.filter((s) => sectionMap.has(s));
  const extraSections = [...sectionMap.keys()].filter((s) => !sectionOrder.includes(s)).sort();
  const orderedSections = [...knownSections, ...extraSections];

  const lines = [];
  lines.push(`# ${siteConfig.title}`);
  lines.push('');
  if (siteConfig.tagline) {
    lines.push(`> ${siteConfig.tagline}`);
    lines.push('');
  }
  lines.push('This is the Markdown index for the Atmos documentation site, written for crawlers, LLMs, and tooling. Every doc page is also available as raw Markdown by appending `.md` to its URL.');
  lines.push('');
  lines.push('## Machine-readable indexes');
  lines.push('');
  lines.push('- [`/llms.txt`](/llms.txt) — table-of-contents index of every page (follows the llmstxt.org standard).');
  lines.push('- [`/llms-full.txt`](/llms-full.txt) — entire docs corpus in a single file.');
  lines.push('- Per-page Markdown — any docs URL serves Markdown at `<url>.md` with `Content-Type: text/markdown`.');
  lines.push('');
  lines.push('## Sections');
  lines.push('');
  for (const section of orderedSections) {
    const docs = sectionMap.get(section);
    if (!docs || docs.length === 0) continue;
    // Surface the section landing page (shortest route in the group) when one exists.
    const landing = docs.find((d) => d.routePath === `/${section}`);
    const heading = landing ? `[${landing.title}](${landing.routePath}.md)` : `${section}`;
    lines.push(`- ${heading} — ${docs.length} page${docs.length === 1 ? '' : 's'}.`);
  }
  lines.push('');
  lines.push('## Site root');
  lines.push('');
  lines.push(`The HTML homepage at \`${siteConfig.url}/\` is a marketing landing page rendered from React, not Markdown. Start exploring the docs at [/intro](/intro.md) or browse the full index at [/llms.txt](/llms.txt).`);
  lines.push('');

  const outPath = path.join(outDir, 'index.md');
  await fs.writeFile(outPath, lines.join('\n'), 'utf-8');
  console.log(`✓ Wrote synthesized index.md to ${outPath}`);
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
    generatePerPageMarkdown: enablePerPageMarkdown = false,
    llmsTxtFilename = 'llms.txt',
    llmsFullTxtFilename = 'llms-full.txt',
  } = options;

  // Docusaurus reports `source` as `@site/...` — convert to an absolute path.
  function normalizeSourcePath(src) {
    if (!src) return src;
    if (src.startsWith('@site/')) return path.join(context.siteDir, src.slice('@site/'.length));
    if (path.isAbsolute(src)) return src;
    return path.join(context.siteDir, src);
  }

  // Walk the .docusaurus/<plugin>/<id>/*.json cache that Docusaurus writes
  // before postBuild. Each per-document file carries the authoritative
  // permalink + source, so we don't have to re-implement Docusaurus's
  // permalink resolution (frontmatter id/slug overrides, routeBasePath, etc).
  async function buildPermalinkSourceMap() {
    const map = new Map();
    const cacheRoot = path.join(context.siteDir, '.docusaurus');
    const pluginIds = ['docusaurus-plugin-content-docs', 'docusaurus-plugin-content-blog'];
    for (const pluginId of pluginIds) {
      const pluginRoot = path.join(cacheRoot, pluginId);
      let entries;
      try {
        entries = await fs.readdir(pluginRoot, { withFileTypes: true });
      } catch {
        continue; // plugin not present
      }
      for (const instanceEntry of entries) {
        if (!instanceEntry.isDirectory()) continue;
        const instanceDir = path.join(pluginRoot, instanceEntry.name);
        let files;
        try {
          files = await fs.readdir(instanceDir);
        } catch {
          continue;
        }
        for (const file of files) {
          if (!file.endsWith('.json')) continue;
          if (file === '__plugin.json') continue;
          try {
            const raw = await fs.readFile(path.join(instanceDir, file), 'utf-8');
            const data = JSON.parse(raw);
            const permalink = data.permalink || data.metadata?.permalink;
            const source = data.source || data.metadata?.source;
            if (permalink && source) {
              map.set(permalink, normalizeSourcePath(source));
            }
          } catch {
            // Skip unparseable cache files.
          }
        }
      }
    }
    return map;
  }

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

      const permalinkToSource = await buildPermalinkSourceMap();
      console.log(`Loaded ${permalinkToSource.size} permalink→source entries from .docusaurus cache`);

      // Use the authoritative map when populated; fall back to filename-based
      // search for any route the map didn't cover (defensive).
      const contentRoutes = [];
      const seen = new Set();
      for (const routePath of routesPaths) {
        if (seen.has(routePath)) continue;
        const sourceAbs = permalinkToSource.get(routePath);
        if (sourceAbs) {
          contentRoutes.push({ path: routePath, sourceAbs });
          seen.add(routePath);
        }
      }
      // Always backfill from the legacy filename-based search for any route
      // the cache map didn't cover (per-PR-feedback: partial misses were
      // silently dropped when the previous gate ran only on an empty map).
      const legacy = extractContentRoutes(routesPaths, context.siteDir);
      for (const r of legacy) {
        if (seen.has(r.path)) continue;
        contentRoutes.push({ path: r.path, sourceAbs: path.join(context.siteDir, r.sourcePath) });
        seen.add(r.path);
      }
      console.log(`Found ${contentRoutes.length} content routes from Docusaurus`);

      // Process each route
      const documents = [];
      for (const route of contentRoutes) {
        const doc = await processMarkdownFile(route.sourceAbs, route.path, siteUrl);
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

      if (enablePerPageMarkdown) {
        await generatePerPageMarkdown(documents, outDir);
        await generateRootIndexMarkdown(documents, outDir, siteConfig);
      }
    },
  };
};
