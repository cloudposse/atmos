/**
 * Utility functions for the FileBrowser component.
 */
import {
  faFile,
  faFolder,
  faFolderOpen,
  faGear,
  faBarsStaggered,
  faLayerGroup,
  faCube,
  faCode,
  faFileCode,
  faFileLines,
  faImage,
  faLock,
  faTerminal,
  faBox,
} from '@fortawesome/free-solid-svg-icons';
import type { IconDefinition } from '@fortawesome/fontawesome-svg-core';
import type { TreeNode, FileNode, DirectoryNode, ExampleProject } from './types';

/**
 * Icon mapping for different file types.
 */
const iconMap: Record<string, IconDefinition> = {
  file: faFile,
  folder: faFolder,
  folderOpen: faFolderOpen,
  config: faGear,
  code: faBarsStaggered,
  stack: faLayerGroup,
  component: faCube,
  hcl: faFileCode,
  yaml: faBarsStaggered,
  json: faCode,
  markdown: faFileLines,
  image: faImage,
  lock: faLock,
  shell: faTerminal,
  docker: faBox,
};

/**
 * Gets the appropriate icon for a tree node.
 */
export function getFileIcon(node: TreeNode, isOpen = false): IconDefinition {
  if (node.type === 'directory') {
    return isOpen ? iconMap.folderOpen : iconMap.folder;
  }

  const { name, extension } = node as FileNode;
  const lowerName = name.toLowerCase();

  // Special file detection.
  if (/^atmos\.ya?ml$/i.test(lowerName)) return iconMap.config;
  if (/.*stack.*\.ya?ml$/i.test(lowerName)) return iconMap.stack;
  if (lowerName === 'dockerfile') return iconMap.docker;
  if (lowerName === 'makefile') return iconMap.shell;
  if (lowerName.endsWith('.lock')) return iconMap.lock;
  if (lowerName.endsWith('.sh') || lowerName.endsWith('.bash')) return iconMap.shell;

  // Extension-based detection.
  switch (extension.toLowerCase()) {
    case 'tf':
    case 'hcl':
      return iconMap.hcl;
    case 'yaml':
    case 'yml':
      return iconMap.yaml;
    case 'json':
      return iconMap.json;
    case 'md':
    case 'mdx':
      return iconMap.markdown;
    case 'png':
    case 'jpg':
    case 'jpeg':
    case 'gif':
    case 'svg':
    case 'webp':
      return iconMap.image;
    default:
      return iconMap.file;
  }
}

/**
 * Formats a file size in human-readable format.
 */
export function formatFileSize(bytes: number): string {
  if (bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  const size = bytes / Math.pow(1024, i);
  return `${size.toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
}

/**
 * Sorts tree node children: directories first, then files, alphabetically.
 */
export function sortTreeNodes(nodes: TreeNode[]): TreeNode[] {
  return [...nodes].sort((a, b) => {
    if (a.type === 'directory' && b.type !== 'directory') return -1;
    if (a.type !== 'directory' && b.type === 'directory') return 1;
    return a.name.localeCompare(b.name);
  });
}

/**
 * Finds a node in the tree by path.
 */
export function findNodeByPath(root: DirectoryNode, targetPath: string): TreeNode | null {
  if (root.path === targetPath) return root;

  for (const child of root.children) {
    if (child.path === targetPath) return child;
    if (child.type === 'directory') {
      const found = findNodeByPath(child, targetPath);
      if (found) return found;
    }
  }

  return null;
}

/**
 * Finds an example project by name.
 */
export function findExampleByName(
  examples: ExampleProject[],
  name: string
): ExampleProject | undefined {
  return examples.find((e) => e.name === name);
}

/**
 * Extracts the example name from a path.
 */
export function getExampleNameFromPath(path: string): string {
  const parts = path.split('/').filter(Boolean);
  return parts[0] || '';
}

/**
 * Gets breadcrumb parts from a path.
 */
export function getBreadcrumbParts(
  path: string,
  routeBasePath: string
): Array<{ name: string; path: string }> {
  const parts = path.split('/').filter(Boolean);
  const breadcrumbs: Array<{ name: string; path: string }> = [];

  let currentPath = routeBasePath;
  for (const part of parts) {
    currentPath = `${currentPath}/${part}`;
    breadcrumbs.push({ name: part, path: currentPath });
  }

  return breadcrumbs;
}

/**
 * Checks if a file is a markdown file.
 */
export function isMarkdownFile(node: FileNode): boolean {
  const ext = node.extension.toLowerCase();
  return ext === 'md' || ext === 'mdx';
}

/**
 * Checks if a file is a binary file (based on extension).
 */
export function isBinaryFile(node: FileNode): boolean {
  const binaryExtensions = [
    'png',
    'jpg',
    'jpeg',
    'gif',
    'svg',
    'webp',
    'ico',
    'pdf',
    'zip',
    'tar',
    'gz',
    'exe',
    'dll',
    'so',
    'dylib',
    'bin',
    'dat',
  ];
  return binaryExtensions.includes(node.extension.toLowerCase());
}

/**
 * Gets the parent path from a file/directory path.
 */
export function getParentPath(path: string): string {
  const parts = path.split('/').filter(Boolean);
  parts.pop();
  return parts.join('/');
}

/**
 * Wraps content in a fenced code block using a fence longer than any
 * backtick run already in the content, so a nested file containing its own
 * ``` doesn't close the wrapping fence early and corrupt the document.
 * Mirrors codeFence() in website/plugins/file-browser/index.js.
 */
function codeFence(content: string, language: string): string {
  const longestRun = Math.max(0, ...(content.match(/`+/g) ?? []).map((run) => run.length));
  const fence = '`'.repeat(Math.max(3, longestRun + 1));
  return `${fence}${language}\n${content}\n${fence}`;
}

/**
 * Recursively concatenates every readable file under a directory into one
 * Markdown document — the whole skill, references included, as a single
 * block of context that can be copied without installing anything. The
 * node's own `readme` (e.g. SKILL.md) leads, since it's the entry point;
 * everything else follows in tree order. Binary files and files skipped at
 * scan time (over the plugin's `maxFileSize`, `content === null`) are
 * omitted rather than left as empty sections.
 */
export function collectMarkdownContext(root: DirectoryNode): string {
  const sections: string[] = [];
  const readmePath = root.readme?.path;

  const addFile = (node: FileNode) => {
    if (node.content == null || isBinaryFile(node)) return;
    const trimmed = node.content.trim();
    const body = isMarkdownFile(node) ? trimmed : codeFence(trimmed, node.language);
    sections.push(`## ${node.path}\n\n${body}`);
  };

  const visit = (node: TreeNode) => {
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
 * Counts total files in a tree.
 */
export function countFiles(node: DirectoryNode): number {
  let count = 0;
  for (const child of node.children) {
    if (child.type === 'file') {
      count++;
    } else {
      count += countFiles(child);
    }
  }
  return count;
}

/**
 * Counts total directories in a tree.
 */
export function countDirectories(node: DirectoryNode): number {
  let count = 0;
  for (const child of node.children) {
    if (child.type === 'directory') {
      count++;
      count += countDirectories(child);
    }
  }
  return count;
}
