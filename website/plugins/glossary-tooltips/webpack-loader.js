const path = require('path');

/**
 * Webpack loader that transforms markdown links to terms into Term React components.
 * Example: [component](/terms/component) -> <Term termId="/terms/component">component</Term>
 */
module.exports = function(source) {
  const options = this.query || {};
  const termsDir = options.termsDir || 'docs/glossary/';
  const baseUrl = options.baseUrl || '/';

  // Regex to match markdown links: [text](url).
  const markdownLinkRegex = /(?<!!)\[([^\]]+)\]\(([^)]+)\)/g;

  let hasTerms = false;
  let transformedSource = source;

  // Find all markdown links.
  const matches = Array.from(source.matchAll(markdownLinkRegex));

  for (const match of matches) {
    const fullMatch = match[0];
    const linkText = match[1];
    const linkUrl = match[2];

    // Broad initial filter: check if this might be a term link.
    // We use a permissive filter here to catch both absolute (/terms/) and relative (../glossary/)
    // links, then perform more specific validation below to ensure only actual term links are transformed.
    if (linkUrl.includes('/terms/') || linkUrl.includes('glossary/')) {
      // Skip multi-line link text.
      if (linkText.includes('\n')) {
        continue;
      }

      // Extract term ID from URL.
      let termId = linkUrl;

      // Handle relative paths.
      if (linkUrl.startsWith('./') || linkUrl.startsWith('../')) {
        // Convert relative path to absolute term path.
        const resourceDir = path.dirname(this.resourcePath);
        const absolutePath = path.resolve(resourceDir, linkUrl);
        const relativePath = path.relative(process.cwd(), absolutePath);

        if (relativePath.includes('glossary/')) {
          // Extract the term filename without extension.
          const termFile = path.basename(linkUrl, path.extname(linkUrl));
          termId = `/terms/${termFile}`;
        }
      } else if (linkUrl.includes('glossary/')) {
        // Handle absolute paths that include glossary.
        const termFile = path.basename(linkUrl, path.extname(linkUrl));
        termId = `/terms/${termFile}`;
      } else if (linkUrl.startsWith('/terms/')) {
        // Already in the correct format.
        termId = linkUrl.replace(/\.(md|mdx)$/, '');
      }

      // Normalize term ID.
      termId = baseUrl.replace(/\/$/, '') + termId.replace(/\.(md|mdx)$/, '');

      // Replace the markdown link with Term component.
      const termComponent = `<Term termId="${termId}">${linkText}</Term>`;
      transformedSource = transformedSource.replace(fullMatch, termComponent);
      hasTerms = true;
    }
  }

  // If we found terms, add the import statement at the top.
  if (hasTerms) {
    // Find the frontmatter boundary or start of content.
    const frontmatterRegex = /^---\r?\n[\s\S]*?\r?\n---\r?\n/;
    const frontmatterMatch = transformedSource.match(frontmatterRegex);

    const importStatement = `import Term from '@site/src/components/Term';\n\n`;

    if (frontmatterMatch) {
      // Insert after frontmatter.
      const insertPosition = frontmatterMatch[0].length;
      transformedSource =
        transformedSource.slice(0, insertPosition) +
        importStatement +
        transformedSource.slice(insertPosition);
    } else {
      // Insert at the beginning.
      transformedSource = importStatement + transformedSource;
    }
  }

  return transformedSource;
};
