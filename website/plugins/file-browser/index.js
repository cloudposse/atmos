/**
 * Docusaurus plugin for creating a GitHub-like static file browser.
 * Scans a source directory and generates pages for browsing files.
 *
 * Each file gets its own URL, making content crawlable by Algolia.
 */
const fs = require('fs');
const path = require('path');
const crypto = require('crypto');

const matter = require('gray-matter');

// File names recognized as an item's primary content — the ones treated as
// its "readme" for description/title/tags extraction and index-page preview.
// SKILL.md is the Agent Skills open standard's fixed file name (skills don't
// ship a README.md), so it's recognized alongside the usual README variants.
const PRIMARY_CONTENT_FILENAMES = new Set(['readme.md', 'readme.mdx', 'skill.md']);

// Default patterns to exclude from scanning.
const DEFAULT_EXCLUDE_PATTERNS = [
  '**/node_modules/**',
  '**/.git/**',
  '**/.terraform/**',
  '**/vendor/**',
  '**/terraform.tfstate*',
  '**/.DS_Store',
  '**/*.tfstate',
  '**/*.tfstate.*',
  '**/terraform.tfvars',
  '**/.envrc',
];

// Default section order for the index page. Instances can override with the
// `tagOrder` plugin option (the gists gallery defines its own chapters).
const DEFAULT_TAG_ORDER = [
  'Quickstart',
  'Stacks',
  'Components',
  'Kubernetes',
  'Automation',
  'Hooks',
  'Emulators',
  'AI',
  'DX',
];

// Curated ("featured") examples, in display order. This list is editorial — like the
// roadmap's featured[] it is hand-maintained and never auto-promoted. These are pinned to the
// top of the /examples index page so the gallery leads with the demos we want people to try
// first. Edit deliberately.
const FEATURED = [
  'quick-start-simple',
  'quick-start-advanced',
  'sops-secrets',
  'toolchain',
  'custom-commands',
  'emulator-aws',
];

// Friendly display titles for examples (falls back to the directory name when absent).
const TITLES_MAP = {
  'quick-start-simple': 'Quick Start (Simple)',
  'quick-start-advanced': 'Quick Start (Advanced)',
  'sops-secrets': 'SOPS Secrets',
  toolchain: 'Toolchain',
  'custom-commands': 'Custom Commands',
  'emulator-aws': 'AWS Emulator',
  'emulator-k8s': 'Kubernetes Emulator',
  'packer-docker': 'Packer + Docker',
};

// Tags mapping for examples (an example can have multiple tags; the first tag
// is the example's section on the index page). A README front matter `tags:`
// list overrides this map, so new examples can self-categorize.
const TAGS_MAP = {
  'quick-start-simple': ['Quickstart'],
  'quick-start-advanced': ['Quickstart'],
  'demo-stacks': ['Stacks'],
  'demo-context': ['Stacks'],
  'demo-env': ['Stacks'],
  'config-profiles': ['Stacks'],
  'demo-auth': ['Stacks'],
  'demo-schemas': ['Stacks'],
  'stack-names': ['Stacks'],
  'remote-stack-imports': ['Stacks'],
  locals: ['Stacks'],
  compositions: ['Stacks'],
  'sops-secrets': ['Stacks'],
  'onepassword-secrets': ['Stacks'],
  'auth-stores': ['Stacks'],
  'demo-vendoring': ['Components'],
  'demo-component-versions': ['Components'],
  'source-provisioning': ['Components'],
  'demo-library': ['Components'],
  'custom-components': ['Components'],
  'container-component': ['Components'],
  'native-terraform': ['Components'],
  'terraform-tests': ['Components'],
  caching: ['Components'],
  'demo-workflows': ['Automation'],
  'demo-atlantis': ['Automation'],
  'custom-commands': ['Automation'],
  'interactive-workflows': ['Automation'],
  'demo-custom-command': ['Automation'],
  'generate-files': ['Automation'],
  'demo-ansible': ['Automation'],
  'packer-docker': ['Automation'],
  'background-steps': ['Automation'],
  'parallel-steps': ['Automation'],
  'workflow-retries': ['Automation'],
  'container-step': ['Automation'],
  'http-webhooks': ['Automation'],
  'say-something': ['Automation'],
  gitops: ['Automation'],
  'hooks-checkov': ['Hooks'],
  'hooks-custom-command': ['Hooks'],
  'hooks-infracost': ['Hooks'],
  'hooks-kics': ['Hooks'],
  'hooks-trivy': ['Hooks'],
  kubernetes: ['Kubernetes'],
  helm: ['Kubernetes'],
  kustomize: ['Kubernetes'],
  'demo-helmfile': ['Kubernetes'],
  'emulator-aws': ['Emulators'],
  'emulator-k8s': ['Emulators', 'Kubernetes'],
  'local-gitops': ['Emulators', 'Automation'],
  toolchain: ['DX'],
  devcontainer: ['DX'],
  'devcontainer-build': ['DX'],
  'container-sandbox': ['DX'],
  'secrets-masking': ['DX'],
  ai: ['DX'],
  'ai-claude-code': ['DX'],
  mcp: ['DX'],
  'mcp-for-ai-coding-assistants': ['DX'],
  'mcp-with-aws': ['DX', 'Automation'],
  scaffolding: ['Scaffold', 'Init'],
};

