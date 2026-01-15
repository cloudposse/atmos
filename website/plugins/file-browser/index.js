/**
 * Docusaurus plugin for creating a GitHub-like static file browser.
 * Scans a source directory and generates pages for browsing files.
 *
 * Each file gets its own URL, making content crawlable by Algolia.
 */
const fs = require('fs');
const path = require('path');
const crypto = require('crypto');

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

// Category mapping for examples.
const CATEGORY_MAP = {
  'quick-start-simple': 'Quickstart',
  'quick-start-advanced': 'Quickstart',
  'demo-stacks': 'Stacks',
  'demo-context': 'Stacks',
  'demo-env': 'Stacks',
  'config-profiles': 'Stacks',
  'demo-auth': 'Stacks',
  'demo-schemas': 'Stacks',
  'demo-vendoring': 'Components',
  'demo-component-versions': 'Components',
  'source-provisioning': 'Components',
  'demo-library': 'Components',
  'demo-workflows': 'Automation',
  'demo-atlantis': 'Automation',
  'demo-custom-command': 'Automation',
  toolchain: 'DX',
  devcontainer: 'DX',
  'devcontainer-build': 'DX',
  'demo-localstack': 'DX',
  'demo-helmfile': 'DX',
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
  dockerfile: 'dockerfile',
  makefile: 'makefile',
  toml: 'toml',
  ini: 'ini',
  cfg: 'ini',
  txt: 'text',
  gitignore: 'text',
  editorconfig: 'ini',
};

/**
 * Matches a path against a glob-style pattern.
 * Supports ** (any directories) and * (any characters in segment).
 * @param {string} filePath - Path to test.
 * @param {string} pattern - Glob pattern.
 * @returns {boolean} - True if matches.
 */
function matchesPattern(filePath, pattern) {
  // Convert glob pattern to regex.
  const regexPattern = pattern
    .replace(/\*\*/g, '{{GLOBSTAR}}')
    .replace(/\*/g, '[^/]*')
    .replace(/{{GLOBSTAR}}/g, '.*')
    .replace(/\//g, '\\/');
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
  if (lowerFilename === 'dockerfile') return 'dockerfile';
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

      // Track README files.
      if (entry.name.toLowerCase() === 'readme.md' || entry.name.toLowerCase() === 'readme.mdx') {
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

  // Remove frontmatter if present.
  let text = content;
  if (text.startsWith('---')) {
    const endIndex = text.indexOf('---', 3);
    if (endIndex !== -1) {
      text = text.slice(endIndex + 3).trim();
    }
  }

  // Skip headers and find first paragraph.
  const lines = text.split('\n');
  for (const line of lines) {
    const trimmed = line.trim();
    // Skip empty lines and headers.
    if (!trimmed || trimmed.startsWith('#')) continue;
    // Return first non-empty, non-header line (truncated).
    const description = trimmed.slice(0, 200);
    return description.length < trimmed.length ? `${description}...` : description;
  }

  return '';
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

    // Get description from README.
    const description = tree.readme ? extractDescription(tree.readme.content) : '';

    // Check for atmos.yaml.
    const hasAtmosYaml = tree.children.some(
      (child) => child.type === 'file' && (child.name === 'atmos.yaml' || child.name === 'atmos.yml')
    );

    examples.push({
      name: entry.name,
      path: entry.name,
      description,
      hasReadme: !!tree.readme,
      hasAtmosYaml,
      category: CATEGORY_MAP[entry.name] || 'Other',
      root: tree,
    });

    totalFiles += tree.fileCount;
  }

  // Sort examples alphabetically.
  examples.sort((a, b) => a.name.localeCompare(b.name));

  // Collect unique categories in display order.
  const categoryOrder = ['Quickstart', 'Stacks', 'Components', 'Automation', 'DX'];
  const categories = categoryOrder.filter((cat) => examples.some((ex) => ex.category === cat));

  return {
    examples,
    categories,
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
    excludePatterns = [],
    maxFileSize = 100 * 1024, // 100KB default.
  } = options;

  const mergedExcludePatterns = [...DEFAULT_EXCLUDE_PATTERNS, ...excludePatterns];
  const absoluteSourceDir = path.resolve(context.siteDir, sourceDir);

  return {
    name: 'file-browser',

    async loadContent() {
      if (!fs.existsSync(absoluteSourceDir)) {
        console.warn(`[file-browser] Source directory not found: ${absoluteSourceDir}`);
        return { tree: { examples: [], totalFiles: 0, totalExamples: 0 } };
      }

      const tree = scanExamples(absoluteSourceDir, {
        excludePatterns: mergedExcludePatterns,
        maxFileSize,
        githubRepo,
        githubBranch,
        githubPath,
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
        },
      };
    },

    async contentLoaded({ content, actions }) {
      const { createData, addRoute } = actions;
      const { tree, options: pluginOptions } = content;

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
  };
};