// Display labels for the `metadata.category` slug used by SKILL.md front matter
// (see agent-skills/skills/*/SKILL.md). Content with no top-level `tags:` front
// matter and a recognized category slug is tagged with this label instead of
// falling through to TAGS_MAP, which has no entries for skill directory names.
const CATEGORY_LABELS = {
  'core-config': 'Core Configuration & Architecture',
  orchestrators: 'Orchestration Engines',
  security: 'Auth, Secrets & Compliance',
  aws: 'AWS Integrations',
  'ci-automation': 'CI/CD & Automation',
  'state-versioning': 'State, Versioning & Provenance',
  'dev-tooling': 'Developer Tooling',
  'templating-data': 'Templates & Data',
  ai: 'AI & MCP',
  scaffolding: 'Scaffolding & Init',
};

// Cast recordings for examples whose README.md doubles as copied scaffold
// template output (`atmos scaffold generate` copies the whole source
// directory verbatim) — Docusaurus front matter in that README would leak
// into every generated project, so the cast is registered here instead. A
// README front matter `cast:` block still wins when present.
const CAST_MAP = {
  scaffolding: { file: '/casts/examples/scaffolding/generate-example.cast', title: 'atmos scaffold generate' },
};

// Documentation pages mapping for examples.
const DOCS_MAP = {
  'quick-start-simple': [
    { label: 'Quick Start', url: '/quick-start' },
    { label: 'Simple Tutorial', url: '/quick-start/simple' },
  ],
  'quick-start-advanced': [
    { label: 'Quick Start', url: '/quick-start' },
    { label: 'Advanced Tutorial', url: '/quick-start/advanced' },
  ],
  'demo-stacks': [
    { label: 'Stacks', url: '/stacks' },
  ],
  'demo-context': [
    { label: 'Describe Configuration', url: '/cli/commands/describe/config' },
  ],
  'demo-env': [
    { label: 'Environment Variables', url: '/stacks/env' },
  ],
  'demo-auth': [
    { label: 'Authentication', url: '/stacks/auth' },
    { label: 'Auth Commands', url: '/cli/commands/auth/usage' },
  ],
  'demo-schemas': [
    { label: 'JSON Schema Validation', url: '/validation/json-schema' },
  ],
  'demo-vendoring': [
    { label: 'Vendoring', url: '/vendor/' },
    { label: 'Vendor Configuration', url: '/vendor/vendor-config' },
    { label: 'CLI Configuration', url: '/cli/configuration/vendor' },
  ],
  'demo-component-versions': [
    { label: 'Component Versions', url: '/design-patterns/version-management' },
  ],
  'source-provisioning': [
    { label: 'Source Provisioning', url: '/cli/commands/terraform/source' },
  ],
  'demo-library': [
    { label: 'Components', url: '/components' },
  ],
  'demo-workflows': [
    { label: 'Workflows', url: '/workflows' },
    { label: 'CLI Configuration', url: '/cli/configuration/workflows' },
  ],
  'demo-atlantis': [
    { label: 'Atlantis Integration', url: '/cli/configuration/integrations/atlantis' },
  ],
  'custom-commands': [
    { label: 'Custom Commands', url: '/cli/configuration/commands' },
  ],
  'custom-components': [
    { label: 'Custom Component Types', url: '/components/custom' },
    { label: 'Custom Commands', url: '/cli/configuration/commands' },
    { label: 'Custom Component Types Reference', url: '/cli/configuration/commands/component#custom-component-types' },
  ],
  'interactive-workflows': [
    { label: 'Workflows', url: '/workflows' },
    { label: 'CLI Configuration', url: '/cli/configuration/workflows' },
  ],
  'generate-files': [
    { label: 'Generate Files', url: '/cli/commands/terraform/generate/files' },
  ],
  'config-profiles': [
    { label: 'CLI Configuration', url: '/cli/configuration' },
  ],
  toolchain: [
    { label: 'Toolchain Configuration', url: '/cli/configuration/toolchain' },
    { label: 'Toolchain Registries', url: '/cli/configuration/toolchain/registries' },
    { label: 'Toolchain Commands', url: '/cli/commands/toolchain/usage' },
  ],
  devcontainer: [
    { label: 'Devcontainer Configuration', url: '/cli/configuration/devcontainer' },
  ],
  'devcontainer-build': [
    { label: 'Devcontainer Configuration', url: '/cli/configuration/devcontainer' },
  ],
  'demo-helmfile': [
    { label: 'Helmfile', url: '/stacks/components/helmfile' },
  ],
  scaffolding: [
    { label: 'Init Command', url: '/cli/commands/init' },
    { label: 'Scaffold Generate', url: '/cli/commands/scaffold/generate' },
  ],
  'stack-names': [
    { label: 'Stack Names', url: '/stacks/name' },
  ],
  'demo-ansible': [
    { label: 'Ansible Playbook', url: '/cli/commands/ansible/playbook' },
  ],
  'mcp-with-aws': [
    { label: 'Custom Commands', url: '/cli/configuration/commands' },
    { label: 'Authentication', url: '/stacks/auth' },
    { label: 'Toolchain', url: '/cli/configuration/toolchain' },
  ],
  'packer-docker': [
    { label: 'Packer Components', url: '/components/packer' },
    { label: 'Packer Build', url: '/cli/commands/packer/build' },
    { label: 'Toolchain Configuration', url: '/cli/configuration/toolchain' },
  ],
  init: [
    { label: 'Init Command', url: '/cli/commands/init' },
    { label: 'Scaffold Generate', url: '/cli/commands/scaffold/generate' },
  ],
  scaffolds: [
    { label: 'Init Command', url: '/cli/commands/init' },
    { label: 'Scaffold Generate', url: '/cli/commands/scaffold/generate' },
  ],
};

// Map file extensions to syntax highlighting languages.
const LANGUAGE_MAP = {
  yaml: 'yaml',
  yml: 'yaml',
  json: 'json',
  tf: 'hcl',
  hcl: 'hcl',
  md: 'markdown',
  mdx: 'markdown',
  sh: 'bash',
  bash: 'bash',
  js: 'javascript',
  ts: 'typescript',
  jsx: 'jsx',
  tsx: 'tsx',
  py: 'python',
  go: 'go',
  dockerfile: 'docker',
  makefile: 'makefile',
  toml: 'toml',
  ini: 'ini',
  cfg: 'ini',
  txt: 'text',
  gitignore: 'text',
  editorconfig: 'ini',
};

/**
 * Escapes special regex characters in a string.
 * @param {string} str - String to escape.
 * @returns {string} - Escaped string safe for regex.
 */
function escapeRegex(str) {
  return str.replace(/[.+?^${}()|[\]\\]/g, '\\$&');
}

/**
 * Matches a path against a glob-style pattern.
 * Supports ** (any directories) and * (any characters in segment).
 * @param {string} filePath - Path to test.
 * @param {string} pattern - Glob pattern.
 * @returns {boolean} - True if matches.
 */
function matchesPattern(filePath, pattern) {
  // Escape special regex characters first, then convert glob syntax.
  // Order matters: escape first so placeholders aren't affected.
  const regexPattern = pattern
    .replace(/[.+?^${}()|[\]\\]/g, '\\$&') // Escape regex metacharacters (not * or /).
    .replace(/\*\*/g, '{{GLOBSTAR}}') // Placeholder for ** (matches any path depth).
    .replace(/\*/g, '[^/]*') // Single * matches anything except path separator.
    .replace(/{{GLOBSTAR}}/g, '.*') // Replace placeholder with .* (any characters).
    .replace(/\//g, '\\/'); // Escape path separators.
  const regex = new RegExp(`^${regexPattern}$`);
  return regex.test(filePath);
}

/**
 * Checks if a path should be excluded.
 * @param {string} relativePath - Path relative to source directory.
 * @param {string[]} excludePatterns - Glob patterns to exclude.
 * @returns {boolean} - True if should be excluded.
 */
function shouldExclude(relativePath, excludePatterns) {
  return excludePatterns.some((pattern) => matchesPattern(relativePath, pattern));
}

/**
 * Gets the language for syntax highlighting from a filename.
 * @param {string} filename - The filename.
 * @returns {string} - Language identifier.
 */
function getLanguageFromFilename(filename) {
  const lowerFilename = filename.toLowerCase();

  // Handle special filenames.
  if (lowerFilename === 'dockerfile') return 'docker';
  if (lowerFilename === 'makefile') return 'makefile';
  if (lowerFilename === '.gitignore') return 'text';
  if (lowerFilename === '.editorconfig') return 'ini';

  const ext = path.extname(filename).slice(1).toLowerCase();
  return LANGUAGE_MAP[ext] || 'text';
}

/**
 * Generates a short hash for a path.
 * @param {string} filePath - The file path.
 * @returns {string} - Short hash.
 */
function hashPath(filePath) {
  return crypto.createHash('md5').update(filePath).digest('hex').slice(0, 8);
}

/**
 * Generates GitHub URL for a file.
 * @param {string} relativePath - Path relative to source directory.
 * @param {object} options - Plugin options.
 * @returns {string} - GitHub URL.
 */
function generateGitHubUrl(relativePath, options) {
  const { githubRepo, githubBranch, githubPath } = options;
  if (!githubRepo) return null;
  const fullPath = githubPath ? `${githubPath}/${relativePath}` : relativePath;
  return `https://github.com/${githubRepo}/blob/${githubBranch || 'main'}/${fullPath}`;
}

/**
 * Parses README front matter with gray-matter so nested metadata (e.g. the
 * `cast:` block with `file:`/`title:`) survives, without changing README bodies.
 * @param {string} content - README content.
 * @returns {{data: object, body: string}} Parsed metadata and markdown body.
 */
function parseReadmeFrontmatter(content) {
  if (!content) {
    return { data: {}, body: '' };
  }

  try {
    const parsed = matter(content);
    return { data: parsed.data || {}, body: parsed.content.trim() };
  } catch (err) {
    // Malformed front matter: fall back to treating the whole file as body.
    return { data: {}, body: content };
  }
}

/**
 * Recursively scans a directory and builds a file tree.
 * @param {string} dirPath - Absolute path to directory.
 * @param {string} relativePath - Path relative to source root.
 * @param {object} options - Plugin options.
 * @returns {object} - Directory node with children.
 */
function scanDirectory(dirPath, relativePath, options) {
  const { excludePatterns, maxFileSize, githubRepo } = options;
  const entries = fs.readdirSync(dirPath, { withFileTypes: true });

  const children = [];
  let readme = null;
  let fileCount = 0;

  for (const entry of entries) {
    const entryRelativePath = relativePath ? `${relativePath}/${entry.name}` : entry.name;

    // Check exclusions.
    if (shouldExclude(entryRelativePath, excludePatterns)) {
      continue;
    }

    const entryAbsolutePath = path.join(dirPath, entry.name);

    if (entry.isDirectory()) {
      const subDir = scanDirectory(entryAbsolutePath, entryRelativePath, options);
      if (subDir.children.length > 0 || subDir.readme) {
        children.push(subDir);
        fileCount += subDir.fileCount;
      }
    } else if (entry.isFile()) {
      const stats = fs.statSync(entryAbsolutePath);
      const ext = path.extname(entry.name).slice(1).toLowerCase();
      const language = getLanguageFromFilename(entry.name);

      // Read file content if within size limit.
      let content = null;
      if (stats.size <= maxFileSize) {
        try {
          content = fs.readFileSync(entryAbsolutePath, 'utf-8');
        } catch {
          // Binary file or read error - skip content.
          content = null;
        }
      }

      const fileNode = {
        name: entry.name,
        type: 'file',
        path: entryRelativePath,
        relativePath: entry.name,
        extension: ext,
        language,
        size: stats.size,
        content,
        githubUrl: generateGitHubUrl(entryRelativePath, options),
      };

      // Track README files. SKILL.md is recognized alongside README.md/README.mdx
      // as primary content — it's the file name mandated by the Agent Skills
      // open standard (https://agentskills.io), which the skills gallery instance uses.
      if (PRIMARY_CONTENT_FILENAMES.has(entry.name.toLowerCase())) {
        readme = fileNode;
      }

      children.push(fileNode);
      fileCount++;
    }
  }

  // Sort: directories first, then files, alphabetically.
  children.sort((a, b) => {
    if (a.type === 'directory' && b.type !== 'directory') return -1;
    if (a.type !== 'directory' && b.type === 'directory') return 1;
    return a.name.localeCompare(b.name);
  });

  return {
    name: path.basename(dirPath),
    type: 'directory',
    path: relativePath,
    relativePath: path.basename(dirPath),
    children,
    readme,
    fileCount,
    githubUrl: generateGitHubUrl(relativePath, options),
  };
}

/**
 * Extracts description from README content.
 * @param {string} content - README content.
 * @returns {string} - Description or empty string.
 */
function extractDescription(content) {
  if (!content) return '';
  const { body: text } = parseReadmeFrontmatter(content);

  // Skip leading headers/blank lines, then collect every line of the first
  // paragraph (a blank line or the next heading ends it) so hard-wrapped
  // markdown source doesn't get cut mid-sentence or mid-token.
  const lines = text.split('\n');
  const paragraph = [];
  for (const line of lines) {
    const trimmed = line.trim();
    if (paragraph.length === 0) {
      if (!trimmed || trimmed.startsWith('#')) continue;
    } else if (!trimmed || trimmed.startsWith('#')) {
      break;
    }
    paragraph.push(trimmed);
  }

  return paragraph.join(' ');
}

// Target length for card descriptions, matching the length of the
// hand-curated `description:` front matter values already in use.
const MAX_DESCRIPTION_LENGTH = 200;

/**
 * Strips inline Markdown syntax down to plain text.
 * @param {string} text - Markdown text.
 * @returns {string} - Plain text.
 */
function stripMarkdown(text) {
  return text
    .replace(/!\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/`([^`]*)`/g, '$1')
    .replace(/\*\*([^*]*)\*\*/g, '$1')
    .replace(/\*([^*]*)\*/g, '$1')
    .replace(/_([^_]*)_/g, '$1');
}

/**
 * Reduces a description to a safe length for card display. Short
 * descriptions are returned untouched (preserving Markdown formatting);
 * long ones are flattened to plain text and cut at a word boundary so
 * truncation never leaves a dangling Markdown token.
 * @param {string} description - Raw (possibly Markdown) description.
 * @returns {string} - Description within MAX_DESCRIPTION_LENGTH.
 */
function reduceDescriptionLength(description) {
  if (!description || description.length <= MAX_DESCRIPTION_LENGTH) return description;

  const plain = stripMarkdown(description);
  if (plain.length <= MAX_DESCRIPTION_LENGTH) return plain;

  const truncated = plain.slice(0, MAX_DESCRIPTION_LENGTH);
  const lastSpace = truncated.lastIndexOf(' ');
  return `${(lastSpace > 0 ? truncated.slice(0, lastSpace) : truncated).trimEnd()}…`;
}

/**
 * Scans examples directory and builds project list.
 * @param {string} sourceDir - Path to examples directory.
 * @param {object} options - Plugin options.
 * @returns {object} - Examples tree data.
 */
function scanExamples(sourceDir, options) {
  const entries = fs.readdirSync(sourceDir, { withFileTypes: true });
  const examples = [];
  let totalFiles = 0;

  for (const entry of entries) {
    if (!entry.isDirectory()) continue;

    // Skip hidden directories.
    if (entry.name.startsWith('.')) continue;

    const examplePath = path.join(sourceDir, entry.name);
    const tree = scanDirectory(examplePath, entry.name, options);
    const readmeMetadata = tree.readme ? parseReadmeFrontmatter(tree.readme.content) : { data: {} };

    // Get description: prefer explicit front matter, fall back to the README's first paragraph.
    const frontmatterDescription =
      typeof readmeMetadata.data.description === 'string' ? readmeMetadata.data.description.trim() : '';
    const description = reduceDescriptionLength(
      frontmatterDescription || (tree.readme ? extractDescription(tree.readme.content) : '')
    );
    // Guard against a scalar `cast:` value so a malformed README can't break the build.
    // Front matter wins; otherwise fall back to CAST_MAP (see its comment for why).
    const castMeta = readmeMetadata.data.cast;
    const cast = castMeta && typeof castMeta === 'object' ? castMeta : CAST_MAP[entry.name] || {};

    // Tags: README front matter wins so examples can self-categorize; next, a
    // recognized SKILL.md `metadata.category` slug (see CATEGORY_LABELS); then
    // fall back to the hand-maintained map. The first tag is the index section.
    const frontmatterTags = Array.isArray(readmeMetadata.data.tags)
      ? readmeMetadata.data.tags.filter((tag) => typeof tag === 'string')
      : [];
    const categorySlug = readmeMetadata.data.metadata && readmeMetadata.data.metadata.category;
    const categoryLabel = typeof categorySlug === 'string' ? CATEGORY_LABELS[categorySlug] : undefined;
    const tags = frontmatterTags.length > 0
      ? frontmatterTags
      : categoryLabel
        ? [categoryLabel]
        : TAGS_MAP[entry.name] || [];

    // Check for atmos.yaml.
    const hasAtmosYaml = tree.children.some(
      (child) => child.type === 'file' && (child.name === 'atmos.yaml' || child.name === 'atmos.yml')
    );

    // Title: README front matter wins so examples name themselves; fall back
    // to the hand-maintained map, then the directory name.
    const frontmatterTitle =
      typeof readmeMetadata.data.title === 'string' ? readmeMetadata.data.title.trim() : '';

    examples.push({
      name: entry.name,
      path: entry.name,
      title: frontmatterTitle || TITLES_MAP[entry.name] || entry.name,
      description,
      hasReadme: !!tree.readme,
      hasAtmosYaml,
      featured: FEATURED.includes(entry.name),
      tags,
      docs: DOCS_MAP[entry.name] || [],
      cast: {
        file: cast.file || '',
        title: cast.title || '',
      },
      root: tree,
    });

    totalFiles += tree.fileCount;
  }

  // Sort examples alphabetically.
  examples.sort((a, b) => a.name.localeCompare(b.name));

  // Collect unique tags in display order. The order is a per-instance plugin
  // option so the examples and gists galleries can define their own chapters.
  const tagOrder = Array.isArray(options.tagOrder) && options.tagOrder.length > 0
    ? options.tagOrder
    : DEFAULT_TAG_ORDER;
  const knownTags = tagOrder.filter((tag) => examples.some((ex) => ex.tags.includes(tag)));
  // Any tag not in the configured order still gets a section (after the known
  // ones, alphabetically) instead of being silently dropped from the filter bar.
  const extraTags = [...new Set(examples.flatMap((ex) => ex.tags))]
    .filter((tag) => !tagOrder.includes(tag))
    .sort((a, b) => a.localeCompare(b));
  const tags = [...knownTags, ...extraTags];

  // Build the curated featured list in FEATURED order (skip any that don't resolve).
  const featured = FEATURED.map((name) => examples.find((ex) => ex.name === name)).filter(Boolean);

  return {
    examples,
    featured,
    tags,
    generatedAt: new Date().toISOString(),
    totalFiles,
    totalExamples: examples.length,
  };
}

/**
 * Recursively collects all files from a tree.
 * @param {object} node - Tree node.
 * @param {string} basePath - Base path for URLs.
 * @returns {object[]} - Array of file info objects.
 */
function collectFiles(node, basePath) {
  const files = [];

  if (node.type === 'file') {
    files.push({
      ...node,
      urlPath: `${basePath}/${node.path}`,
    });
  } else if (node.type === 'directory' && node.children) {
    for (const child of node.children) {
      files.push(...collectFiles(child, basePath));
    }
  }

  return files;
}

/**
 * Recursively collects all directories from a tree.
 * @param {object} node - Tree node.
 * @param {string} basePath - Base path for URLs.
 * @returns {object[]} - Array of directory info objects.
 */
function collectDirectories(node, basePath) {
  const dirs = [];

  if (node.type === 'directory') {
    dirs.push({
      ...node,
      urlPath: node.path ? `${basePath}/${node.path}` : basePath,
    });

    if (node.children) {
      for (const child of node.children) {
        dirs.push(...collectDirectories(child, basePath));
      }
    }
  }

  return dirs;
}

// Binary extensions skipped when concatenating Markdown context — mirrors
// isBinaryFile() in website/src/components/FileBrowser/utils.ts.
const BINARY_EXTENSIONS = new Set([
  'png', 'jpg', 'jpeg', 'gif', 'svg', 'webp', 'ico', 'pdf', 'zip', 'tar',
  'gz', 'exe', 'dll', 'so', 'dylib', 'bin', 'dat',
]);

/**
 * Wraps content in a fenced code block using a fence longer than any
 * backtick run already in the content, so a nested file containing its own
 * ``` doesn't close the wrapping fence early and corrupt the document.
 * Mirrors codeFence() in website/src/components/FileBrowser/utils.ts.
 * @param {string} content - File content (already trimmed by caller).
 * @param {string} language - Syntax-highlighting language hint.
 * @returns {string} - Fenced code block.
 */
function codeFence(content, language) {
  const longestRun = Math.max(0, ...(content.match(/`+/g) || []).map((run) => run.length));
  const fence = '`'.repeat(Math.max(3, longestRun + 1));
  return `${fence}${language}\n${content}\n${fence}`;
}

/**
 * Recursively concatenates every readable file under a directory into one
 * Markdown document — the whole item, nested reference files included, as a
 * single block of context. Mirrors collectMarkdownContext() in
 * website/src/components/FileBrowser/utils.ts (used by the client-side "Copy
 * as Markdown" button); this build-time twin backs the per-page `.md` files
 * written by generatePerPageMarkdown() below. Keep both in sync.
 * @param {object} root - Directory node (an example's `root`).
 * @returns {string} - Concatenated Markdown.
 */
function collectMarkdownContext(root) {
  const sections = [];
  const readmePath = root.readme ? root.readme.path : undefined;

  const addFile = (node) => {
    if (node.content == null || BINARY_EXTENSIONS.has((node.extension || '').toLowerCase())) return;
    const ext = (node.extension || '').toLowerCase();
    const trimmed = node.content.trim();
    const body = ext === 'md' || ext === 'mdx' ? trimmed : codeFence(trimmed, node.language);
    sections.push(`## ${node.path}\n\n${body}`);
  };

  const visit = (node) => {
    if (node.type === 'directory') {
      node.children.forEach(visit);
    } else if (node.path !== readmePath) {
      addFile(node);
    }
  };

  if (root.readme) addFile(root.readme);
  visit(root);

  return sections.join('\n\n---\n\n');
}

/**
 * Writes a raw `.md` file for each example, mirroring the sitewide
 * "append `.md` to any URL for raw Markdown" convention that
 * docusaurus-plugin-llms-txt provides for regular docs/blog pages — those
 * pages are invisible to that plugin since file-browser routes aren't
 * docs/blog content, so this instance has to generate its own.
 * @param {object} tree - The scanned examples tree (see scanExamples()).
 * @param {string} outDir - Docusaurus build output directory.
 * @param {string} routeBasePath - This instance's route base path.
 * @param {string} id - Plugin instance id, for logging.
 */
async function generatePerPageMarkdown(tree, outDir, routeBasePath, id) {
  let written = 0;
  for (const example of tree.examples) {
    const heading = `# ${example.title || example.name}\n\n${example.description ? `${example.description}\n\n` : ''}`;
    const body = heading + collectMarkdownContext(example.root);
    const outPath = path.join(outDir, routeBasePath, `${example.name}.md`);
    await fs.promises.mkdir(path.dirname(outPath), { recursive: true });
    await fs.promises.writeFile(outPath, body, 'utf-8');
    written += 1;
  }
  console.log(`[file-browser:${id}] Wrote ${written} per-page .md files`);
}

module.exports = function fileBrowserPlugin(context, options) {
  const {
    id = 'default',
    sourceDir = '../examples',
    routeBasePath = '/examples',
    title = 'Examples',
    description = 'Browse example projects',
    githubRepo = '',
    githubBranch = 'main',
    githubPath = '',
    disclaimer = '',
    excludePatterns = [],
    maxFileSize = 100 * 1024, // 100KB default.
    tagOrder = DEFAULT_TAG_ORDER,
    // Shows a free-text search box on the index page. Defaults to false so the
    // existing examples/gists instances render unchanged unless opted in.
    searchable = false,
    // Card icon and CTA label, see ICON_MAP in IndexPage.tsx. Default to the
    // folder icon and "Open" so examples/gists render unchanged.
    cardIcon = 'folder',
    cardCtaLabel = 'Open',
    // Shows a "Copy as Markdown" button on each item's root page, which
    // concatenates the item's readme and every nested file into one
    // clipboard-ready document. Defaults to false so existing instances
    // render unchanged unless opted in.
    enableCopyMarkdown = false,
    // Writes a raw `.md` file for each item's root page at build time (e.g.
    // /ai/skills/atmos-terraform.md), so it's fetchable the same way any
    // docs/blog page is via docusaurus-plugin-llms-txt's `<url>.md`
    // convention. Defaults to false so existing instances are unaffected.
    enablePerPageMarkdown = false,
    // Renders each item's title as a code-formatted `/name` (e.g. `/atmos-terraform`)
    // instead of plain text, signaling how it's invoked. Defaults to false —
    // examples/gists have friendly English titles this wouldn't suit.
    titleAsCode = false,
    // Label and command template for a per-item install command box, rendered at
    // the top of each item's root page (e.g. "Use this skill" / "atmos ai skill
    // install {name}"). `{name}` is replaced with the item's directory name.
    // Default to '' so existing instances render unchanged unless opted in.
    installCommandLabel = '',
    installCommandTemplate = '',
  } = options;

  const mergedExcludePatterns = [...DEFAULT_EXCLUDE_PATTERNS, ...excludePatterns];
  const absoluteSourceDir = path.resolve(context.siteDir, sourceDir);

  return {
    name: 'file-browser',

    async loadContent() {
      if (!fs.existsSync(absoluteSourceDir)) {
        console.warn(`[file-browser] Source directory not found: ${absoluteSourceDir}`);
        return { tree: { examples: [], featured: [], tags: [], totalFiles: 0, totalExamples: 0 } };
      }

      const tree = scanExamples(absoluteSourceDir, {
        excludePatterns: mergedExcludePatterns,
        maxFileSize,
        githubRepo,
        githubBranch,
        githubPath,
        tagOrder,
      });

      console.log(
        `[file-browser] Scanned ${tree.totalExamples} examples with ${tree.totalFiles} files`
      );

      return {
        tree,
        options: {
          routeBasePath,
          title,
          description,
          githubRepo,
          githubBranch,
          githubPath,
          disclaimer,
          searchable,
          cardIcon,
          cardCtaLabel,
          enableCopyMarkdown,
          titleAsCode,
          installCommandLabel,
          installCommandTemplate,
        },
      };
    },

    async contentLoaded({ content, actions }) {
      const { createData, addRoute, setGlobalData } = actions;
      const { tree, options: pluginOptions } = content;

      // Export examples data globally for use by EmbedExample component.
      setGlobalData({
        examples: tree.examples,
        options: pluginOptions,
      });

      // Create the main tree data file.
      const treeDataPath = await createData(
        `file-browser-tree-${id}.json`,
        JSON.stringify(tree, null, 2)
      );

      // Create options data file.
      const optionsDataPath = await createData(
        `file-browser-options-${id}.json`,
        JSON.stringify(pluginOptions, null, 2)
      );

      // Add index page route.
      addRoute({
        path: routeBasePath,
        component: '@site/src/components/FileBrowser/IndexPage.tsx',
        modules: {
          treeData: treeDataPath,
          optionsData: optionsDataPath,
        },
        exact: true,
      });

      // Add routes for each example.
      for (const example of tree.examples) {
        // Collect all directories for this example.
        const directories = collectDirectories(example.root, routeBasePath);

        // Add directory routes.
        for (const dir of directories) {
          // Create directory-specific data.
          const dirDataPath = await createData(
            `file-browser-dir-${hashPath(dir.path)}.json`,
            JSON.stringify(dir, null, 2)
          );

          addRoute({
            path: dir.urlPath,
            component: '@site/src/components/FileBrowser/DirectoryPage.tsx',
            modules: {
              treeData: treeDataPath,
              optionsData: optionsDataPath,
              dirData: dirDataPath,
            },
            exact: true,
          });
        }

        // Collect all files for this example.
        const files = collectFiles(example.root, routeBasePath);

        // Add file routes.
        for (const file of files) {
          // Create file-specific data (including content).
          const fileDataPath = await createData(
            `file-browser-file-${hashPath(file.path)}.json`,
            JSON.stringify(file, null, 2)
          );

          addRoute({
            path: file.urlPath,
            component: '@site/src/components/FileBrowser/FilePage.tsx',
            modules: {
              treeData: treeDataPath,
              optionsData: optionsDataPath,
              fileData: fileDataPath,
            },
            exact: true,
          });
        }
      }
    },

    getPathsToWatch() {
      // Watch the source directory for changes during development.
      return [absoluteSourceDir];
    },

    async postBuild({ content, outDir }) {
      if (!enablePerPageMarkdown) return;
      const { tree, options: pluginOptions } = content;
      await generatePerPageMarkdown(tree, outDir, pluginOptions.routeBasePath, id);
    },
  };
};
